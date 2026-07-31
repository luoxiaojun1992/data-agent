'use client';

import React, { useState, useEffect } from 'react';
import { useRouter, useParams } from 'next/navigation';
import AppLayout from '../../../providers';
import { useAuth } from '@/lib/api';

interface TaskRun {
  run_id: string;
  task_id: string;
  status: string;
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
}

interface TaskDef {
  task_id: string;
  title?: string;
  type?: string;
  status: string;
  cron_expr?: string;
  created_at: string;
}

const STATUS_LABELS: Record<string, { label: string; cls: string }> = {
  pending: { label: '等待中', cls: 'text-amber-400 bg-amber-400/10' },
  running: { label: '运行中', cls: 'text-blue-400 bg-blue-400/10' },
  completed: { label: '已完成', cls: 'text-emerald-400 bg-emerald-400/10' },
  failed: { label: '失败', cls: 'text-red-400 bg-red-400/10' },
  cancelled: { label: '已取消', cls: 'text-gray-400 bg-gray-400/10' },
  queued: { label: '排队中', cls: 'text-amber-400 bg-amber-400/10' },
};

export default function TaskRunsPage() {
  const router = useRouter();
  const params = useParams<{ taskId: string }>();
  const taskId = params.taskId;
  const { apiFetch, auth } = useAuth();

  const [task, setTask] = useState<TaskDef | null>(null);
  const [runs, setRuns] = useState<TaskRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [page, setPage] = useState(1);
  const [pageSize] = useState(20);
  const [total, setTotal] = useState(0);

  useEffect(() => {
    if (!taskId || !auth.hydrated || !auth.token) return;
    setPage(1); // reset on task switch / filter change
    loadData(1, statusFilter);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [taskId, statusFilter, auth.hydrated, auth.token]);

  useEffect(() => {
    if (!taskId || !auth.hydrated || !auth.token) return;
    loadData(page, statusFilter);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, auth.hydrated, auth.token]);

  const loadData = async (p: number, status: string) => {
    setLoading(true);
    try {
      const params = new URLSearchParams({ page: String(p), page_size: String(pageSize) });
      if (status !== 'all') params.set('status', status);
      const [taskRes, runsRes] = await Promise.all([
        apiFetch(`/tasks/${taskId}`),
        apiFetch(`/tasks/${taskId}/runs?${params.toString()}`),
      ]);
      if (taskRes.ok) setTask(await taskRes.json());
      if (runsRes.ok) {
        const data = await runsRes.json();
        setRuns(data.runs || []);
        setTotal(typeof data.total === 'number' ? data.total : (data.runs || []).length);
      }
    } catch (e) {
      console.error('[runs-list] load failed:', e);
    } finally {
      setLoading(false);
    }
  };

  const triggerRun = async () => {
    setCreating(true);
    try {
      const res = await apiFetch(`/tasks/${taskId}/run`, { method: 'POST' });
      if (res.ok) {
        await loadData(page, statusFilter);
      }
    } catch (e) {
      console.error('[runs-list] trigger failed:', e);
    } finally {
      setCreating(false);
    }
  };

  const openRun = (runId: string) => {
    router.push(`/agent/runs/${runId}`);
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

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-6 flex items-center gap-3">
          <button onClick={() => router.push('/agent')}
            className="text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
            data-testid="runs-back-btn">← 返回任务列表</button>
        </div>

        <div className="mb-6 flex items-center justify-between">
          <div>
            <h2 className="text-2xl font-bold text-[var(--text-primary)]">
              {task?.title || task?.type || taskId.slice(0, 16)}
            </h2>
            <p className="text-sm text-[var(--text-secondary)] mt-1">
              任务 ID: {taskId}
              {task?.cron_expr && <span className="ml-3">⏱ 定时: {task.cron_expr}</span>}
            </p>
          </div>
          <button onClick={triggerRun} disabled={creating}
            className="px-4 py-2 bg-[var(--accent)] text-white rounded-xl text-sm font-medium hover:opacity-90 disabled:opacity-40"
            data-testid="runs-trigger-btn">
            {creating ? '创建中...' : '▶ 手动运行'}
          </button>
        </div>

        {/* Status filter */}
        <div className="flex gap-2 mb-4" data-testid="runs-status-filter">
          {['all', 'pending', 'running', 'completed', 'failed', 'cancelled'].map(f => (
            <button key={f} onClick={() => setStatusFilter(f)}
              className={`px-3 py-1 text-xs rounded-full border transition-colors ${
                statusFilter === f
                  ? 'border-[var(--accent)] text-[var(--accent)] bg-[var(--accent)]/10'
                  : 'border-[var(--border-glass)] text-[var(--text-secondary)]'
              }`}
              data-testid={`runs-filter-${f}`}>
              {f === 'all' ? '全部' : STATUS_LABELS[f]?.label || f}
            </button>
          ))}
        </div>

        {loading ? (
          <div className="text-center py-12 text-[var(--text-secondary)]">加载中...</div>
        ) : runs.length === 0 ? (
          <div className="glass p-12 text-center" data-testid="runs-empty">
            <span className="text-5xl block mb-4">⚡</span>
            <p className="text-lg text-[var(--text-primary)] mb-2">暂无运行记录</p>
            <p className="text-sm text-[var(--text-secondary)]">点击「▶ 手动运行」触发一次执行</p>
          </div>
        ) : (
          <>
            <div className="space-y-3" data-testid="runs-list">
            {runs.map((run, idx) => (
              <button key={run.run_id} onClick={() => openRun(run.run_id)}
                className="glass w-full text-left p-4 hover:bg-white/5 transition-colors"
                data-testid={`runs-row-${idx}`}>
                <div className="flex items-center justify-between">
                  <div className="flex-1">
                    <div className="flex items-center gap-2 mb-1">
                      <span className="text-sm font-medium text-[var(--text-primary)]">
                        Run #{idx + 1}
                      </span>
                      <span className="text-xs text-[var(--text-secondary)] font-mono">
                        {run.run_id.slice(0, 24)}
                      </span>
                    </div>
                    <div className="text-xs text-[var(--text-secondary)]">
                      创建: {new Date(run.created_at).toLocaleString()}
                      {run.started_at && ` · 开始: ${new Date(run.started_at).toLocaleString()}`}
                      {run.completed_at && ` · 完成: ${new Date(run.completed_at).toLocaleString()}`}
                      {run.duration_ms > 0 && ` · 耗时: ${formatDuration(run.duration_ms)}`}
                    </div>
                    {run.error && (
                      <p className="text-xs text-red-400 mt-1 line-clamp-1">
                        Error: {run.error.slice(0, 100)}
                      </p>
                    )}
                  </div>
                  <div className="flex items-center gap-3 ml-4">
                    {run.progress != null && run.status === 'running' && (
                      <span className="text-xs text-[var(--text-secondary)]">
                        {run.progress.percent}%
                      </span>
                    )}
                    {statusPill(run.status)}
                    <span className="text-xs text-[var(--text-secondary)]">▶</span>
                  </div>
                </div>
              </button>
            ))}
          </div>

          {/* Pagination */}
          <div className="flex items-center justify-between mt-4" data-testid="runs-pagination">
            <span className="text-xs text-[var(--text-secondary)]">
              共 {total} 个 run · 第 {page} / {Math.max(1, Math.ceil(total / pageSize))} 页
            </span>
            <div className="flex gap-2">
              <button onClick={() => setPage(p => Math.max(1, p - 1))} disabled={page <= 1}
                className="px-3 py-1 text-xs rounded-lg border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] disabled:opacity-30"
                data-testid="runs-page-prev">← 上一页</button>
              <button onClick={() => setPage(p => p + 1)} disabled={page * pageSize >= total}
                className="px-3 py-1 text-xs rounded-lg border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] disabled:opacity-30"
                data-testid="runs-page-next">下一页 →</button>
            </div>
          </div>
          </>
        )}
      </div>
    </AppLayout>
  );
}