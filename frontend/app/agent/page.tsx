'use client';

import React, { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import AppLayout from '../providers';
import { useAuth } from '@/lib/api';

const FILTER_LABELS: Record<string, string> = {
  pending: '等待中',
  running: '运行中',
  completed: '已完成',
  failed: '失败',
};

interface AgentTask {
  task_id: string;
  title?: string;
  type?: string;
  status: string; // pending | running | completed | failed | cancelled
  progress?: number;
  created_at: string;
  updated_at?: string;
  cron_expr?: string;
  logs?: string[];
  artifacts?: { name: string; id: string }[];
}

export default function AgentPage() {
  const router = useRouter();
  const { apiFetch } = useAuth();
  const [tasks, setTasks] = useState<AgentTask[]>([]);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [filter, setFilter] = useState<string>('all');
  const [newTask, setNewTask] = useState({ title: '', description: '', async: false, cron: '', cronEnabled: false });

  useEffect(() => { loadTasks(); }, []);

  const loadTasks = async () => {
    try {
      const res = await apiFetch('/tasks');
      const data = await res.json();
      const rawTasks = Array.isArray(data) ? data : (data.tasks || []);
      setTasks(rawTasks.map((t: AgentTask) => ({
        ...t,
        title: t.title || t.type || '',
      })));
    } catch (e) { console.error('[agent] loadTasks failed:', e); }
    finally { setLoading(false); }
  };

  const createTask = async () => {
    if (!newTask.title.trim()) return;
    try {
      const res = await apiFetch('/tasks', {
        method: 'POST',
        body: JSON.stringify({
          title: newTask.title,
          description: newTask.description,
          async: newTask.async,
          cron_expr: newTask.cronEnabled ? newTask.cron : undefined,
        }),
      });
      if (res.ok) {
        await loadTasks();
        setShowModal(false);
        setNewTask({ title: '', description: '', async: false, cron: '', cronEnabled: false });
      }
    } catch (e) { console.error('[agent] loadTasks failed:', e); }
  };

  const cancelTask = async (taskId: string) => {
    await apiFetch(`/tasks/${taskId}/cancel`, { method: 'PUT' });
    await loadTasks();
  };

  const openTask = (taskId: string) => {
    router.push(`/agent/tasks/${taskId}`);
  };

  const statusPill = (s: string) => {
    const map: Record<string, { label: string; cls: string }> = {
      pending: { label: '等待中', cls: 'text-amber-400 bg-amber-400/10' },
      running: { label: '运行中', cls: 'text-blue-400 bg-blue-400/10' },
      completed: { label: '已完成', cls: 'text-emerald-400 bg-emerald-400/10' },
      failed: { label: '失败', cls: 'text-red-400 bg-red-400/10' },
      cancelled: { label: '已取消', cls: 'text-gray-400 bg-gray-400/10' },
    };
    const m = map[s] || { label: s, cls: 'text-[var(--text-secondary)] bg-[var(--glass-bg)]' };
    return <span className={`text-xs px-2.5 py-1 rounded-full ${m.cls}`} data-testid={`task-status-${s}`}>{m.label}</span>;
  };

  const filtered = filter === 'all' ? tasks : tasks.filter(t => t.status === filter);

  return (
    <AppLayout>
      <div className="animate-fade-in">
        {/* Header */}
        <div className="mb-6 flex items-center justify-between" data-testid="agent-page-header">
          <div>
            <h2 className="text-2xl font-bold text-[var(--text-primary)]">Agent 任务</h2>
            <p className="text-sm text-[var(--text-secondary)] mt-1">批量数据分析任务管理与执行</p>
          </div>
          <button onClick={() => setShowModal(true)}
            className="px-4 py-2 bg-[var(--accent)] text-white rounded-xl text-sm font-medium hover:opacity-90"
            data-testid="agent-create-task-btn">+ 新建任务</button>
        </div>


        {/* Filters */}
        <div className="flex gap-2 mb-4" data-testid="agent-task-filters">
          {['all', 'pending', 'running', 'completed', 'failed'].map(f => (
            <button key={f} onClick={() => setFilter(f)}
              className={`px-3 py-1 text-xs rounded-full border transition-colors ${
                filter === f ? 'border-[var(--accent)] text-[var(--accent)] bg-[var(--accent)]/10' : 'border-[var(--border-glass)] text-[var(--text-secondary)]'
              }`}
              data-testid={`agent-filter-${f}`}>{f === 'all' ? '全部' : FILTER_LABELS[f]}</button>
          ))}
        </div>

        {/* Task list */}
        {loading ? (
          <div className="text-center py-12 text-[var(--text-secondary)]" data-testid="agent-loading">加载中...</div>
        ) : filtered.length === 0 ? (
          <div className="glass p-12 text-center" data-testid="agent-empty">
            <span className="text-5xl block mb-4">⚡</span>
            <p className="text-lg text-[var(--text-primary)] mb-2">暂无任务</p>
            <p className="text-sm text-[var(--text-secondary)]">点击「+ 新建任务」创建 Agent 分析任务</p>
          </div>
        ) : (
          <div className="space-y-3" data-testid="agent-task-table">
            {filtered.map((task, idx) => (
              <div key={task.task_id} className="glass" data-testid={`agent-task-row-${idx}`}>
                {/* Row header */}
                <button onClick={() => openTask(task.task_id)}
                  className="w-full text-left p-4 flex items-center justify-between hover:bg-white/5 transition-colors"
                  data-testid={`agent-task-title-${idx}`}>
                  <div>
                    <p className="text-sm font-medium text-[var(--text-primary)]">{task.title || task.type || task.task_id?.slice(0, 12)}</p>
                    <p className="text-xs text-[var(--text-secondary)] mt-1">{new Date(task.created_at).toLocaleString()}</p>
                  </div>
                  <div className="flex items-center gap-2">
                    {statusPill(task.status)}
                    {task.progress != null && task.status === 'running' && (
                      <div className="w-20 h-1.5 bg-[var(--glass-bg)] rounded-full overflow-hidden" data-testid={`task-progress-bar-${idx}`}>
                        <div className="h-full bg-[var(--accent)] rounded-full" style={{ width: `${task.progress}%` }} data-testid={`task-progress-fill-${idx}`} />
                      </div>
                    )}
                    <span className="text-xs text-[var(--text-secondary)]">▶</span>
                  </div>
                </button>
              </div>
            ))}

            {/* Pagination */}
            {filtered.length >= 10 && (
              <div className="flex justify-center gap-2 mt-4" data-testid="agent-task-pagination">
                <span className="text-xs text-[var(--text-secondary)]">第 1 页</span>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Create Task Modal */}
      {showModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center" data-testid="agent-task-modal">
          <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={() => setShowModal(false)} />
          <div className="relative glass p-6 rounded-2xl max-w-lg w-full mx-4">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">新建分析任务</h3>
              <button onClick={() => setShowModal(false)} className="text-[var(--text-secondary)] hover:text-[var(--text-primary)]">✕</button>
            </div>
            <div className="space-y-4">
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">任务标题</label>
                <input type="text" value={newTask.title} onChange={e => setNewTask(p => ({ ...p, title: e.target.value }))}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)]"
                  data-testid="agent-task-title-input" placeholder="例如：销售趋势分析" />
              </div>
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">描述（可选）</label>
                <textarea value={newTask.description} onChange={e => setNewTask(p => ({ ...p, description: e.target.value }))}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)] resize-none"
                  data-testid="agent-task-desc-input" rows={2} placeholder="描述分析目标..." />
              </div>
              <label className="flex items-center gap-2 text-sm text-[var(--text-primary)] cursor-pointer">
                <input type="checkbox" checked={newTask.async} onChange={e => setNewTask(p => ({ ...p, async: e.target.checked }))}
                  data-testid="agent-task-async-toggle" className="rounded" />
                异步执行
              </label>
              <label className="flex items-center gap-2 text-sm text-[var(--text-primary)] cursor-pointer">
                <input type="checkbox" checked={newTask.cronEnabled} onChange={e => setNewTask(p => ({ ...p, cronEnabled: e.target.checked }))}
                  data-testid="agent-task-cron-toggle" className="rounded" />
                设为定时任务
              </label>
              {newTask.cronEnabled && (
                <div data-testid="agent-task-cron-config">
                  <label className="block text-xs text-[var(--text-secondary)] mb-1">调度规则</label>
                  <select value={newTask.cron} onChange={e => setNewTask(p => ({ ...p, cron: e.target.value }))}
                    className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)]"
                    data-testid="agent-task-cron-select">
                    <option value="">选择...</option>
                    <option value="0 8 * * *">每日 8:00</option>
                    <option value="0 9 * * 1">每周一 9:00</option>
                    <option value="0 0 1 * *">每月1号 0:00</option>
                  </select>
                </div>
              )}
              <div className="flex gap-3 pt-2">
                <button onClick={() => setShowModal(false)}
                  className="flex-1 px-4 py-2 text-sm rounded-xl border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">取消</button>
                <button onClick={createTask}
                  className="flex-1 px-4 py-2 text-sm rounded-xl bg-[var(--accent)] text-white hover:opacity-90 disabled:opacity-40"
                  data-testid="agent-task-create-btn" disabled={!newTask.title.trim()}>创建任务</button>
              </div>
            </div>
          </div>
        </div>
      )}
    </AppLayout>
  );
}
