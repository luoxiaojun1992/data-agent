// lib/voice.ts — SPEC-099 服务端语音转写（分片上传）
//
// 替代 SPEC-095 的 whisper.wasm 本地转写。录音时每 5s 分片上传到主后端
// （/api/v1/voice/start → chunk → finish），停止后合并一次性转写返回。
//
// ⚠️ 分片策略（实测踩坑）：Chrome MediaRecorder.start(timeslice) 只有第一个
// chunk 带 EBML webm 头，后续 chunk 是裸 Cluster 片段，无法独立 decodeAudioData。
// 因此本地累积原始分片，每 5s 把「已累积的完整 webm」全量解码 → 重采样为
// 16kHz int16 PCM（little-endian，与服务端 binary.LittleEndian 对齐）→ 只把
// 「新增 PCM 部分」按单分片上限切片上传。转写仍是结束一次性（D2）。

export type VoicePhase = 'idle' | 'recording' | 'transcribing';

export type ApiFetch = (url: string, init?: RequestInit) => Promise<Response>;

const CHUNK_MS = 5000; // 分片上传间隔（5s）
const MAX_DURATION_MS = 60000; // 最长录音 60s（硬上限，与服务端 TTL 对齐）
const SAMPLE_RATE = 16000; // 目标采样率（whisper 要求）
// 与后端 Manager.maxChunk 默认值一致（5s×16kHz×2B）。上传时按此切片，
// 避免解码/上传耗时间隔漂移导致单片超限 413。
const MAX_CHUNK_BYTES = 160000;

// 模块级录音状态（同一时间只允许一路录音）。
let mediaRecorder: MediaRecorder | null = null;
let mediaStream: MediaStream | null = null;
let audioCtx: AudioContext | null = null;
let requestId: string | null = null;
let seq = 0;
let localChunks: Blob[] = []; // 本地累积的原始 webm 分片（连续流片段）
let uploadedBytes = 0; // 已上传的 PCM 字节数（增量游标）
let uploadChain: Promise<void> = Promise.resolve(); // 串行化上传，保证 seq 顺序
let chunkTimer: ReturnType<typeof setInterval> | null = null;
let maxTimer: ReturnType<typeof setTimeout> | null = null;
let stopping = false;

function getAudioContext(): AudioContext {
  if (!audioCtx) {
    const Ctor =
      window.AudioContext ||
      (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    audioCtx = new Ctor({ sampleRate: SAMPLE_RATE });
  }
  return audioCtx;
}

function cleanup() {
  mediaRecorder = null;
  requestId = null;
  seq = 0;
  localChunks = [];
  uploadedBytes = 0;
  uploadChain = Promise.resolve();
  stopping = false;
  if (chunkTimer) {
    clearInterval(chunkTimer);
    chunkTimer = null;
  }
  if (maxTimer) {
    clearTimeout(maxTimer);
    maxTimer = null;
  }
}

// bytes → base64（分块避免展开大数组导致 call stack 溢出）。
function bytesToBase64(bytes: Uint8Array): string {
  let binary = '';
  const CHUNK = 0x8000;
  for (let i = 0; i < bytes.length; i += CHUNK) {
    binary += String.fromCharCode(...bytes.subarray(i, i + CHUNK));
  }
  return btoa(binary);
}

// 把「已累积的完整 webm」解码为 16kHz 单声道 int16 PCM（little-endian）。
async function decodeAccumulated(): Promise<Uint8Array> {
  const blob = new Blob(localChunks);
  const arr = await blob.arrayBuffer();
  const ctx = getAudioContext();
  // 不带 type，让 decodeAudioData 自行嗅探容器（Chrome=webm、Safari=mp4）。
  const audioBuffer = await ctx.decodeAudioData(arr);
  // 重采样到 16kHz 单声道（OfflineAudioContext 自动重采样）。
  const offline = new OfflineAudioContext(
    1,
    Math.ceil(audioBuffer.duration * SAMPLE_RATE),
    SAMPLE_RATE,
  );
  const src = offline.createBufferSource();
  src.buffer = audioBuffer;
  src.connect(offline.destination);
  src.start(0);
  const rendered = await offline.startRendering();
  const float32 = rendered.getChannelData(0);

  // float32 [-1,1] → int16 little-endian。
  const int16 = new Int16Array(float32.length);
  for (let i = 0; i < float32.length; i++) {
    const s = Math.max(-1, Math.min(1, float32[i]));
    int16[i] = s < 0 ? Math.round(s * 0x8000) : Math.round(s * 0x7fff);
  }
  return new Uint8Array(int16.buffer);
}

// 上传一片 PCM（调用方已保证 seq 分配与链式串行）。
async function uploadChunk(apiFetch: ApiFetch, audio: Uint8Array): Promise<void> {
  if (!requestId) throw new Error('录音会话已失效');
  const res = await apiFetch('/voice/chunk', {
    method: 'POST',
    body: JSON.stringify({ request_id: requestId, seq: seq++, audio: bytesToBase64(audio) }),
  });
  if (!res.ok) {
    let msg = `分片上传失败 (${res.status})`;
    try {
      const d = await res.json();
      if (d?.error) msg = d.error;
    } catch {
      // keep default
    }
    throw new Error(msg);
  }
}

// flush：解码已累积的 webm → 上传「新增 PCM」（超上限切片）。串行入链。
function enqueueFlush(apiFetch: ApiFetch): void {
  uploadChain = uploadChain.then(async () => {
    const all = await decodeAccumulated();
    let fresh = all.subarray(uploadedBytes);
    if (fresh.length === 0) return;
    for (let offset = 0; offset < fresh.length; offset += MAX_CHUNK_BYTES) {
      const piece = fresh.subarray(offset, offset + MAX_CHUNK_BYTES);
      await uploadChunk(apiFetch, piece);
    }
    uploadedBytes = all.byteLength;
  });
}

/**
 * 开始录音：申请麦克风 → POST /voice/start 建立会话 → MediaRecorder 本地
 * 累积原始流 → 每 5s 解码增量上传。到达 60s 硬上限时自动停止并转写，结果
 * 经 onAutoStop 回调返回。
 */
export async function startRecording(
  apiFetch: ApiFetch,
  opts: { lang?: string; onAutoStop?: (text: string) => void } = {},
): Promise<void> {
  mediaStream = await navigator.mediaDevices.getUserMedia({
    audio: {
      channelCount: 1,
      echoCancellation: true,
      autoGainControl: true,
      noiseSuppression: true,
    },
    video: false,
  });

  const startRes = await apiFetch('/voice/start', {
    method: 'POST',
    body: JSON.stringify({ lang: opts.lang ?? 'auto' }),
  });
  if (!startRes.ok) {
    mediaStream.getTracks().forEach((t) => t.stop());
    mediaStream = null;
    let msg = `无法开始录音 (${startRes.status})`;
    try {
      const d = await startRes.json();
      if (d?.error) msg = d.error;
    } catch {
      // keep default
    }
    throw new Error(msg);
  }
  const data = await startRes.json();
  requestId = data?.request_id;
  if (!requestId) {
    mediaStream.getTracks().forEach((t) => t.stop());
    mediaStream = null;
    throw new Error('服务端未返回会话 ID');
  }

  seq = 0;
  localChunks = [];
  uploadedBytes = 0;
  uploadChain = Promise.resolve();
  stopping = false;

  mediaRecorder = new MediaRecorder(mediaStream);
  // timeslice 仅用于把流数据定期 flush 到本地（后续 chunk 无独立头，
  // 不能直接上传——必须先与已有分片合并解码，见文件头注释）。
  mediaRecorder.ondataavailable = (e) => {
    if (e.data.size > 0) localChunks.push(e.data);
  };
  mediaRecorder.start(1000);

  chunkTimer = setInterval(() => enqueueFlush(apiFetch), CHUNK_MS);

  maxTimer = setTimeout(() => {
    doStopAndTranscribe(apiFetch)
      .then((text) => opts.onAutoStop?.(text))
      .catch(() => {
        // 自动停止失败静默（用户可手动重试），不打断页面。
      });
  }, MAX_DURATION_MS);
}

// 停止录音 → 上传尾部增量 → finish 一次性转写。
async function doStopAndTranscribe(apiFetch: ApiFetch): Promise<string> {
  if (stopping) return '';
  stopping = true;
  if (chunkTimer) {
    clearInterval(chunkTimer);
    chunkTimer = null;
  }
  if (maxTimer) {
    clearTimeout(maxTimer);
    maxTimer = null;
  }
  if (!mediaRecorder || !requestId) {
    stopping = false;
    throw new Error('未在录音');
  }

  return new Promise<string>((resolve, reject) => {
    mediaRecorder!.onstop = async () => {
      mediaStream?.getTracks().forEach((t) => t.stop());
      mediaStream = null;
      try {
        await uploadChain; // 等待进行中的 flush 完成
        // 最终 flush：stop 会触发最后一次 ondataavailable（已在 localChunks）
        const all = await decodeAccumulated();
        const fresh = all.subarray(uploadedBytes);
        for (let offset = 0; offset < fresh.length; offset += MAX_CHUNK_BYTES) {
          await uploadChunk(apiFetch, fresh.subarray(offset, offset + MAX_CHUNK_BYTES));
        }
        const rid = requestId;
        cleanup();
        const res = await apiFetch('/voice/finish', {
          method: 'POST',
          body: JSON.stringify({ request_id: rid }),
        });
        if (!res.ok) {
          let msg = `转写失败 (${res.status})`;
          try {
            const d = await res.json();
            if (d?.error) msg = d.error;
          } catch {
            // keep default
          }
          throw new Error(msg);
        }
        const data = await res.json();
        resolve(data?.text ?? '');
      } catch (e) {
        cleanup();
        reject(e as Error);
      }
    };
    mediaRecorder!.stop();
  });
}

/** 停止录音并转写，返回原文文本（不脱敏、不增强）。 */
export function stopRecordingAndTranscribe(apiFetch: ApiFetch): Promise<string> {
  return doStopAndTranscribe(apiFetch);
}
