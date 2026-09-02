'use client';

import React, { useState, useEffect, useRef } from 'react';
import { useRouter } from 'next/navigation';
import AppLayout from '../providers';
import ModelSelector from '../components/ModelSelector';
import Pagination from '../components/Pagination';
import { useAuth } from '@/lib/api';
import { fileToAttachment, MAX_ATTACHMENT_IMAGES, MAX_ATTACHMENT_IMAGE_BYTES, type Attachment } from '@/lib/attachment';

interface AgentTask {
  task_id: string;
  title?: string;
  type?: string;
  status: string; // pending | running | completed | failed | cancelled
  progress?: number;
  run_count?: number;       // atomic counter, updated on each run creation
  last_run_at?: string;     // updated atomically on each run creation
  created_at: string;
  updated_at?: string;
  cron_expr?: string;
  schedule_mode?: string;
  scheduled_at?: string;
  scheduled_enabled?: boolean;
  logs?: string[];
  artifacts?: { name: string; id: string }[];
}

export default function AgentPage() {
  const router = useRouter();
  const { apiFetch, auth } = useAuth();
  const [tasks, setTasks] = useState<AgentTask[]>([]);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(20);
  const [total, setTotal] = useState(0);
  const [newTask, setNewTask] = useState({ title: '', description: '', cron: '', cronEnabled: false, scheduleMode: 'recurring' as 'recurring' | 'one_time', scheduledAt: '', modelId: '' });
  const [attachments, setAttachments] = useState<Attachment[]>([]); // image attachments (max 5)
  const [attachError, setAttachError] = useState('');
  const attachmentInputRef = useRef<HTMLInputElement>(null);

  // Wait for auth hydration before loading — otherwise loadTasks fires with
  // auth.token=null and the request misses the Authorization header.
  useEffect(() => {
    if (!auth.hydrated || !auth.token) return;
    loadTasks(page);
  }, [page, auth.hydrated, auth.token]);

  const toggleScheduledEnabled = async (t: AgentTask) => {
    const enabled = t.scheduled_enabled === false;
    const res = await apiFetch('/admin/tasks/' + t.task_id + '/scheduled-enabled', {
      method: 'PATCH',
      body: JSON.stringify({ enabled }),
    });
    if (res.ok) {
      setTasks(prev => prev.map(x => x.task_id === t.task_id ? { ...x, scheduled_enabled: enabled } : x));
    }
  };

  const loadTasks = async (p: number) => {
    setLoading(true);
    try {
      const res = await apiFetch(`/tasks?page=${p}&page_size=${pageSize}`);
      const data = await res.json();
      const rawTasks: AgentTask[] = Array.isArray(data) ? data : (data.tasks || []);
      setTasks(rawTasks.map((t: AgentTask) => ({ ...t, title: t.title || t.type || '' })));
      setTotal(typeof data.total === 'number' ? data.total : rawTasks.length);
    } catch (e) { console.error('[agent] loadTasks failed:', e); }
    finally { setLoading(false); }
  };

  // Add image attachments from a FileList, enforcing the 5-image / 2MiB limits.
  const addAttachments = async (files: File[]) => {
    const images = files.filter((f) => f.type.startsWith('image/'));
    if (images.length === 0) return;
    if (attachments.length + images.length > MAX_ATTACHMENT_IMAGES) {
      setAttachError(`最多 ${MAX_ATTACHMENT_IMAGES} 张图片`);
      setTimeout(() => setAttachError(''), 3000);
      return;
    }
    for (const f of images) {
      if (f.size > MAX_ATTACHMENT_IMAGE_BYTES) {
        setAttachError(`图片 ${f.name} 超过 2MB 限制`);
        setTimeout(() => setAttachError(''), 3000);
        continue;
      }
      try {
        const att = await fileToAttachment(f);
        setAttachments((prev) => (prev.length >= MAX_ATTACHMENT_IMAGES ? prev : [...prev, att]));
      } catch {
        setAttachError('读取图片失败');
        setTimeout(() => setAttachError(''), 3000);
      }
    }
  };

  const handleAttachClick = () => attachmentInputRef.current?.click();

  const handleAttachChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files) addAttachments(Array.from(e.target.files));
    e.target.value = ''; // allow re-selecting the same file
  };

  const handlePaste = (e: React.ClipboardEvent) => {
    const files = Array.from(e.clipboardData?.files || []);
    if (files.length > 0) {
      e.preventDefault();
      addAttachments(files);
    }
  };

  const removeAttachment = (index: number) => {
    setAttachments((prev) => prev.filter((_, i) => i !== index));
  };

  const createTask = async () => {
    if (!newTask.title.trim()) return;
    if (newTask.cronEnabled && ((newTask.scheduleMode === "recurring" && !newTask.cron) || (newTask.scheduleMode === "one_time" && !newTask.scheduledAt))) { alert("请填写完整的定时信息"); return; }
    // If cron is enabled, a schedule must be chosen.
    if (newTask.cronEnabled && !newTask.cron && !newTask.scheduledAt) return;
    try {
      const body: Record<string, unknown> = {
        title: newTask.title,
        description: newTask.description,
        type: newTask.cronEnabled ? 'scheduled_exec' : 'agent_exec',
        model_id: newTask.modelId || undefined,
      };
      if (attachments.length > 0) {
        body.images = attachments.map((a) => ({ data: a.base64, mime_type: a.mimeType }));
      }
      if (newTask.cronEnabled) {
        body.schedule_mode = newTask.scheduleMode;
        if (newTask.scheduleMode === 'recurring' && newTask.cron) {
          body.cron_expr = newTask.cron;
        } else if (newTask.scheduleMode === 'one_time' && newTask.scheduledAt) {
          body.scheduled_at = new Date(newTask.scheduledAt).toISOString();
        }
      }
      const res = await apiFetch('/tasks', { method: 'POST', body: JSON.stringify(body) });
      if (res.ok) {
        await loadTasks(page);
        setShowModal(false);
        setNewTask({ title: '', description: '', cron: '', cronEnabled: false, scheduleMode: 'recurring', scheduledAt: '', modelId: '' });
        setAttachments([]);
        setAttachError('');
      }
    } catch (e) { console.error('[agent] task create failed:', e); }
  };

  const cancelTask = async (taskId: string) => {
    await apiFetch(`/tasks/${taskId}/cancel`, { method: 'PUT' });
    await loadTasks(page);
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

  const filtered = tasks; // pagination already server-side

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
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <p className="text-sm font-medium text-[var(--text-primary)]">{task.title || task.type || task.task_id?.slice(0, 12)}</p>
                      <span style={{
                        fontSize: '10px', padding: '1px 6px', borderRadius: '4px', fontWeight: 500,
                        background: task.type === 'scheduled_exec' ? 'rgba(96,165,250,0.15)' : 'rgba(148,163,184,0.15)',
                        color: task.type === 'scheduled_exec' ? '#60a5fa' : '#94a3b8',
                      }}>
                        {task.type === 'scheduled_exec' ? '⏰ 定时' : '▶ 实时'}
                      </span>
                      {task.type === 'scheduled_exec' && (
                        <button onClick={(e) => { e.stopPropagation(); toggleScheduledEnabled(task); }}
                          style={{
                            fontSize: '10px', padding: '1px 6px', borderRadius: '4px', cursor: 'pointer',
                            border: '1px solid rgba(255,255,255,0.15)',
                            background: task.scheduled_enabled !== false ? 'rgba(16,185,129,0.15)' : 'rgba(239,68,68,0.1)',
                            color: task.scheduled_enabled !== false ? '#10b981' : '#ef4444',
                          }}>
                          {task.scheduled_enabled !== false ? 'ON' : 'OFF'}
                        </button>
                      )}
                    </div>
                    <div className="flex items-center gap-3 mt-1 text-xs text-[var(--text-secondary)]">
                      <span data-testid={`agent-task-run-count-${idx}`}>
                        🔁 {(task.run_count ?? 0)} 次运行
                      </span>
                      {task.last_run_at && (
                        <span data-testid={`agent-task-last-run-${idx}`}>
                          · 上次: {new Date(task.last_run_at).toLocaleString()}
                        </span>
                      )}
                      {task.type === 'scheduled_exec' && task.schedule_mode === 'recurring' && task.cron_expr && (
                        <span>· 📋 {task.cron_expr}</span>
                      )}
                      {task.type === 'scheduled_exec' && task.schedule_mode === 'one_time' && task.scheduled_at && (
                        <span>· 🕐 {new Date(task.scheduled_at).toLocaleString()}</span>
                      )}
                    </div>
                  </div>
                  <div className="text-[var(--text-secondary)] text-sm">▶</div>
                </button>
              </div>
            ))}

            {/* Pagination */}
            <Pagination page={page} total={total} pageSize={pageSize} onChange={setPage} testIdPrefix="agent-task" />
          </div>
        )}
      </div>

      {/* Create Task Modal */}
      {showModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center" data-testid="agent-task-modal">
          <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={() => setShowModal(false)} />
          <div className="relative glass p-6 rounded-2xl max-w-lg w-full mx-4" onPaste={handlePaste}>
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
                <label className="block text-xs text-[var(--text-secondary)] mb-1">图片附件（可选，最多 5 张，可粘贴）</label>
                <input
                  ref={attachmentInputRef}
                  type="file"
                  accept="image/*"
                  multiple
                  style={{ display: 'none' }}
                  data-testid="agent-task-attach-input"
                  onChange={handleAttachChange}
                />
                {attachments.length > 0 && (
                  <div className="flex flex-wrap gap-2 mb-2" data-testid="agent-task-attachments">
                    {attachments.map((att, idx) => (
                      <div key={idx} className="relative" data-testid={`agent-task-attachment-${idx}`}>
                        <img src={att.dataUrl} alt={att.name} className="w-14 h-14 rounded-lg object-cover border border-white/20" />
                        <button onClick={() => removeAttachment(idx)} title="移除图片"
                          data-testid={`agent-task-attachment-remove-${idx}`}
                          className="absolute -top-1.5 -right-1.5 w-5 h-5 rounded-full bg-black/70 text-white text-xs leading-none flex items-center justify-center hover:bg-black/90">✕</button>
                      </div>
                    ))}
                  </div>
                )}
                {attachError && <p className="text-xs text-[#ef4444] mb-1" data-testid="agent-task-attach-error">{attachError}</p>}
                <button onClick={handleAttachClick}
                  disabled={attachments.length >= MAX_ATTACHMENT_IMAGES}
                  className="px-3 py-1.5 text-xs rounded-lg border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] disabled:opacity-40"
                  data-testid="agent-task-attach-btn">📎 添加图片</button>
              </div>
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">模型</label>
                <ModelSelector
                  value={newTask.modelId}
                  onChange={(id) => setNewTask(p => ({ ...p, modelId: id }))}
                  token={auth.token}
                />
              </div>
              <label className="flex items-center gap-2 text-sm text-[var(--text-primary)] cursor-pointer">
                <input type="checkbox" checked={newTask.cronEnabled} onChange={e => setNewTask(p => ({ ...p, cronEnabled: e.target.checked }))}
                  data-testid="agent-task-cron-toggle" className="rounded" />
                设为定时任务
              </label>
              {newTask.cronEnabled && (
                <div data-testid="agent-task-cron-config" className="space-y-3">
                  {/* Mode toggle */ }
                  <div>
                    <label className="block text-xs text-[var(--text-secondary)] mb-1">调度方式</label>
                    <div className="flex gap-2">
                      {(['recurring', 'one_time'] as const).map(mode => (
                        <button key={mode} type="button"
                          onClick={() => setNewTask(p => ({ ...p, scheduleMode: mode, cron: mode === 'recurring' ? p.cron : '', scheduledAt: mode === 'one_time' ? p.scheduledAt : '' }))}
                          className={`flex-1 px-3 py-2 text-xs rounded-lg border transition-all ${
                            newTask.scheduleMode === mode
                              ? 'bg-[var(--accent)]/15 border-[var(--accent)] text-[var(--accent)]'
                              : 'bg-[var(--glass-bg)] border-[var(--border-glass)] text-[var(--text-secondary)]'
                          }`}>
                          {mode === 'recurring' ? '🔄 重复执行' : '📅 一次性'}
                        </button>
                      ))}
                    </div>
                  </div>

                  {newTask.scheduleMode === 'recurring' ? (
                    <div>
                      <label className="block text-xs text-[var(--text-secondary)] mb-1">计划选项</label>
                      <div className="flex flex-wrap gap-2">
                        {[
                          { label: '每小时', value: '0 * * * *' },
                          { label: '每天 0:00', value: '0 0 * * *' },
                          { label: '每天 6:00', value: '0 6 * * *' },
                          { label: '每天 8:00', value: '0 8 * * *' },
                          { label: '每周一 9:00', value: '0 9 * * 1' },
                          { label: '每月1号 0:00', value: '0 0 1 * *' },
                          { label: '每年1月1日', value: '0 0 1 1 *' },
                        ].map(p => (
                          <button key={p.value} type="button"
                            onClick={() => setNewTask(prev => ({ ...prev, cron: p.value }))}
                            className={`px-3 py-1.5 text-xs rounded-lg border transition-all ${
                              newTask.cron === p.value
                                ? 'bg-[var(--accent)]/15 border-[var(--accent)] text-[var(--accent)]'
                                : 'bg-[var(--glass-bg)] border-[var(--border-glass)] text-[var(--text-secondary)] hover:border-[var(--accent)]/40'
                            }`}>
                            {p.label}
                          </button>
                        ))}
                      </div>
                      <input value={newTask.cron} onChange={e => setNewTask(p => ({ ...p, cron: e.target.value }))}
                        className="w-full mt-2 px-3 py-2 text-xs rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] font-mono"
                        placeholder="自定义 cron: 分 时 日 月 周" />
                    </div>
                  ) : (
                    <div>
                      <label className="block text-xs text-[var(--text-secondary)] mb-1">执行时间</label>
                      <input type="datetime-local" value={newTask.scheduledAt}
                        min={new Date().toISOString().slice(0, 16)}
                        onChange={e => setNewTask(p => ({ ...p, scheduledAt: e.target.value }))}
                        className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)]"
                      />
                    </div>
                  )}
                </div>
              )}
              <div className="flex gap-3 pt-2">
                <button onClick={() => setShowModal(false)}
                  className="flex-1 px-4 py-2 text-sm rounded-xl border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">取消</button>
                <button onClick={createTask}
                  className="flex-1 px-4 py-2 text-sm rounded-xl bg-[var(--accent)] text-white hover:opacity-90 disabled:opacity-40"
                  data-testid="agent-task-create-btn" disabled={!newTask.title.trim() || (newTask.cronEnabled && !(newTask.cron || newTask.scheduledAt))}>创建任务</button>
              </div>
            </div>
          </div>
        </div>
      )}
    </AppLayout>
  );
}
