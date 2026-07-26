'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '../../../lib/api';

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1';
const MASK = '••••••••••';

export default function ModelsPage() {
  const { auth, apiFetch } = useAuth();

  // List + UI state
  const [modelList, setModelList] = useState<any[]>([]);
  const [revealedKeys, setRevealedKeys] = useState<Record<string, string>>({});
  const [showAddModal, setShowAddModal] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editForm, setEditForm] = useState<any>(null);
  const [search, setSearch] = useState('');
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  const showToast = (msg: string, type: 'success' | 'error') => {
    setToast({ message: msg, type });
    setTimeout(() => setToast(null), 3000);
  };

  const fetchModelList = useCallback(async () => {
    try {
      const res = await apiFetch('/models?page=1&page_size=100');
      if (res.ok) {
        const data = await res.json();
        setModelList(data.models || []);
      }
    } catch (err) {
      console.error('fetchModelList:', err);
    }
  }, [apiFetch]);

  useEffect(() => {
    if (auth.hydrated) fetchModelList();
  }, [auth.hydrated, fetchModelList]);

  const revealKey = async (id: string) => {
    if (revealedKeys[id]) {
      // Toggle off
      setRevealedKeys(prev => {
        const c = { ...prev };
        delete c[id];
        return c;
      });
      return;
    }
    try {
      const res = await apiFetch(`/models/${id}/api-key`);
      if (res.ok) {
        const data = await res.json();
        setRevealedKeys(prev => ({ ...prev, [id]: data.plaintext }));
        // Auto-hide after 30s
        setTimeout(() => {
          setRevealedKeys(prev => {
            const c = { ...prev };
            delete c[id];
            return c;
          });
        }, 30000);
      } else {
        showToast('解密失败', 'error');
      }
    } catch {
      showToast('解密失败', 'error');
    }
  };

  const deleteModel = async (id: string) => {
    if (!confirm('确定删除该模型？')) return;
    try {
      const res = await apiFetch(`/models/${id}`, { method: 'DELETE' });
      if (res.ok) { showToast('已删除', 'success'); fetchModelList(); }
      else showToast('删除失败', 'error');
    } catch { showToast('删除失败', 'error'); }
  };

  const setDefaultModel = async (id: string, useCases: string[] = []) => {
    try {
      const res = await apiFetch(`/models/${id}/default`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ use_cases: useCases }),
      });
      if (res.ok) {
        showToast('已设为默认', 'success');
        fetchModelList();
      } else showToast('设置失败', 'error');
    } catch { showToast('设置失败', 'error'); }
  };

  const openEdit = (m: any) => {
    setEditingId(m.id);
    setEditForm({
      id: m.id,
      name: m.name || '',
      base_url: m.base_url || '',
      type: m.type || 'llm',
      instruction: m.instruction || '',
      api_key: MASK, // always masked; user clears field to keep
      temperature: String(m.temperature ?? 0.7),
      max_tokens: String(m.max_tokens ?? 16000),
      is_default: !!m.is_default,
      is_default_for: m.is_default_for || [],
    });
  };

  const closeEdit = () => {
    setEditingId(null);
    setEditForm(null);
  };

  const saveEdit = async () => {
    if (!editForm) return;
    if (!editForm.name.trim()) { showToast('名称必填', 'error'); return; }
    try {
      const body: any = {
        name: editForm.name,
        base_url: editForm.base_url,
        type: editForm.type,
        instruction: editForm.instruction,
        temperature: parseFloat(editForm.temperature) || 0.7,
        max_tokens: parseInt(editForm.max_tokens, 10) || 16000,
      };
      // Only send api_key when user changed it (not the masked value)
      if (editForm.api_key && editForm.api_key !== MASK) {
        body.api_key = editForm.api_key;
      }
      const res = await apiFetch(`/models/${editForm.id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (res.ok) {
        showToast('已保存', 'success');
        closeEdit();
        fetchModelList();
      } else {
        const d = await res.json().catch(() => ({}));
        showToast(d.error || '保存失败', 'error');
      }
    } catch { showToast('保存失败', 'error'); }
  };

  const addModel = async () => {
    if (!editForm || !editForm.name.trim()) { showToast('名称必填', 'error'); return; }
    try {
      const body: any = {
        name: editForm.name,
        base_url: editForm.base_url,
        type: editForm.type,
        instruction: editForm.instruction,
        temperature: parseFloat(editForm.temperature) || 0.7,
        max_tokens: parseInt(editForm.max_tokens, 10) || 16000,
      };
      if (editForm.api_key && editForm.api_key !== MASK) body.api_key = editForm.api_key;
      const res = await apiFetch('/models', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (res.ok) {
        showToast('已添加', 'success');
        setShowAddModal(false);
        setEditForm(null);
        fetchModelList();
      } else {
        const d = await res.json().catch(() => ({}));
        showToast(d.error || '添加失败', 'error');
      }
    } catch { showToast('添加失败', 'error'); }
  };

  const openAdd = () => {
    setEditForm({
      id: '',
      name: '',
      base_url: 'https://api.openai.com/v1',
      type: 'llm',
      instruction: '',
      api_key: '',
      temperature: '0.7',
      max_tokens: '128000',
      is_default: false,
      is_default_for: [],
    });
    setShowAddModal(true);
  };

  const filtered = modelList.filter(m =>
    !search ||
    m.name?.toLowerCase().includes(search.toLowerCase()) ||
    m.id?.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-6 flex items-center justify-between">
          <div>
            <h2 className="text-2xl font-bold text-[var(--text-primary)]">模型配置</h2>
            <p className="text-sm text-[var(--text-secondary)] mt-1">
              每个模型独立管理 API Key（Vault 加密），系统进程按 use case 选择默认模型
            </p>
          </div>
          <button
            data-testid="model-add-btn"
            onClick={openAdd}
            className="px-4 py-2 text-sm rounded-lg bg-[var(--accent)] text-white hover:opacity-90 transition-opacity"
          >+ 新增模型</button>
        </div>

        {/* Toast */}
        {toast && (
          <div className="fixed top-5 right-5 z-50 px-4 py-2 rounded-lg text-white text-sm font-medium shadow-lg"
            style={{ background: toast.type === 'success' ? 'rgba(16,185,129,0.95)' : 'rgba(239,68,68,0.95)' }}>
            {toast.message}
          </div>
        )}

        {/* Search */}
        <input
          type="text"
          placeholder="搜索模型..."
          value={search}
          onChange={e => setSearch(e.target.value)}
          className="w-full mb-3 px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] placeholder-[var(--text-secondary)] focus:outline-none"
          data-testid="model-search"
        />

        {/* Model List Table */}
        <div className="glass" style={{ padding: 0 }} data-testid="model-list-card">
          <div style={{ overflowX: 'auto' }} data-testid="model-list-table">
            <table style={{ width: '100%', fontSize: '13px', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid rgba(255,255,255,0.1)' }}>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>名称 / Model Name</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>Base URL</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>API Key</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>提示词</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>Max Tokens</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>默认</th>
                  <th style={{ textAlign: 'right', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>操作</th>
                </tr>
              </thead>
              <tbody>
                {filtered.length === 0 && (
                  <tr><td colSpan={7} style={{ padding: '20px', textAlign: 'center', color: 'var(--text-secondary)' }} data-testid="model-list-empty">
                    暂无模型 — 点击右上角「+ 新增模型」创建
                  </td></tr>
                )}
                {filtered.map((m, i) => {
                  const isRevealed = !!revealedKeys[m.id];
                  const keyDisplay = isRevealed ? revealedKeys[m.id] : (m.api_key_exists ? MASK : <span style={{ color: 'var(--text-secondary)' }}>未设置</span>);
                  return (
                    <tr key={m.id || i} style={{ borderBottom: '1px solid rgba(255,255,255,0.05)' }} data-testid={`model-list-row-${i}`}>
                      <td style={{ padding: '10px 12px' }}>
                        <div style={{ color: 'var(--text-primary)', fontWeight: 500 }}>{m.name}</div>
                        <div style={{ color: 'var(--text-secondary)', fontFamily: 'monospace', fontSize: '10px' }}>{m.id}</div>
                      </td>
                      <td style={{ padding: '10px 12px', color: 'var(--text-secondary)', fontFamily: 'monospace', fontSize: '11px' }}>{m.base_url || '-'}</td>
                      <td style={{ padding: '10px 12px' }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                          <code style={{ fontSize: '11px', color: 'var(--text-secondary)', maxWidth: '180px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                            {keyDisplay}
                          </code>
                          <button
                            data-testid={`model-list-key-eye-${i}`}
                            onClick={() => revealKey(m.id)}
                            title={isRevealed ? '隐藏' : '查看明文'}
                            style={{ background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '4px', padding: '2px 6px', cursor: 'pointer', fontSize: '12px' }}
                          >{isRevealed ? '🙈' : '👁'}</button>
                        </div>
                      </td>
                      <td style={{ padding: '10px 12px', color: 'var(--text-secondary)', fontSize: '12px', maxWidth: '200px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {m.instruction || '-'}
                      </td>
                      <td style={{ padding: '10px 12px', color: 'var(--text-secondary)' }}>{m.max_tokens}</td>
                      <td style={{ padding: '10px 12px' }}>
                        {m.is_default ? (
                          <span data-testid={`model-list-default-${i}`} style={{ color: '#10b981', fontSize: '11px' }}>
                            ✓ 全局
                          </span>
                        ) : (m.is_default_for && m.is_default_for.length > 0) ? (
                          <span style={{ color: '#5c7cfa', fontSize: '11px' }}>
                            ✓ {m.is_default_for.join('/')}
                          </span>
                        ) : (
                          <button
                            data-testid={`model-list-set-default-${i}`}
                            onClick={() => setDefaultModel(m.id)}
                            style={{ background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '4px', padding: '2px 8px', color: 'var(--text-secondary)', cursor: 'pointer', fontSize: '11px' }}
                          >设为默认</button>
                        )}
                      </td>
                      <td style={{ padding: '10px 12px', textAlign: 'right' }}>
                        <button
                          data-testid={`model-list-edit-${i}`}
                          onClick={() => openEdit(m)}
                          style={{ background: 'transparent', border: 'none', color: 'var(--accent)', cursor: 'pointer', fontSize: '12px', marginRight: '8px' }}
                        >编辑</button>
                        <button
                          data-testid={`model-list-delete-${i}`}
                          onClick={() => deleteModel(m.id)}
                          style={{ background: 'transparent', border: 'none', color: '#ef4444', cursor: 'pointer', fontSize: '12px' }}
                        >删除</button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>

        {/* Edit / Add Modal */}
        {(editingId !== null || showAddModal) && editForm && (
          <div className="fixed inset-0 z-50 flex items-center justify-center" data-testid="model-edit-modal">
            <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={closeEdit} />
            <div className="relative glass p-6 rounded-2xl max-w-lg w-full mx-4 space-y-3 max-h-[90vh] overflow-y-auto">
              <h3 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)' }}>
                {editingId !== null ? '编辑模型' : '新增模型'}
              </h3>

              <Field label="名称 (Model Name)">
                <input data-testid="model-edit-name" value={editForm.name} onChange={e => setEditForm({ ...editForm, name: e.target.value })} placeholder="如 gpt-4o / deepseek-v4-pro"
                  style={inputStyle} />
              </Field>

              <Field label="Base URL (OpenAI 兼容)">
                <input data-testid="model-edit-base-url" value={editForm.base_url} onChange={e => setEditForm({ ...editForm, base_url: e.target.value })} placeholder="https://api.openai.com/v1"
                  style={inputStyle} />
              </Field>

              <Field label="API Key">
                <input data-testid="model-edit-api-key" value={editForm.api_key} onChange={e => setEditForm({ ...editForm, api_key: e.target.value })} placeholder="留空保持原值"
                  style={inputStyle} />
                <p className="text-[10px] text-[var(--text-secondary)] mt-1">
                  输入新值覆盖；留空不修改；显示 {MASK} 表示已设置
                </p>
              </Field>

              <Field label="系统提示词 (System Prompt)">
                <textarea data-testid="model-edit-instruction" value={editForm.instruction} onChange={e => setEditForm({ ...editForm, instruction: e.target.value })} placeholder="系统提示词（可选）"
                  style={{ ...inputStyle, minHeight: '80px', resize: 'vertical' }} />
              </Field>

              <div className="grid grid-cols-2 gap-3">
                <Field label="Temperature">
                  <input data-testid="model-edit-temperature" type="number" step="0.1" min="0" max="2" value={editForm.temperature} onChange={e => setEditForm({ ...editForm, temperature: e.target.value })}
                    style={inputStyle} />
                </Field>
                <Field label="Max Tokens (最大输出)">
                  <input data-testid="model-edit-max-tokens" type="number" step="1000" min="1" value={editForm.max_tokens} onChange={e => setEditForm({ ...editForm, max_tokens: e.target.value })}
                    style={inputStyle} />
                </Field>
              </div>

              <div className="flex gap-3 justify-end pt-2">
                <button onClick={closeEdit} className="px-4 py-2 text-sm rounded-lg border border-[var(--border-glass)] text-[var(--text-primary)] hover:bg-white/5">
                  取消
                </button>
                <button
                  data-testid="model-edit-save-btn"
                  onClick={editingId !== null ? saveEdit : addModel}
                  className="px-4 py-2 text-sm rounded-lg bg-[var(--accent)] text-white hover:opacity-90"
                >保存</button>
              </div>
            </div>
          </div>
        )}
      </div>
    </AppLayout>
  );
}

const inputStyle: React.CSSProperties = {
  width: '100%',
  padding: '8px 12px',
  fontSize: '13px',
  borderRadius: '6px',
  background: 'rgba(255,255,255,0.05)',
  border: '1px solid rgba(255,255,255,0.1)',
  color: 'var(--text-primary)',
  outline: 'none',
};

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label style={{ display: 'block', fontSize: '12px', color: 'var(--text-secondary)', marginBottom: '4px' }}>
        {label}
      </label>
      {children}
    </div>
  );
}