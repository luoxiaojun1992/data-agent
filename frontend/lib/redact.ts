// lib/redact.ts — Chat 输入框脱敏 API 封装（SPEC-093）。
//
// 脱敏能力由后端 Presidio 服务提供（POST /api/v1/chat/redact，权限与增强
// 提示词相同）。本模块只负责调用与开关状态持久化，不含任何模型加载逻辑。

export const REDACT_LOCAL_STORAGE_KEY = 'chat_auto_redact'; // D4

/** 读取自动脱敏开关（localStorage，默认关闭）。 */
export function loadRedactAuto(): boolean {
  try {
    return localStorage.getItem(REDACT_LOCAL_STORAGE_KEY) === '1';
  } catch {
    return false;
  }
}

/** 持久化自动脱敏开关。 */
export function saveRedactAuto(on: boolean): void {
  try {
    localStorage.setItem(REDACT_LOCAL_STORAGE_KEY, on ? '1' : '0');
  } catch {
    // ignore (private mode)
  }
}

/**
 * 调用后端脱敏 API。失败时抛错——由调用方展示错误提示（手动脱敏保留原文；
 * 自动脱敏中止发送）。
 */
export async function redactText(
  apiFetch: (url: string, init?: RequestInit) => Promise<Response>,
  text: string,
): Promise<string> {
  const res = await apiFetch('/chat/redact', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text }),
  });
  if (!res.ok) {
    let msg = `脱敏失败 (${res.status})`;
    try {
      const data = await res.json();
      if (data?.error) msg = data.error;
    } catch {
      // keep default message
    }
    throw new Error(msg);
  }
  const data = await res.json();
  return data.redacted ?? text;
}
