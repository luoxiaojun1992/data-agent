'use client';

import React, { useState, useEffect } from 'react';
import { useRouter, useParams } from 'next/navigation';
import { useTranslations } from 'next-intl';
import AppLayout from '../../../providers';
import Pagination from '../../../components/Pagination';
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
  model_id?: string;
  cron_expr?: string;
  created_at: string;
}

export default function TaskRunsPage() {
  const t = useTranslations('agentTask');
  const router = useRouter();
  const params = useParams<{ taskId: string }>();
  const taskId = params.taskId;
  const { apiFetch, auth } = useAuth();

  const [task, setTask] = useState<TaskDef | null>(null);
  const [runs, setRuns] = useState<TaskRun[]>([]);
  const [modelName, setModelName] = useState<string>('');
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
      const urlParams = new URLSearchParams({ page: String(p), page_size: String(pageSize) });
      if (status !== 'all') urlParams.set('status', status);
      const [taskRes, runsRes, modelsRes] = await Promise.all([
        apiFetch(`/tasks/${taskId}`),
        apiFetch(`/tasks/${taskId}/runs?${urlParams.toString()}`),
        apiFetch(`/models/list`),
      ]);
      if (taskRes.ok) {
        const taskData = await taskRes.json();
        setTask(taskData);
        // Resolve model_id → model name once we have both.
        if (modelsRes.ok && taskData.model_id) {
          const data = await modelsRes.json();
          const list: { id: string; name: string }[] = data.models || [];
          setModelName(list.find((m) => m.id === taskData.model_id)?.name || '');
        }
      }
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

  // SPEC-082 §5.8: cancel a run execution (only pending/queued/running).
  const cancelRun = async (runId: string) => {
    if (!window.confirm(t('cancelConfirm'))) return;
    try {
      const res = await apiFetch(`/task-runs/${runId}/cancel`, { method: 'PUT' });
      if (res.ok) {
        await loadData(page, statusFilter);
      }
    } catch (e) {
      console.error('[runs-list] cancel failed:', e);
    }
  };

  const formatDuration = (ms: number) => {
    if (!ms) return '—';
    if (ms < 1000) return `${ms}ms`;
    if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
    return `${(ms / 60000).toFixed(1)}m`;
  };

  const STATUS_LABELS: Record<string, { label: string; cls: string }> = {
    pending: { label: t('statusPending'), cls: 'text-amber-400 bg-amber-400/10' },
    running: { label: t('statusRunning'), cls: 'text-blue-400 bg-blue-400/10' },
    completed: { label: t('statusCompleted'), cls: 'text-emerald-400 bg-emerald-400/10' },
    failed: { label: t('statusFailed'), cls: 'text-red-400 bg-red-400/10' },
    cancelled: { label: t('statusCancelled'), cls: 'text-gray-400 bg-gray-400/10' },
    queued: { label: t('statusQueued'), cls: 'text-amber-400 bg-amber-400/10' },
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
            data-testid="runs-back-btn">{t('backToList')}</button>
        </div>

        <div className="mb-6 flex items-center justify-between">
          <div>
            <h2 className="text-2xl font-bold text-[var(--text-primary)]">
              {task?.title || task?.type || taskId.slice(0, 16)}
            </h2>
            <p className="text-sm text-[var(--text-secondary)] mt-1">
              {t('taskIdLabel')}: {taskId}
              {task?.cron_expr && <span className="ml-3">{t('cronLabel')}: {task.cron_expr}</span>}
              {task?.model_id && <span className="ml-3">{t('modelLabel')}: {modelName || task.model_id}</span>}
            </p>
          </div>
          <button onClick={triggerRun} disabled={creating}
            className="px-4 py-2 bg-[var(--accent)] text-white rounded-xl text-sm font-medium hover:opacity-90 disabled:opacity-40"
            data-testid="runs-trigger-btn">
            {creating ? t('creating') : t('triggerRun')}
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
              {f === 'all' ? t('all') : STATUS_LABELS[f]?.label || f}
            </button>
          ))}
        </div>

        {loading ? (
          <div className="text-center py-12 text-[var(--text-secondary)]">{t('loading')}</div>
        ) : runs.length === 0 ? (
          <div className="glass p-12 text-center" data-testid="runs-empty">
            <span className="text-5xl block mb-4">⚡</span>
            <p className="text-lg text-[var(--text-primary)] mb-2">{t('empty')}</p>
            <p className="text-sm text-[var(--text-secondary)]">{t('emptyHint')}</p>
          </div>
        ) : (
          <>
            <div className="space-y-3" data-testid="runs-list">
            {runs.map((run, idx) => (
              <button key={run.run_id} onClick={() => openRun(run.run_id)}
                className="glass w-full text-left p-4 hover:bg-[var(--surface-5)] transition-colors"
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
                      {t('createdLabel')}: {new Date(run.created_at).toLocaleString()}
                      {run.started_at && ` · ${t('startedLabel')}: ${new Date(run.started_at).toLocaleString()}`}
                      {run.completed_at && ` · ${t('completedLabel')}: ${new Date(run.completed_at).toLocaleString()}`}
                      {run.duration_ms > 0 && ` · ${t('durationLabel')}: ${formatDuration(run.duration_ms)}`}
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
                    {['pending', 'queued', 'running'].includes(run.status) && (
                      <button onClick={(e) => { e.stopPropagation(); cancelRun(run.run_id); }}
                        className="text-xs text-red-400 hover:text-red-300"
                        data-testid={`run-cancel-${run.run_id}`}>{t('cancel')}</button>
                    )}
                    <span className="text-xs text-[var(--text-secondary)]">▶</span>
                  </div>
                </div>
              </button>
            ))}
          </div>

          {/* Pagination */}
          <Pagination page={page} total={total} pageSize={pageSize} onChange={setPage} testIdPrefix="runs" />
          </>
        )}
      </div>
    </AppLayout>
  );
}
