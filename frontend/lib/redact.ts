// lib/redact.ts — Chat 输入框本地脱敏核心模块（SPEC-093）。
//
// 职责：
//  1. 单例加载 openai/privacy-filter（transformers.js + onnxruntime-web）
//  2. 设备降级：webgpu → wasm（D2 拍板）
//  3. 推理 + span 替换（类别占位 + 尖括号，与 Presidio 风格一致，D1 拍板）
//
// 模型加载（D7 修订 2026-09-09）：**不进代码库、不预下载**，运行时由
// transformers.js 直接从 HuggingFace 下载加载（浏览器 Cache API 自动缓存，
// 后续加载命中缓存）。

import type { TokenClassificationPipeline } from '@huggingface/transformers';

// 模型生命周期状态（不含「推理中」——推理进行中由调用方的弹窗状态管理）。
export type RedactStatus = 'loading' | 'ready' | 'failed';

export const REDACT_LOCAL_STORAGE_KEY = 'chat_auto_redact'; // D4

type StatusListener = (s: RedactStatus) => void;

let status: RedactStatus = 'loading';
let classifierPromise: Promise<TokenClassificationPipeline> | null = null;
const listeners = new Set<StatusListener>();

export function getRedactStatus(): RedactStatus {
  return status;
}

/** 订阅模型状态变化；返回取消订阅函数。 */
export function subscribeRedactStatus(fn: StatusListener): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

function setStatus(s: RedactStatus) {
  status = s;
  listeners.forEach((fn) => fn(s));
}

/** 实体名 → 占位符：类别大写 + 尖括号（D1，与 Presidio `<PII>` 同风格）。 */
export function placeholderFor(entity: string): string {
  const normalized = String(entity || '').replace(/^[BIS]-/, '');
  return `<${normalized.toUpperCase()}>`;
}

/** 模型分类实体 span（aggregation 后）。 */
export interface RedactSpan {
  start: number;
  end: number;
  entity: string;
}

/**
 * 纯函数：按字符偏移把 spans 就地替换为占位符。倒序替换避免偏移失效；
 * 越界 span 自动裁剪；空 span 跳过。输出为「原文被替换」的脱敏文本。
 */
export function applySpans(text: string, spans: RedactSpan[]): string {
  if (!text) return text;
  const sorted = [...spans].sort((a, b) => b.start - a.start);
  let result = text;
  for (const s of sorted) {
    const start = Math.max(0, Math.min(text.length, s.start));
    const end = Math.max(start, Math.min(text.length, s.end));
    if (end <= start) continue;
    result = result.slice(0, start) + placeholderFor(s.entity) + result.slice(end);
  }
  return result;
}

/** 懒加载 transformers.js（动态 import，避免 SSR 副作用）。
 *  next.config.js 已 alias 到浏览器入口 transformers.web.js，
 *  避免 webpack 走 exports 的 node condition 拖入 onnxruntime-node 二进制。 */
async function importTransformers() {
  return import('@huggingface/transformers');
}

/**
 * 创建 token-classification pipeline。webgpu 失败自动降级 wasm（D2）；
 * 两者均失败抛错 → 状态 failed。
 */
async function createClassifier(): Promise<TokenClassificationPipeline> {
  const { pipeline, env } = await importTransformers();
  // D7 修订：运行时从 HF 下载加载；浏览器 Cache API 缓存模型文件（显式开启，
  // v4 默认即 true）——首次加载后同源请求命中缓存，不再重复下载。
  env.useBrowserCache = true;
  const options = { dtype: 'q4f16' as const };
  try {
    return await pipeline('token-classification', 'openai/privacy-filter', {
      ...options,
      device: 'webgpu',
    });
  } catch (err) {
    console.warn('[redact] webgpu 初始化失败，降级 wasm:', err);
    return await pipeline('token-classification', 'openai/privacy-filter', {
      ...options,
      device: 'wasm',
    });
  }
}

/**
 * 启动模型加载（幂等）。页面 mount 时调用；加载完成后状态变 ready / failed。
 * 返回是否就绪。
 */
export async function loadRedactor(): Promise<boolean> {
  if (classifierPromise) {
    try {
      await classifierPromise;
      return true;
    } catch {
      return false;
    }
  }
  setStatus('loading');
  classifierPromise = createClassifier();
  try {
    await classifierPromise;
    setStatus('ready');
    return true;
  } catch (err) {
    classifierPromise = null;
    setStatus('failed');
    console.error('[redact] 模型加载失败:', err);
    return false;
  }
}

/**
 * 对文本执行本地脱敏推理。要求模型已 ready；推理抛错时向上传播
 * （调用方负责报错，第 5/6 条拍板）。
 */
export async function redactText(text: string): Promise<string> {
  if (!classifierPromise) {
    throw new Error('脱敏模型未加载');
  }
  const classifier = await classifierPromise;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const output = (await classifier(text, {
    aggregation_strategy: 'simple',
  })) as any[];

  // v4: simple 聚合输出用 entity_group（raw 输出用 entity），兼容两者。
  const spans: RedactSpan[] = (output || []).map((o) => ({
    start: o?.start ?? 0,
    end: o?.end ?? 0,
    entity: String(o?.entity_group ?? o?.entity ?? ''),
  }));
  return applySpans(text, spans);
}
