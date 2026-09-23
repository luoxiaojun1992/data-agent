'use client';

import React, { useState, useEffect, useRef } from 'react';
import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import AppLayout from '../providers';
import ModelSelector from '../components/ModelSelector';
import Pagination from '../components/Pagination';
import { useAuth } from '@/lib/api';
import { fileToAttachment, MAX_ATTACHMENT_IMAGES, MAX_ATTACHMENT_IMAGE_BYTES, MAX_PDF_BYTES, MAX_EXCEL_BYTES, type Attachment, type PdfAttachment, type ExcelAttachment } from '@/lib/attachment';
import { parsePdf, isPdfFile } from '@/lib/pdf';
import { parseExcel, isExcelFile } from '@/lib/excel';

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
  const t = useTranslations('agent');
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
  const [pdfs, setPdfs] = useState<PdfAttachment[]>([]); // PDF attachments (name + parsed text)
  const [excels, setExcels] = useState<ExcelAttachment[]>([]); // Excel attachments (name + parsed text)
  const [attachError, setAttachError] = useState('');
  const attachmentInputRef = useRef<HTMLInputElement>(null);
  // SPEC-086: 「常用模版」入口（独立于「新建任务」弹窗）+ 日常总结确认弹窗。
  const [showTemplateMenu, setShowTemplateMenu] = useState(false);
  const [showDailySummaryModal, setShowDailySummaryModal] = useState(false);
  const [dailySummaryTitle, setDailySummaryTitle] = useState(() => t('dailySummaryTitle'));

  // Wait for auth hydration before loading — otherwise loadTasks fires with
  // auth.token=null and the request misses the Authorization header.
  useEffect(() => {
    if (!auth.hydrated || !auth.token) return;
    loadTasks(page);
  }, [page, auth.hydrated, auth.token]);

  const toggleScheduledEnabled = async (task: AgentTask) => {
    const enabled = task.scheduled_enabled === false;
    const res = await apiFetch('/tasks/' + task.task_id + '/enabled', {
      method: 'PATCH',
      body: JSON.stringify({ enabled }),
    });
    if (res.ok) {
      setTasks(prev => prev.map(x => x.task_id === task.task_id ? { ...x, scheduled_enabled: enabled } : x));
    }
  };

  const loadTasks = async (p: number) => {
    setLoading(true);
    try {
      const res = await apiFetch(`/tasks?page=${p}&page_size=${pageSize}`);
      const data = await res.json();
      const rawTasks: AgentTask[] = Array.isArray(data) ? data : (data.tasks || []);
      setTasks(rawTasks.map((task: AgentTask) => ({ ...task, title: task.title || task.type || '' })));
      setTotal(typeof data.total === 'number' ? data.total : rawTasks.length);
    } catch (e) { console.error('[agent] loadTasks failed:', e); }
    finally { setLoading(false); }
  };

  // Add image + PDF + Excel attachments from a FileList, enforcing the 5-image
  // / 2MiB limits and the 20MiB PDF/Excel size limits (SPEC-077 / SPEC-096).
  const addAttachments = async (files: File[]) => {
    const images = files.filter((f) => f.type.startsWith('image/'));
    const pdfFiles = files.filter(
      (f) => f.type === 'application/pdf' || isPdfFile(f.name),
    );
    const excelFiles = files.filter((f) => isExcelFile(f.name));
    if (images.length > 0 && attachments.length + images.length > MAX_ATTACHMENT_IMAGES) {
      setAttachError(t('errMaxImages', { n: MAX_ATTACHMENT_IMAGES }));
      setTimeout(() => setAttachError(''), 3000);
      return;
    }
    for (const f of images) {
      if (f.size > MAX_ATTACHMENT_IMAGE_BYTES) {
        setAttachError(t('errImageSize', { name: f.name }));
        setTimeout(() => setAttachError(''), 3000);
        continue;
      }
      try {
        const att = await fileToAttachment(f);
        setAttachments((prev) => (prev.length >= MAX_ATTACHMENT_IMAGES ? prev : [...prev, att]));
      } catch {
        setAttachError(t('errReadImage'));
        setTimeout(() => setAttachError(''), 3000);
      }
    }

    // PDF 附件：解析文字存 pdfs，解析图并入图片附件（合并计数 ≤5，SPEC-096 R1）。
    for (const f of pdfFiles) {
      if (f.size > MAX_PDF_BYTES) {
        setAttachError(t('errPdfSize', { name: f.name }));
        setTimeout(() => setAttachError(''), 3000);
        continue;
      }
      try {
        const { text, images: pdfImages } = await parsePdf(f);
        setPdfs((prev) => [...prev, { name: f.name, text }]);
        for (const img of pdfImages) {
          const base64 = img.dataUrl.split(',')[1] || '';
          if (Math.floor((base64.length * 3) / 4) > MAX_ATTACHMENT_IMAGE_BYTES) continue;
          setAttachments((prev) =>
            prev.length >= MAX_ATTACHMENT_IMAGES
              ? prev
              : [...prev, { name: f.name, mimeType: img.mimeType, base64, dataUrl: img.dataUrl }],
          );
        }
      } catch {
        setAttachError(t('errParsePdf', { name: f.name }));
        setTimeout(() => setAttachError(''), 3000);
      }
    }

    // Excel 附件：解析为纯文本存 excels（无图片，SPEC-096 R3）。
    for (const f of excelFiles) {
      if (f.size > MAX_EXCEL_BYTES) {
        setAttachError(t('errExcelSize', { name: f.name }));
        setTimeout(() => setAttachError(''), 3000);
        continue;
      }
      try {
        const { text } = await parseExcel(f);
        setExcels((prev) => [...prev, { name: f.name, text }]);
      } catch {
        setAttachError(t('errParseExcel', { name: f.name }));
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

  const removePdf = (index: number) => {
    setPdfs((prev) => prev.filter((_, i) => i !== index));
  };

  const removeExcel = (index: number) => {
    setExcels((prev) => prev.filter((_, i) => i !== index));
  };

  const createTask = async () => {
    if (!newTask.title.trim()) return;
    if (newTask.cronEnabled && ((newTask.scheduleMode === "recurring" && !newTask.cron) || (newTask.scheduleMode === "one_time" && !newTask.scheduledAt))) { alert(t('errScheduleIncomplete')); return; }
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
      if (pdfs.length > 0) {
        body.pdfs = pdfs.map((p) => ({ name: p.name, text: p.text }));
      }
      if (excels.length > 0) {
        body.excels = excels.map((e) => ({ name: e.name, text: e.text }));
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
        setPdfs([]);
        setExcels([]);
        setAttachError('');
      }
    } catch (e) { console.error('[agent] task create failed:', e); }
  };

  // SPEC-082 §5.8: "delete" ≠ "cancel" — DELETE /tasks/:id physically removes
  // the task definition (irreversible); historical run records are kept.
  const deleteTask = async (taskId: string) => {
    if (!window.confirm(t('deleteConfirm'))) return;
    await apiFetch(`/tasks/${taskId}`, { method: 'DELETE' });
    await loadTasks(page);
  };

  // SPEC-086: create the「日常总结」template task (scheduled_exec, daily 01:00).
  const createDailySummaryTask = async () => {
    if (!dailySummaryTitle.trim()) return;
    try {
      const res = await apiFetch('/tasks', {
        method: 'POST',
        body: JSON.stringify({
          title: dailySummaryTitle.trim(),
          type: 'scheduled_exec',
          schedule_mode: 'recurring',
          cron_expr: '0 1 * * *',
          params: {
            message: '你是日常总结助手。请执行：1) 用 memory_list 按创建时间倒序分页读取今天的记忆（offset 从 0 开始，每页 limit=20，翻页直到某页返回的 created_at 早于今天为止）；2) 将今天的记忆归纳为结构化 markdown 总结；3) 用 kb_create_doc 创建文档，title 用「YYYY-MM-DD 日常总结」；4) 用 save_task_result 保存结果。',
          },
        }),
      });
      if (res.ok) {
        await loadTasks(page);
        setShowDailySummaryModal(false);
        setDailySummaryTitle(t('dailySummaryTitle'));
      }
    } catch (e) { console.error('[agent] daily summary template create failed:', e); }
  };

  const openTask = (taskId: string) => {
    router.push(`/agent/tasks/${taskId}`);
  };

  const statusPill = (s: string) => {
    const map: Record<string, { label: string; cls: string }> = {
      pending: { label: t('statusPending'), cls: 'text-amber-400 bg-amber-400/10' },
      running: { label: t('statusRunning'), cls: 'text-blue-400 bg-blue-400/10' },
      completed: { label: t('statusCompleted'), cls: 'text-emerald-400 bg-emerald-400/10' },
      failed: { label: t('statusFailed'), cls: 'text-red-400 bg-red-400/10' },
      cancelled: { label: t('statusCancelled'), cls: 'text-gray-400 bg-gray-400/10' },
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
            <h2 className="text-2xl font-bold text-[var(--text-primary)]">{t('title')}</h2>
            <p className="text-sm text-[var(--text-secondary)] mt-1">{t('desc')}</p>
          </div>
          <div className="flex items-center gap-2 relative">
            <button onClick={() => setShowTemplateMenu(v => !v)}
              className="px-4 py-2 rounded-xl text-sm font-medium border border-[var(--border-glass)] text-[var(--text-primary)] hover:border-[var(--accent)]/40"
              data-testid="agent-template-btn">{t('templateMenu')}</button>
            <button onClick={() => setShowModal(true)}
              className="px-4 py-2 bg-[var(--accent)] text-white rounded-xl text-sm font-medium hover:opacity-90"
              data-testid="agent-create-task-btn">{t('createTask')}</button>

            {/* Template menu (SPEC-086) */}
            {showTemplateMenu && (
              <div className="absolute right-0 top-11 z-40 glass rounded-xl p-2 w-64" data-testid="agent-template-menu">
                <button onClick={() => { setShowDailySummaryModal(true); setShowTemplateMenu(false); }}
                  className="w-full text-left p-3 rounded-lg hover:bg-[var(--surface-5)] transition-colors"
                  data-testid="agent-template-daily-summary">
                  <p className="text-sm font-medium text-[var(--text-primary)]">{t('dailySummary')}</p>
                  <p className="text-xs text-[var(--text-secondary)] mt-0.5">{t('dailySummaryDesc')}</p>
                </button>
              </div>
            )}
          </div>
        </div>


        {/* Task list */}
        {loading ? (
          <div className="text-center py-12 text-[var(--text-secondary)]" data-testid="agent-loading">{t('loading')}</div>
        ) : filtered.length === 0 ? (
          <div className="glass p-12 text-center" data-testid="agent-empty">
            <span className="text-5xl block mb-4">⚡</span>
            <p className="text-lg text-[var(--text-primary)] mb-2">{t('empty')}</p>
            <p className="text-sm text-[var(--text-secondary)]">{t('emptyHint')}</p>
          </div>
        ) : (
          <div className="space-y-3" data-testid="agent-task-table">
            {filtered.map((task, idx) => (
              <div key={task.task_id} className="glass" data-testid={`agent-task-row-${idx}`}>
                {/* Row header */}
                <button onClick={() => openTask(task.task_id)}
                  className="w-full text-left p-4 flex items-center justify-between hover:bg-[var(--surface-5)] transition-colors"
                  data-testid={`agent-task-title-${idx}`}>
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <p className="text-sm font-medium text-[var(--text-primary)]">{task.title || task.type || task.task_id?.slice(0, 12)}</p>
                      <span style={{
                        fontSize: '10px', padding: '1px 6px', borderRadius: '4px', fontWeight: 500,
                        background: task.type === 'scheduled_exec' ? 'rgba(96,165,250,0.15)' : 'rgba(148,163,184,0.15)',
                        color: task.type === 'scheduled_exec' ? '#60a5fa' : '#94a3b8',
                      }}>
                        {task.type === 'scheduled_exec' ? t('typeScheduled') : t('typeRealtime')}
                      </span>
                      {task.type === 'scheduled_exec' && (
                        <button onClick={(e) => { e.stopPropagation(); toggleScheduledEnabled(task); }}
                          style={{
                            fontSize: '10px', padding: '1px 6px', borderRadius: '4px', cursor: 'pointer',
                            border: '1px solid var(--surface-15)',
                            background: task.scheduled_enabled !== false ? 'rgba(16,185,129,0.15)' : 'rgba(239,68,68,0.1)',
                            color: task.scheduled_enabled !== false ? '#10b981' : '#ef4444',
                          }}>
                          {task.scheduled_enabled !== false ? 'ON' : 'OFF'}
                        </button>
                      )}
                    </div>
                    <div className="flex items-center gap-3 mt-1 text-xs text-[var(--text-secondary)]">
                      <span data-testid={`agent-task-run-count-${idx}`}>
                        🔁 {(task.run_count ?? 0)} {t('runTimes')}
                      </span>
                      {task.last_run_at && (
                        <span data-testid={`agent-task-last-run-${idx}`}>
                          · {t('lastRun')}: {new Date(task.last_run_at).toLocaleString()}
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
                  <div className="flex items-center gap-2">
                    <button onClick={(e) => { e.stopPropagation(); deleteTask(task.task_id); }}
                      className="text-[10px] text-red-400 hover:text-red-300"
                      data-testid={`agent-task-delete-${task.task_id}`}>{t('delete')}</button>
                    <div className="text-[var(--text-secondary)] text-sm">▶</div>
                  </div>
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
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">{t('newTaskTitle')}</h3>
              <button onClick={() => setShowModal(false)} className="text-[var(--text-secondary)] hover:text-[var(--text-primary)]">✕</button>
            </div>
            <div className="space-y-4">
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">{t('taskTitleLabel')}</label>
                <input type="text" value={newTask.title} onChange={e => setNewTask(p => ({ ...p, title: e.target.value }))}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)]"
                  data-testid="agent-task-title-input" placeholder={t('taskTitlePlaceholder')} />
              </div>
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">{t('descriptionLabel')}</label>
                <textarea value={newTask.description} onChange={e => setNewTask(p => ({ ...p, description: e.target.value }))}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)] resize-none"
                  data-testid="agent-task-desc-input" rows={2} placeholder={t('descriptionPlaceholder')} />
              </div>
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">{t('attachmentLabel')}</label>
                <input
                  ref={attachmentInputRef}
                  type="file"
                  accept="image/*,application/pdf,.pdf,.xlsx"
                  multiple
                  style={{ display: 'none' }}
                  data-testid="agent-task-attach-input"
                  onChange={handleAttachChange}
                />
                {attachments.length > 0 && (
                  <div className="flex flex-wrap gap-2 mb-2" data-testid="agent-task-attachments">
                    {attachments.map((att, idx) => (
                      <div key={idx} className="relative" data-testid={`agent-task-attachment-${idx}`}>
                        <img src={att.dataUrl} alt={att.name} className="w-14 h-14 rounded-lg object-cover border border-[var(--surface-20)]" />
                        <button onClick={() => removeAttachment(idx)} title={t('removeImage')}
                          data-testid={`agent-task-attachment-remove-${idx}`}
                          className="absolute -top-1.5 -right-1.5 w-5 h-5 rounded-full bg-black/70 text-white text-xs leading-none flex items-center justify-center hover:bg-black/90">✕</button>
                      </div>
                    ))}
                  </div>
                )}
                {pdfs.length > 0 && (
                  <div className="flex flex-wrap gap-2 mb-2" data-testid="agent-task-pdf-attachments">
                    {pdfs.map((pdf, idx) => (
                      <div key={idx} className="relative flex items-center gap-2 pl-3 pr-8 py-1.5 rounded-lg border border-[var(--surface-20)] bg-[var(--glass-bg)]" data-testid={`agent-task-pdf-attachment-${idx}`}>
                        <span className="text-sm leading-none">📄</span>
                        <span className="text-xs max-w-[140px] truncate" title={pdf.name}>{pdf.name}</span>
                        <button onClick={() => removePdf(idx)} title={t('removePdf')}
                          data-testid={`agent-task-pdf-attachment-remove-${idx}`}
                          className="absolute -top-1.5 -right-1.5 w-5 h-5 rounded-full bg-black/70 text-white text-xs leading-none flex items-center justify-center hover:bg-black/90">✕</button>
                      </div>
                    ))}
                  </div>
                )}
                {excels.length > 0 && (
                  <div className="flex flex-wrap gap-2 mb-2" data-testid="agent-task-excel-attachments">
                    {excels.map((excel, idx) => (
                      <div key={idx} className="relative flex items-center gap-2 pl-3 pr-8 py-1.5 rounded-lg border border-[var(--surface-20)] bg-[var(--glass-bg)]" data-testid={`agent-task-excel-attachment-${idx}`}>
                        <span className="text-sm leading-none">📊</span>
                        <span className="text-xs max-w-[140px] truncate" title={excel.name}>{excel.name}</span>
                        <button onClick={() => removeExcel(idx)} title={t('removeExcel')}
                          data-testid={`agent-task-excel-attachment-remove-${idx}`}
                          className="absolute -top-1.5 -right-1.5 w-5 h-5 rounded-full bg-black/70 text-white text-xs leading-none flex items-center justify-center hover:bg-black/90">✕</button>
                      </div>
                    ))}
                  </div>
                )}
                {attachError && <p className="text-xs text-[#ef4444] mb-1" data-testid="agent-task-attach-error">{attachError}</p>}
                <button onClick={handleAttachClick}
                  disabled={attachments.length >= MAX_ATTACHMENT_IMAGES}
                  className="px-3 py-1.5 text-xs rounded-lg border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] disabled:opacity-40"
                  data-testid="agent-task-attach-btn">{t('addAttachment')}</button>
              </div>
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">{t('modelLabel')}</label>
                <ModelSelector
                  value={newTask.modelId}
                  onChange={(id) => setNewTask(p => ({ ...p, modelId: id }))}
                  token={auth.token}
                />
              </div>
              <label className="flex items-center gap-2 text-sm text-[var(--text-primary)] cursor-pointer">
                <input type="checkbox" checked={newTask.cronEnabled} onChange={e => setNewTask(p => ({ ...p, cronEnabled: e.target.checked }))}
                  data-testid="agent-task-cron-toggle" className="rounded" />
                {t('setScheduled')}
              </label>
              {newTask.cronEnabled && (
                <div data-testid="agent-task-cron-config" className="space-y-3">
                  {/* Mode toggle */ }
                  <div>
                    <label className="block text-xs text-[var(--text-secondary)] mb-1">{t('scheduleModeLabel')}</label>
                    <div className="flex gap-2">
                      {(['recurring', 'one_time'] as const).map(mode => (
                        <button key={mode} type="button"
                          onClick={() => setNewTask(p => ({ ...p, scheduleMode: mode, cron: mode === 'recurring' ? p.cron : '', scheduledAt: mode === 'one_time' ? p.scheduledAt : '' }))}
                          className={`flex-1 px-3 py-2 text-xs rounded-lg border transition-all ${
                            newTask.scheduleMode === mode
                              ? 'bg-[var(--accent)]/15 border-[var(--accent)] text-[var(--accent)]'
                              : 'bg-[var(--glass-bg)] border-[var(--border-glass)] text-[var(--text-secondary)]'
                          }`}>
                          {mode === 'recurring' ? t('recurring') : t('oneTime')}
                        </button>
                      ))}
                    </div>
                  </div>

                  {newTask.scheduleMode === 'recurring' ? (
                    <div>
                      <label className="block text-xs text-[var(--text-secondary)] mb-1">{t('scheduleOptionsLabel')}</label>
                      <div className="flex flex-wrap gap-2">
                        {[
                          { label: t('cronHourly'), value: '0 * * * *' },
                          { label: t('cronDaily0'), value: '0 0 * * *' },
                          { label: t('cronDaily6'), value: '0 6 * * *' },
                          { label: t('cronDaily8'), value: '0 8 * * *' },
                          { label: t('cronWeeklyMon9'), value: '0 9 * * 1' },
                          { label: t('cronMonthly1'), value: '0 0 1 * *' },
                          { label: t('cronYearly'), value: '0 0 1 1 *' },
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
                        placeholder={t('cronPlaceholder')} />
                    </div>
                  ) : (
                    <div>
                      <label className="block text-xs text-[var(--text-secondary)] mb-1">{t('execTimeLabel')}</label>
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
                  className="flex-1 px-4 py-2 text-sm rounded-xl border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">{t('cancel')}</button>
                <button onClick={createTask}
                  className="flex-1 px-4 py-2 text-sm rounded-xl bg-[var(--accent)] text-white hover:opacity-90 disabled:opacity-40"
                  data-testid="agent-task-create-btn" disabled={!newTask.title.trim() || (newTask.cronEnabled && !(newTask.cron || newTask.scheduledAt))}>{t('createTaskBtn')}</button>
              </div>
              <p className="text-center text-[11px] text-[var(--text-secondary)]" data-testid="agent-task-ai-tips">{t('aiTips')}</p>
            </div>
          </div>
        </div>
      )}

      {/* Daily summary template confirm modal (SPEC-086) */}
      {showDailySummaryModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center" data-testid="agent-template-confirm">
          <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={() => setShowDailySummaryModal(false)} />
          <div className="relative glass p-6 rounded-2xl max-w-md w-full mx-4">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">{t('dailySummaryModalTitle')}</h3>
              <button onClick={() => setShowDailySummaryModal(false)} className="text-[var(--text-secondary)] hover:text-[var(--text-primary)]">✕</button>
            </div>
            <div className="space-y-4">
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">{t('taskTitleLabel')}</label>
                <input type="text" value={dailySummaryTitle} onChange={e => setDailySummaryTitle(e.target.value)}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)]"
                  data-testid="agent-template-daily-summary-title" placeholder={t('dailySummaryTitle')} />
              </div>
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">{t('scheduleLabel')}</label>
                <div className="px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-secondary)]">
                  {t('dailyAt0100')}
                </div>
              </div>
              <p className="text-xs text-[var(--text-secondary)]">{t('dailySummaryHint')}</p>
              <div className="flex gap-3 pt-2">
                <button onClick={() => setShowDailySummaryModal(false)}
                  className="flex-1 px-4 py-2 text-sm rounded-xl border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">{t('cancel')}</button>
                <button onClick={createDailySummaryTask}
                  className="flex-1 px-4 py-2 text-sm rounded-xl bg-[var(--accent)] text-white hover:opacity-90 disabled:opacity-40"
                  data-testid="agent-template-create-btn" disabled={!dailySummaryTitle.trim()}>{t('createTemplateTask')}</button>
              </div>
            </div>
          </div>
        </div>
      )}
    </AppLayout>
  );
}
