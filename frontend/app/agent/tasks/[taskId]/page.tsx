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
  const { apiFetch } = useAuth();

  const [task, setTask] = useState<TaskDef | null>(null);
  const [runs, setRuns] = useState<TaskRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    if (!taskId) return;
    loadData();
  }, [taskId]);

  const loadData = async () => {
    setLoading(true);
    try {
      const [taskRes, runsRes] = await Promise.all([
        apiFetch(`/tasks/${taskId}`),
        apiFetch(`/tasks/${taskId}/runs?page_size=50`),
      ]);
      if (taskRes.ok) setTask(await taskRes.json());
      if (runsRes.ok) {
        const data = await runsRes.json();
        setRuns(data.runs || []);
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
        await loadData();
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

        {loading ? (
          <div className="text-center py-12 text-[var(--text-secondary)]">加载中...</div>
        ) : runs.length === 0 ? (
          <div className="glass p-12 text-center" data-testid="runs-empty">
            <span className="text-5xl block mb-4">⚡</span>
            <p className="text-lg text-[var(--text-primary)] mb-2">暂无运行记录</p>
            <p className="text-sm text-[var(--text-secondary)]">点击「▶ 手动运行」触发一次执行</p>
          </div>
        ) : (
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
        )}
      </div>
    </AppLayout>
  );
}