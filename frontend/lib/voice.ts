// SPEC-095: Chat 语音输入（whisper.wasm 纯 CPU 本地转写）
// 音频全程本地处理、不上传。封装 whisper.cpp 官方 wasm 产物（/whisper/libmain.js）。

export type VoicePhase = 'idle' | 'loading-model' | 'recording' | 'transcribing';

const WASM_SCRIPT_URL = '/whisper/libmain.js';
const MODEL_URL = '/models/whisper/ggml-tiny.bin';
const MODEL_FS_NAME = 'whisper.bin';

interface WhisperModule {
  init(path: string): number;
  free(index: number): void;
  full_default(index: number, audio: Float32Array, lang: string, nthreads: number, translate: boolean): number;
  FS_createDataFile(parent: string, name: string, data: Uint8Array, canRead: boolean, canWrite: boolean): void;
  FS_unlink(name: string): void;
  [k: string]: unknown;
}

let moduleInstance: WhisperModule | null = null;
let whisperIndex = 0;
let modelPromise: Promise<void> | null = null;

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const s = document.createElement('script');
    s.src = src;
    s.async = true;
    s.onload = () => resolve();
    s.onerror = () => reject(new Error('whisper 引擎加载失败'));
    document.head.appendChild(s);
  });
}

// 确保 whisper 引擎 + 模型已就绪（幂等，只加载一次）。
export async function ensureWhisper(): Promise<void> {
  if (whisperIndex > 0) return;
  if (modelPromise) return modelPromise;

  modelPromise = (async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const g = window as any;
    g.Module = g.Module || {};
    // stdout 回调在 transcribe() 时动态绑定，此处先占位避免 undefined
    g.Module.print = g.Module.print || (() => {});
    g.Module.printErr = g.Module.printErr || (() => {});

    await loadScript(WASM_SCRIPT_URL);
    moduleInstance = (window as unknown as { Module: WhisperModule }).Module;

    const resp = await fetch(MODEL_URL);
    if (!resp.ok) throw new Error('模型加载失败');
    const buf = new Uint8Array(await resp.arrayBuffer());
    moduleInstance.FS_createDataFile('/', MODEL_FS_NAME, buf, true, true);

    whisperIndex = moduleInstance.init(MODEL_FS_NAME);
    if (!whisperIndex) throw new Error('whisper 模型初始化失败');
  })();

  return modelPromise;
}

// ── 录音 ──
let mediaRecorder: MediaRecorder | null = null;
let mediaStream: MediaStream | null = null;
let audioChunks: Blob[] = [];
let audioCtx: AudioContext | null = null;

function getAudioContext(): AudioContext {
  if (!audioCtx) {
    const Ctor = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    audioCtx = new Ctor({ sampleRate: 16000 });
  }
  return audioCtx;
}

export async function startRecording(): Promise<void> {
  await ensureWhisper(); // 录音前预加载引擎，减少停止后的等待
  mediaStream = await navigator.mediaDevices.getUserMedia({
    audio: {
      channelCount: 1,
      echoCancellation: true,
      autoGainControl: true,
      noiseSuppression: true,
    },
    video: false,
  });
  mediaRecorder = new MediaRecorder(mediaStream);
  audioChunks = [];
  mediaRecorder.ondataavailable = (e) => {
    if (e.data.size > 0) audioChunks.push(e.data);
  };
  mediaRecorder.start();
}

// 停止录音并把音频重采样为 16kHz 单声道 Float32Array。
export async function stopRecording(): Promise<Float32Array> {
  return new Promise<Float32Array>((resolve, reject) => {
    if (!mediaRecorder) {
      reject(new Error('未在录音'));
      return;
    }
    mediaRecorder.onstop = async () => {
      mediaStream?.getTracks().forEach((t) => t.stop());
      mediaStream = null;

      // 不带 type，让 decodeAudioData 自行嗅探容器格式（Chrome=webm、Safari=mp4）。
      const blob = new Blob(audioChunks);
      audioChunks = [];
      const arr = await blob.arrayBuffer();

      try {
        const ctx = getAudioContext();
        const audioBuffer = await ctx.decodeAudioData(arr);
        // 重采样到 whisper 要求的 16kHz 单声道（OfflineAudioContext 自动重采样）。
        const offline = new OfflineAudioContext(
          1,
          Math.ceil(audioBuffer.duration * 16000),
          16000,
        );
        const src = offline.createBufferSource();
        src.buffer = audioBuffer;
        src.connect(offline.destination);
        src.start(0);
        const rendered = await offline.startRendering();
        resolve(rendered.getChannelData(0));
      } catch (e) {
        reject(e as Error);
      }
    };
    mediaRecorder.stop();
  });
}

// ── 转写 ──

// 解析 stdout 输出：提取 "[00:00:00.000 --> 00:00:05.000]  文本" 中的文本。
function parseTranscript(raw: string): string {
  const texts: string[] = [];
  for (const line of raw.split('\n')) {
    const m = line.match(/\[[\d:.]+\s*-->\s*[\d:.]+\]\s*(.*)/);
    if (m && m[1] && m[1].trim()) texts.push(m[1].trim());
  }
  return texts.join('');
}

// 本地转写，返回纯文本。lang 传 'auto' 自动检测语言（中文/英文均适用）。
export async function transcribe(audio: Float32Array, lang = 'auto'): Promise<string> {
  await ensureWhisper();
  if (!moduleInstance || !whisperIndex) throw new Error('whisper 未就绪');

  const g = window as unknown as { Module?: { print?: (t: string) => void; printErr?: (t: string) => void } };
  const prevPrint = g.Module?.print;

  return new Promise<string>((resolve, reject) => {
    const lines: string[] = [];
    let settled = false;

    const finish = (fn: () => void) => {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      fn();
    };

    // 转写完成标志：whisper_print_timings 输出（whisper_full 结束后由引擎打印）。
    g.Module!.print = (t: string) => {
      lines.push(t);
      if (t.includes('whisper_print_timings') || t.includes('total time')) {
        finish(() => resolve(parseTranscript(lines.join('\n'))));
      }
    };

    const timeout = setTimeout(() => {
      finish(() => reject(new Error('转写超时')));
    }, 120000);

    try {
      const ret = moduleInstance!.full_default(whisperIndex, audio, lang, 4, false);
      if (ret !== 0) {
        finish(() => reject(new Error('转写失败：' + ret)));
      }
    } catch (e) {
      finish(() => reject(e as Error));
    }
  }).finally(() => {
    if (g.Module) g.Module.print = prevPrint;
  });
}
