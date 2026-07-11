'use client';

import React, { useState, useEffect } from 'react';
import AppLayout from '../providers';
import { useAuth } from '@/lib/api';

interface AgentTask {
  task_id: string;
  title: string;
  status: string; // pending | running | completed | failed | cancelled
  progress?: number;
  created_at: string;
  updated_at?: string;
  cron_expr?: string;
  logs?: string[];
  artifacts?: { name: string; id: string }[];
}

export default function AgentPage() {
  const { apiFetch } = useAuth();
  const [tasks, setTasks] = useState<AgentTask[]>([]);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [filter, setFilter] = useState<string>('all');
  const [expandedTask, setExpandedTask] = useState<string | null>(null);
  const [newTask, setNewTask] = useState({ title: '', description: '', skills: 'sql_executor', async: false });

  useEffect(() => { loadTasks(); }, []);

  const loadTasks = async () => {
    try {
      const res = await apiFetch('/tasks');
      const data = await res.json();
      setTasks(data.tasks || []);
    } catch { /* ignore */ }
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
          skills: newTask.skills.split(',').map(s => s.trim()).filter(Boolean),
          async: newTask.async,
        }),
      });
      if (res.ok) {
        await loadTasks();
        setShowModal(false);
        setNewTask({ title: '', description: '', skills: 'sql_executor', async: false });
      }
    } catch { /* ignore */ }
  };

  const cancelTask = async (taskId: string) => {
    await apiFetch(`/tasks/${taskId}/cancel`, { method: 'PUT' });
    await loadTasks();
  };

  const toggleExpand = async (taskId: string) => {
    if (expandedTask === taskId) { setExpandedTask(null); return; }
    // Fetch detail
    try {
      const res = await apiFetch(`/tasks/${taskId}`);
      if (res.ok) {
        const data = await res.json();
        setTasks(prev => prev.map(t => t.task_id === taskId ? { ...t, ...data } : t));
        setExpandedTask(taskId);
      }
    } catch { /* ignore */ }
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

        {/* Skills bar */}
        <div className="glass p-4 mb-4">
          <p className="text-xs font-semibold text-[var(--text-primary)] mb-2">可用技能</p>
          <div className="flex flex-wrap gap-2">
            {['sql_executor', 'stats_engine', 'knowledge_search', 'save_report'].map(s => (
              <span key={s} className="px-3 py-1 text-xs rounded-full bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-secondary)]">⚡ {s}</span>
            ))}
          </div>
        </div>

        {/* Filters */}
        <div className="flex gap-2 mb-4" data-testid="agent-task-filters">
          {['all', 'pending', 'running', 'completed', 'failed'].map(f => (
            <button key={f} onClick={() => setFilter(f)}
              className={`px-3 py-1 text-xs rounded-full border transition-colors ${
                filter === f ? 'border-[var(--accent)] text-[var(--accent)] bg-[var(--accent)]/10' : 'border-[var(--border-glass)] text-[var(--text-secondary)]'
              }`}
              data-testid={`agent-filter-${f}`}>{f === 'all' ? '全部' : {pending:'等待中',running:'运行中',completed:'已完成',failed:'失败'}[f as string]}</button>
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
                <button onClick={() => toggleExpand(task.task_id)}
                  className="w-full text-left p-4 flex items-center justify-between hover:bg-white/5 transition-colors"
                  data-testid={`agent-task-title-${idx}`}>
                  <div>
                    <p className="text-sm font-medium text-[var(--text-primary)]">{task.title || task.task_id?.slice(0, 12)}</p>
                    <p className="text-xs text-[var(--text-secondary)] mt-1">{new Date(task.created_at).toLocaleString()}</p>
                  </div>
                  <div className="flex items-center gap-2">
                    {statusPill(task.status)}
                    {task.progress != null && task.status === 'running' && (
                      <div className="w-20 h-1.5 bg-[var(--glass-bg)] rounded-full overflow-hidden" data-testid={`task-progress-bar-${idx}`}>
                        <div className="h-full bg-[var(--accent)] rounded-full" style={{ width: `${task.progress}%` }} data-testid={`task-progress-fill-${idx}`} />
                      </div>
                    )}
                    <span className="text-xs text-[var(--text-secondary)]">{expandedTask === task.task_id ? '▲' : '▼'}</span>
                  </div>
                </button>

                {/* Expanded detail */}
                {expandedTask === task.task_id && (
                  <div className="px-4 pb-4 border-t border-[var(--border-glass)]" data-testid={`agent-task-detail-${idx}`}>
                    {/* Action buttons */}
                    <div className="flex gap-2 py-3" data-testid={`agent-task-actions-${idx}`}>
                      {(task.status === 'running' || task.status === 'pending') && (
                        <button onClick={() => cancelTask(task.task_id)}
                          className="px-3 py-1 text-xs rounded-lg border border-red-400/30 text-red-400 hover:bg-red-400/10"
                          data-testid={`agent-cancel-btn-${idx}`}>取消</button>
                      )}
                      {task.status === 'failed' && (
                        <button onClick={async () => {
                          await apiFetch('/tasks', { method: 'POST', body: JSON.stringify({ title: task.title, skills: ['sql_executor'] }) });
                          await loadTasks();
                        }}
                          className="px-3 py-1 text-xs rounded-lg border border-[var(--accent)]/30 text-[var(--accent)] hover:bg-[var(--accent)]/10"
                          data-testid={`agent-retry-btn-${idx}`}>重试</button>
                      )}
                    </div>
                    {/* Logs */}
                    {task.logs && task.logs.length > 0 && (
                      <div data-testid={`agent-task-logs-${idx}`}>
                        <p className="text-xs font-semibold text-[var(--text-secondary)] mb-1">执行日志</p>
                        <pre className="text-xs text-[var(--text-secondary)] bg-black/20 rounded-lg p-2 max-h-32 overflow-y-auto">{task.logs.join('\n')}</pre>
                      </div>
                    )}
                    {/* Artifacts */}
                    {task.artifacts && task.artifacts.length > 0 && (
                      <div className="mt-2" data-testid={`agent-task-artifacts-${idx}`}>
                        <p className="text-xs font-semibold text-[var(--text-secondary)] mb-1">输出文件</p>
                        {task.artifacts.map(a => (
                          <div key={a.id} className="flex items-center justify-between py-1 text-xs">
                            <span className="text-[var(--text-primary)]">{a.name}</span>
                            <button data-testid={`artifact-download-${a.id}`}
                              className="text-[var(--accent)] hover:underline">下载</button>
                          </div>
                        ))}
                      </div>
                    )}
                    {/* Cron info */}
                    {task.cron_expr && (
                      <p className="text-xs text-[var(--text-secondary)] mt-2">⏱ 定时: {task.cron_expr}</p>
                    )}
                  </div>
                )}
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
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">技能（逗号分隔）</label>
                <input type="text" value={newTask.skills} onChange={e => setNewTask(p => ({ ...p, skills: e.target.value }))}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)]"
                  data-testid="agent-task-skills-input" />
              </div>
              <label className="flex items-center gap-2 text-sm text-[var(--text-primary)] cursor-pointer">
                <input type="checkbox" checked={newTask.async} onChange={e => setNewTask(p => ({ ...p, async: e.target.checked }))}
                  data-testid="agent-task-async-toggle" className="rounded" />
                异步执行
              </label>
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
