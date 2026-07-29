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
        // If session_id is set, fetch the chat history too
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

  const formatDuration = (ms: number) => {
    if (!ms) return '—';
    if (ms < 1000) return `${ms}ms`;
    if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
    return `${(ms / 60000).toFixed(1)}m`;
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
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
            <div>
              <p className="text-xs text-[var(--text-secondary)]">类型</p>
              <p className="text-[var(--text-primary)] mt-0.5">{run.type || '—'}</p>
            </div>
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
              <p className="text-[var(--text-primary)] mt-0.5">{formatDuration(run.duration_ms)}</p>
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
              <p className="text-xs text-[var(--text-secondary)]">进度</p>
              <p className="text-[var(--text-primary)] mt-0.5">
                {run.progress?.percent != null ? `${run.progress.percent}%` : '—'}
              </p>
            </div>
            <div>
              <p className="text-xs text-[var(--text-secondary)]">状态</p>
              <p className="text-[var(--text-primary)] mt-0.5">{STATUS_LABELS[run.status]?.label || run.status}</p>
            </div>
          </div>
        </div>

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

        {/* Params */}
        {run.params && Object.keys(run.params).length > 0 && (
          <div className="glass p-4 mb-6" data-testid="run-params">
            <h3 className="text-sm font-semibold text-[var(--text-primary)] mb-3">⚙ 参数</h3>
            <pre className="text-xs text-[var(--text-secondary)] bg-black/30 rounded-lg p-3 max-h-48 overflow-y-auto whitespace-pre-wrap">
              {JSON.stringify(run.params, null, 2)}
            </pre>
          </div>
        )}

        {/* Chat history from session */}
        {chatLoading ? (
          <div className="text-center py-6 text-[var(--text-secondary)]">加载会话记录...</div>
        ) : chatMessages.length > 0 && (
          <div className="glass p-4" data-testid="run-chat">
            <h3 className="text-sm font-semibold text-[var(--text-primary)] mb-3">💬 执行会话 ({chatMessages.length} 条消息)</h3>
            <div className="space-y-3 max-h-[600px] overflow-y-auto">
              {chatMessages.map((msg, i) => (
                <div key={i} className="border-l-2 border-[var(--border-glass)] pl-3">
                  <p className="text-xs text-[var(--text-secondary)] mb-1">
                    {msg.role || (msg.is_user ? 'user' : 'assistant')}
                  </p>
                  <p className="text-sm text-[var(--text-primary)] whitespace-pre-wrap">
                    {typeof msg.content === 'string' ? msg.content : JSON.stringify(msg.content, null, 2)}
                  </p>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </AppLayout>
  );
}