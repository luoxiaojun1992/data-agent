'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '../../../lib/api';

interface Task {
  task_id: string;
  session_id?: string;
  user_id?: string;
  type?: string;
  status: string;
  skill_chain?: string[];
  created_at?: string;
  updated_at?: string;
  error?: string;
}

export default function TasksPage() {
  const { auth, apiFetch } = useAuth();
  const [tasks, setTasks] = useState<Task[]>([]);
  const [filter, setFilter] = useState('running');
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [detailTask, setDetailTask] = useState<Task | null>(null);

  const showToast = (msg: string, type: 'success' | 'error') => {
    setToast({ message: msg, type });
    setTimeout(() => setToast(null), 3000);
  };

  const fetchTasks = useCallback(async () => {
    try {
      const res = await apiFetch(`/admin/tasks?status=${filter}`);
      if (res.ok) {
        const data = await res.json();
        setTasks(Array.isArray(data) ? data : []);
      }
    } catch (e) { console.error('[admin/tasks] fetchTasks failed:', e); }
  }, [apiFetch, filter]);

  useEffect(() => {
    if (auth.hydrated) fetchTasks();
  }, [auth.hydrated, fetchTasks]);

  const toggleSelect = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  };

  const handleCancel = async (id: string) => {
    if (!confirm('确定要取消该任务吗？')) return;
    try {
      const res = await apiFetch(`/tasks/${id}/cancel`, { method: 'PUT' });
      if (res.ok) showToast('任务已取消', 'success');
      else showToast('取消失败', 'error');
      fetchTasks();
    } catch { showToast('取消失败', 'error'); }
  };

  const handleRetry = async (id: string) => {
    try {
      const res = await apiFetch(`/admin/tasks/${id}/retry`, { method: 'PUT' });
      if (res.ok) showToast('任务已重新入队', 'success');
      else showToast('重试失败', 'error');
      fetchTasks();
    } catch { showToast('重试失败', 'error'); }
  };

  const handleBatchCancel = async () => {
    if (selected.size === 0) return;
    if (!confirm(`确定取消 ${selected.size} 个任务吗？`)) return;
    try {
      const res = await apiFetch('/admin/tasks/batch-cancel', {
        method: 'POST',
        body: JSON.stringify({ task_ids: Array.from(selected) }),
      });
      if (res.ok) showToast(`已取消 ${selected.size} 个任务`, 'success');
      else showToast('批量取消失败', 'error');
      setSelected(new Set());
      fetchTasks();
    } catch { showToast('批量取消失败', 'error'); }
  };

  const filteredTasks = filter === 'all'
    ? tasks
    : tasks.filter((t) => t.status === filter);

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-8" data-testid="admin-tasks-header">
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <h2 className="text-2xl font-bold text-[var(--text-primary)]" data-testid="admin-tasks-title">任务管理</h2>
            {selected.size > 0 && (
              <button data-testid="task-mgmt-batch-cancel-btn" onClick={handleBatchCancel} style={batchBtnStyle}>
                批量取消 ({selected.size})
              </button>
            )}
          </div>
        </div>
        <div data-testid="task-mgmt-page-header" style={{ display: 'none' }} />

        {/* Toast */}
        {toast && (
          <div style={{ position: 'fixed', top: 20, right: 20, zIndex: 9999,
            background: toast.type === 'success' ? 'rgba(16,185,129,0.9)' : 'rgba(239,68,68,0.9)',
            color: '#fff', padding: '12px 20px', borderRadius: '8px', fontSize: '14px', fontWeight: 500,
          }}>
            {toast.message}
          </div>
        )}

        {/* Filter Tabs */}
        <div data-testid="task-mgmt-filter-tabs" style={{ display: 'flex', gap: '8px', marginBottom: '20px' }}>
          {[
            { key: 'running', label: '运行中' },
            { key: 'queued', label: '排队中' },
            { key: 'completed', label: '已完成' },
            { key: 'failed', label: '失败' },
            { key: 'cancelled', label: '已取消' },
            { key: 'all', label: '全部' },
          ].map((tab) => (
            <button key={tab.key} onClick={() => { setFilter(tab.key); setSelected(new Set()); }}
              style={{
                padding: '6px 16px', border: 'none', borderRadius: '8px', cursor: 'pointer',
                fontSize: '13px', fontWeight: 500,
                background: filter === tab.key ? 'var(--accent)' : 'rgba(255,255,255,0.06)',
                color: filter === tab.key ? '#fff' : '#7A7A7A',
              }}>
              {tab.label}
            </button>
          ))}
        </div>

        {/* Task Table */}
        <div className="glass" style={{ overflow: 'hidden' }}>
          <table data-testid="task-mgmt-table" style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ background: 'rgba(255,255,255,0.03)' }}>
                <th style={thStyle}><input type="checkbox" onChange={(e) => {
                  if (e.target.checked) setSelected(new Set(filteredTasks.map((t) => t.task_id)));
                  else setSelected(new Set());
                }} /></th>
                <th style={thStyle}>任务名称</th>
                <th style={thStyle}>状态</th>
                <th style={thStyle}>类型</th>
                <th style={thStyle}>发起人</th>
                <th style={thStyle}>创建时间</th>
                <th style={thStyle}>操作</th>
              </tr>
            </thead>
            <tbody>
              {filteredTasks.map((t) => (
                <tr key={t.task_id} data-testid={`task-mgmt-row-${t.task_id}`} style={{ borderBottom: '1px solid rgba(255,255,255,0.06)' }}>
                  <td style={tdStyle}>
                    <input data-testid="task-mgmt-batch-select" type="checkbox"
                      checked={selected.has(t.task_id)} onChange={() => toggleSelect(t.task_id)} />
                  </td>
                  <td style={tdStyle}>{t.task_id?.slice(0, 8)}...</td>
                  <td style={tdStyle}>
                    <span style={statusBadge(t.status)}>{statusLabel(t.status)}</span>
                  </td>
                  <td style={tdStyle}>{t.type || 'agent'}</td>
                  <td style={tdStyle} data-testid={`task-mgmt-creator-${t.task_id}`}>{t.user_id?.slice(0, 8) || '—'}</td>
                  <td style={tdStyle}>{t.created_at ? new Date(t.created_at).toLocaleString('zh-CN') : '—'}</td>
                  <td style={tdStyle}>
                    <div style={{ display: 'flex', gap: '6px' }}>
                      <button onClick={() => setDetailTask(detailTask?.task_id === t.task_id ? null : t)}
                        style={actionStyle('#5c7cfa')}>查看</button>
                      {t.status === 'running' && (
                        <button data-testid={`task-mgmt-cancel-btn-${t.task_id}`} onClick={() => handleCancel(t.task_id)}
                          style={actionStyle('#f59e0b')}>取消</button>
                      )}
                      {t.status === 'failed' && (
                        <button data-testid={`task-mgmt-retry-btn-${t.task_id}`} onClick={() => handleRetry(t.task_id)}
                          style={actionStyle('#10b981')}>重试</button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
              {filteredTasks.length === 0 && (
                <tr>
                  <td colSpan={7} style={{ ...tdStyle, textAlign: 'center', padding: '40px' }}>
                    <p className="text-sm text-[var(--text-secondary)]">暂无任务</p>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {/* Task Detail Panel */}
        {detailTask && (
          <div className="glass" style={{ marginTop: '20px', padding: '20px' }}>
            <h3 style={{ fontSize: '15px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '12px' }}>
              任务详情
            </h3>
            <div style={{ fontSize: '13px', color: '#7A7A7A', lineHeight: 2 }}>
              <p>ID: {detailTask.task_id}</p>
              <p>状态: {statusLabel(detailTask.status)}</p>
              <p>类型: {detailTask.type || '—'}</p>
              <p>用户: {detailTask.user_id || '—'}</p>
              <p>创建时间: {detailTask.created_at || '—'}</p>
              {detailTask.error && <p style={{ color: '#ef4444' }}>错误: {detailTask.error}</p>}
            </div>
          </div>
        )}
      </div>
    </AppLayout>
  );
}

const statusBadge = (s: string): React.CSSProperties => {
  const colors: Record<string, string> = {
    running: '#3b82f6', queued: '#8b5cf6', completed: '#10b981', failed: '#ef4444', cancelled: '#6b7280',
  };
  return { padding: '2px 10px', borderRadius: '10px', background: `${colors[s] || '#6b7280'}20`, color: colors[s] || '#6b7280', fontSize: '12px', fontWeight: 500 };
};

const statusLabel = (s: string) => {
  const m: Record<string, string> = { running: '运行中', queued: '排队中', completed: '已完成', failed: '失败', cancelled: '已取消', pending: '等待中' };
  return m[s] || s;
};

const thStyle: React.CSSProperties = { padding: '12px 10px', textAlign: 'left', fontSize: '11px', fontWeight: 700, textTransform: 'uppercase', color: '#666', borderBottom: '1px solid rgba(255,255,255,0.06)' };
const tdStyle: React.CSSProperties = { padding: '10px', fontSize: '13px', color: '#7A7A7A' };
const actionStyle = (color: string): React.CSSProperties => ({ background: 'transparent', border: `1px solid ${color}40`, color, borderRadius: '6px', padding: '3px 10px', fontSize: '12px', cursor: 'pointer' });
const batchBtnStyle: React.CSSProperties = { padding: '8px 16px', background: '#ef4444', color: '#fff', border: 'none', borderRadius: '8px', fontSize: '13px', fontWeight: 600, cursor: 'pointer' };
