'use client';

import React, { useState, useEffect } from 'react';
import { useRouter, useParams } from 'next/navigation';
import AppLayout from '../../../providers';
import { useAuth } from '@/lib/api';

interface TaskRun {
  run_id: string;
  task_id: string;
  user_id: string;
  status: string;
  type?: string;
  progress?: { current_step: number; total_steps: number; message: string; percent: number };
  retry_count: number;
  max_retries: number;
  duration_ms: number;
  started_at?: string;
  completed_at?: string;
  created_at: string;
  error?: string;
  result?: { content: string; status: string };
  session_id?: string;
  params?: Record<string, any>;
}

const STATUS_LABELS: Record<string, { label: string; cls: string }> = {
  pending: { label: '等待中', cls: 'text-amber-400 bg-amber-400/10' },
  running: { label: '运行中', cls: 'text-blue-400 bg-blue-400/10' },
  completed: { label: '已完成', cls: 'text-emerald-400 bg-emerald-400/10' },
  failed: { label: '失败', cls: 'text-red-400 bg-red-400/10' },
  cancelled: { label: '已取消', cls: 'text-gray-400 bg-gray-400/10' },
  queued: { label: '排队中', cls: 'text-amber-400 bg-amber-400/10' },
};

export default function RunDetailPage() {
  const router = useRouter();
  const params = useParams<{ runId: string }>();
  const runId = params.runId;
  const { apiFetch } = useAuth();

  const [run, setRun] = useState<TaskRun | null>(null);
  const [task, setTask] = useState<{ title?: string; description?: string; type?: string } | null>(null);
  const [loading, setLoading] = useState(true);
  const [chatMessages, setChatMessages] = useState<any[]>([]);
  const [chatLoading, setChatLoading] = useState(false);

  useEffect(() => {
    if (!runId) return;
    loadData();
  }, [runId]);

  const loadData = async () => {
    setLoading(true);
    try {
      const res = await apiFetch(`/runs/${runId}`);
      if (res.ok) {
        const data = await res.json();
        setRun(data);
        // Fetch task info for the "任务详情" section.
        if (data.task_id) {
          try {
            const taskRes = await apiFetch(`/tasks/${data.task_id}`);
            if (taskRes.ok) {
              const t = await taskRes.json();
              setTask({ title: t.title, description: t.description, type: t.type });
            }
          } catch (e) {
            console.error('[run-detail] task fetch failed:', e);
          }
        }
        // Fetch chat history.
        if (data.session_id) {
          setChatLoading(true);
          try {
            const chatRes = await apiFetch(`/sessions/${data.session_id}/messages`);
            if (chatRes.ok) {
              const chatData = await chatRes.json();
              setChatMessages(chatData.messages || []);
            }
          } catch (e) {
            console.error('[run-detail] chat load failed:', e);
          } finally {
            setChatLoading(false);
          }
        }
      }
    } catch (e) {
      console.error('[run-detail] load failed:', e);
    } finally {
      setLoading(false);
    }
  };

  // Compute duration from completed_at - created_at (more accurate than
  // duration_ms which may not be set if worker died mid-run).
  const computeDuration = (): string => {
    if (!run) return '—';
    const end = run.completed_at ? new Date(run.completed_at).getTime() : Date.now();
    const start = new Date(run.created_at).getTime();
    const ms = end - start;
    if (ms < 1000) return `${ms}ms`;
    if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
    if (ms < 3600000) return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`;
    return `${(ms / 3600000).toFixed(1)}h`;
  };

  // Normalize chat messages: render text, tool_call, and tool_result events
  // from the flat ADK session event format.
  //
  // Event types:
  //   { type: "text", role: "user"|"data_agent", content: "..." }
  //   { type: "tool_call", name: "knowledge_search", args: { ... } }
  //   { type: "tool_result", name: "knowledge_search", result: { ... } }
  const normalizeChatMessages = (raw: any[]): { role: string; text: string }[] => {
    const out: { role: string; text: string }[] = [];
    for (const ev of raw) {
      if (ev.type === 'text') {
        const text = (ev.content || '').trim();
        if (!text) continue;
        const role = ev.role === 'user' ? 'user' : 'assistant';
        out.push({ role, text });
      } else if (ev.type === 'tool_call') {
        out.push({
          role: 'tool_call',
          text: `🔧 ${ev.name}\n${JSON.stringify(ev.args, null, 2)}`,
        });
      } else if (ev.type === 'tool_result') {
        out.push({
          role: 'tool_result',
          text: `📋 ${ev.name}\n${JSON.stringify(ev.result, null, 2)}`,
        });
      }
    }
    return out;
  };

  const statusPill = (s: string) => {
    const m = STATUS_LABELS[s] || { label: s, cls: 'text-[var(--text-secondary)] bg-[var(--glass-bg)]' };
    return <span className={`text-xs px-2.5 py-1 rounded-full ${m.cls}`}>{m.label}</span>;
  };

  if (loading) {
    return (
      <AppLayout>
        <div className="animate-fade-in text-center py-12 text-[var(--text-secondary)]">加载中...</div>
      </AppLayout>
    );
  }

  if (!run) {
    return (
      <AppLayout>
        <div className="animate-fade-in">
          <button onClick={() => router.back()} className="text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)] mb-4">← 返回</button>
          <div className="glass p-12 text-center">
            <p className="text-lg text-[var(--text-primary)]">Run 不存在或已删除</p>
          </div>
        </div>
      </AppLayout>
    );
  }

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-4 flex items-center gap-3">
          <button onClick={() => router.push(`/agent/tasks/${run.task_id}`)}
            className="text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
            data-testid="run-back-btn">← 返回 Runs 列表</button>
        </div>

        <div className="mb-6 flex items-center justify-between">
          <div>
            <h2 className="text-2xl font-bold text-[var(--text-primary)] flex items-center gap-3">
              <span>Run</span>
              {statusPill(run.status)}
            </h2>
            <p className="text-sm text-[var(--text-secondary)] mt-1 font-mono">{run.run_id}</p>
          </div>
        </div>

        {/* Run metadata */}
        <div className="glass p-4 mb-6" data-testid="run-meta">
          <div className="grid grid-cols-2 md:grid-cols-3 gap-4 text-sm">
            <div>
              <p className="text-xs text-[var(--text-secondary)]">创建时间</p>
              <p className="text-[var(--text-primary)] mt-0.5">{new Date(run.created_at).toLocaleString()}</p>
            </div>
            <div>
              <p className="text-xs text-[var(--text-secondary)]">完成时间</p>
              <p className="text-[var(--text-primary)] mt-0.5">{run.completed_at ? new Date(run.completed_at).toLocaleString() : '—'}</p>
            </div>
            <div>
              <p className="text-xs text-[var(--text-secondary)]">耗时</p>
              <p className="text-[var(--text-primary)] mt-0.5">{computeDuration()}</p>
            </div>
            <div>
              <p className="text-xs text-[var(--text-secondary)]">重试次数</p>
              <p className="text-[var(--text-primary)] mt-0.5">{run.retry_count} / {run.max_retries}</p>
            </div>
            <div>
              <p className="text-xs text-[var(--text-secondary)]">会话 ID</p>
              <p className="text-[var(--text-primary)] mt-0.5 font-mono text-xs">
                {run.session_id ? run.session_id.slice(0, 24) : '—'}
              </p>
            </div>
            <div>
              <p className="text-xs text-[var(--text-secondary)]">状态</p>
              <p className="text-[var(--text-primary)] mt-0.5">{STATUS_LABELS[run.status]?.label || run.status}</p>
            </div>
          </div>
        </div>

        {/* 任务详情 — from the linked Task definition (with params fallback) */}
        {(() => {
          const title = task?.title || run.params?.title;
          const description = task?.description || run.params?.description;
          if (!title && !description) return null;
          return (
            <div className="glass p-4 mb-6" data-testid="run-task-detail">
              <h3 className="text-sm font-semibold text-[var(--text-primary)] mb-3">📝 任务详情</h3>
              {title && (
                <div className="mb-2">
                  <p className="text-xs text-[var(--text-secondary)]">标题</p>
                  <p className="text-sm text-[var(--text-primary)] mt-0.5">{title}</p>
                </div>
              )}
              {description && (
                <div>
                  <p className="text-xs text-[var(--text-secondary)]">描述</p>
                  <p className="text-sm text-[var(--text-primary)] mt-0.5 whitespace-pre-wrap">{description}</p>
                </div>
              )}
            </div>
          );
        })()}

        {/* Result */}
        {run.result && (
          <div className="glass p-4 mb-6" data-testid="run-result">
            <h3 className="text-sm font-semibold text-[var(--text-primary)] mb-3">📊 执行结果</h3>
            <pre className="text-sm text-[var(--text-primary)] bg-black/30 rounded-lg p-4 max-h-96 overflow-y-auto whitespace-pre-wrap">
              {run.result.content || JSON.stringify(run.result, null, 2)}
            </pre>
          </div>
        )}

        {/* Error */}
        {run.error && (
          <div className="glass p-4 mb-6 border border-red-400/30" data-testid="run-error">
            <h3 className="text-sm font-semibold text-red-400 mb-3">⚠ 错误信息</h3>
            <pre className="text-sm text-red-300 bg-black/30 rounded-lg p-4 max-h-64 overflow-y-auto whitespace-pre-wrap">
              {run.error}
            </pre>
          </div>
        )}

        {/* Chat history from session */}
        {chatLoading ? (
          <div className="text-center py-6 text-[var(--text-secondary)]">加载会话记录...</div>
        ) : (() => {
          const msgs = normalizeChatMessages(chatMessages);
          if (msgs.length === 0) return null;
          return (
            <div className="glass p-4" data-testid="run-chat">
              <h3 className="text-sm font-semibold text-[var(--text-primary)] mb-3">💬 执行会话 ({msgs.length} 条消息)</h3>
              <div className="space-y-3 max-h-[600px] overflow-y-auto">
                {msgs.map((msg, i) => {
                    if (msg.role === 'tool_call' || msg.role === 'tool_result') {
                      return (
                        <div key={i} className="flex justify-start">
                          <div className="max-w-[90%] rounded-lg px-3 py-2 bg-[var(--glass-bg)] border border-amber-400/20">
                            <p className="text-[10px] text-amber-400 mb-1 font-mono">
                              {msg.role === 'tool_call' ? '🔧 Tool Call' : '📋 Tool Result'} · {msg.text.split('\n')[0]}
                            </p>
                            <pre className="text-xs text-[var(--text-secondary)] max-h-48 overflow-y-auto whitespace-pre-wrap font-mono">
                              {msg.text.includes('\n') ? msg.text.substring(msg.text.indexOf('\n') + 1) : ''}
                            </pre>
                          </div>
                        </div>
                      );
                    }
                    return (
                      <div key={i} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                        <div
                          className={`max-w-[80%] rounded-lg px-3 py-2 ${
                            msg.role === 'user'
                              ? 'bg-[var(--accent)]/15 border border-[var(--accent)]/30'
                              : 'bg-[var(--glass-bg)] border border-[var(--border-glass)]'
                          }`}>
                          <p className={`text-[10px] mb-1 ${msg.role === 'user' ? 'text-[var(--accent)]' : 'text-[var(--text-secondary)]'}`}>
                            {msg.role === 'user' ? '👤 User' : '🤖 Assistant'}
                          </p>
                          <p className="text-sm text-[var(--text-primary)] whitespace-pre-wrap">
                            {msg.text}
                          </p>
                        </div>
                      </div>
                    );
                  })}
              </div>
            </div>
          );
        })()}
      </div>
    </AppLayout>
  );
}