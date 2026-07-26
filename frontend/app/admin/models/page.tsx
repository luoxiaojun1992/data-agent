'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '../../../lib/api';

const MASK = '••••••••••';

// SVG icon for eye toggle (eye / eye-off)
function EyeIcon({ open }: { open: boolean }) {
  if (open) {
    return (
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
        <line x1="1" y1="1" x2="23" y2="23" />
      </svg>
    );
  }
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  );
}

export default function ModelsPage() {
  const { auth, apiFetch } = useAuth();

  // List + UI state
  const [modelList, setModelList] = useState<any[]>([]);
  const [revealedKeys, setRevealedKeys] = useState<Record<string, string>>({});
  const [showAddModal, setShowAddModal] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editForm, setEditForm] = useState<any>(null);
  const [showEditKey, setShowEditKey] = useState(false);
  const [search, setSearch] = useState('');
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  // Hermes config state (独立卡片)
  const [hermesUrl, setHermesUrl] = useState('http://hermes:8081');
  const [hermesModel, setHermesModel] = useState('hermes-3-70b');
  const [hermesApiKey, setHermesApiKey] = useState('');
  const [hermesApiKeyExists, setHermesApiKeyExists] = useState(false);
  const [showHermesKey, setShowHermesKey] = useState(false);
  const [revealedHermesKey, setRevealedHermesKey] = useState<string | null>(null);

  const showToast = (msg: string, type: 'success' | 'error') => {
    setToast({ message: msg, type });
    setTimeout(() => setToast(null), 3000);
  };

  const fetchModelList = useCallback(async () => {
    try {
      const res = await apiFetch('/models/list?page=1&page_size=100');
      if (res.ok) {
        const data = await res.json();
        setModelList(data.models || []);
      }
    } catch (err) {
      console.error('fetchModelList:', err);
    }
  }, [apiFetch]);

  const fetchHermesConfig = useCallback(async () => {
    try {
      const res = await apiFetch('/models');
      if (res.ok) {
        const data = await res.json();
        const m = data.models || {};
        if (m.hermes_url) setHermesUrl(m.hermes_url);
        setHermesApiKeyExists(!!m.hermes_api_key_exists);
      }
    } catch { /* defaults */ }
  }, [apiFetch]);

  useEffect(() => {
    if (auth.hydrated) { fetchModelList(); fetchHermesConfig(); }
  }, [auth.hydrated, fetchModelList, fetchHermesConfig]);

  const revealKey = async (id: string) => {
    if (revealedKeys[id]) {
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
        setTimeout(() => {
          setRevealedKeys(prev => {
            const c = { ...prev };
            delete c[id];
            return c;
          });
        }, 30000);
      } else showToast('解密失败', 'error');
    } catch { showToast('解密失败', 'error'); }
  };

  const revealHermesKey = async () => {
    if (revealedHermesKey !== null) {
      setRevealedHermesKey(null);
      return;
    }
    try {
      const res = await apiFetch('/vault/decrypt', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key: 'data-agent/hermes_api_key' }),
      });
      if (res.ok) {
        const data = await res.json();
        setRevealedHermesKey(data.plaintext);
        setTimeout(() => setRevealedHermesKey(null), 30000);
      } else showToast('解密失败', 'error');
    } catch { showToast('解密失败', 'error'); }
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
      if (res.ok) { showToast('已设为默认', 'success'); fetchModelList(); }
      else showToast('设置失败', 'error');
    } catch { showToast('设置失败', 'error'); }
  };

  const openEdit = (m: any) => {
    setEditingId(m.id);
    setShowEditKey(false);
    setEditForm({
      id: m.id,
      name: m.name || '',
      base_url: m.base_url || '',
      type: m.type || 'llm',
      instruction: m.instruction || '',
      api_key: MASK,
      context_len: String(m.context_len ?? 128000),
      max_tokens: String(m.max_tokens ?? 16000),
      temperature: String(m.temperature ?? 0.7),
      is_default: !!m.is_default,
      is_default_for: m.is_default_for || [],
    });
  };

  const closeEdit = () => { setEditingId(null); setEditForm(null); setShowEditKey(false); };

  const saveEdit = async () => {
    if (!editForm) return;
    if (!editForm.name.trim()) { showToast('名称必填', 'error'); return; }
    try {
      const body: any = {
        name: editForm.name,
        base_url: editForm.base_url,
        type: editForm.type,
        instruction: editForm.instruction,
        context_len: parseInt(editForm.context_len, 10) || 128000,
        max_tokens: parseInt(editForm.max_tokens, 10) || 16000,
        temperature: parseFloat(editForm.temperature) || 0.7,
      };
      if (editForm.api_key && editForm.api_key !== MASK) body.api_key = editForm.api_key;
      const res = await apiFetch(`/models/${editForm.id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (res.ok) { showToast('已保存', 'success'); closeEdit(); fetchModelList(); }
      else { const d = await res.json().catch(() => ({})); showToast(d.error || '保存失败', 'error'); }
    } catch { showToast('保存失败', 'error'); }
  };

  const openAdd = () => {
    setShowEditKey(false);
    setEditForm({
      id: '',
      name: '',
      base_url: 'https://api.openai.com/v1',
      type: 'llm',
      instruction: '',
      api_key: '',
      context_len: '128000',
      max_tokens: '128000',
      temperature: '0.7',
      is_default: false,
      is_default_for: [],
    });
    setShowAddModal(true);
  };

  const addModel = async () => {
    if (!editForm || !editForm.name.trim()) { showToast('名称必填', 'error'); return; }
    try {
      const body: any = {
        name: editForm.name,
        base_url: editForm.base_url,
        type: editForm.type,
        instruction: editForm.instruction,
        context_len: parseInt(editForm.context_len, 10) || 128000,
        max_tokens: parseInt(editForm.max_tokens, 10) || 128000,
        temperature: parseFloat(editForm.temperature) || 0.7,
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
      } else { const d = await res.json().catch(() => ({})); showToast(d.error || '添加失败', 'error'); }
    } catch { showToast('添加失败', 'error'); }
  };

  const saveHermes = async () => {
    try {
      const fields: { key: string; value: string }[] = [
        { key: 'hermes_url', value: hermesUrl },
        { key: 'hermes_model', value: hermesModel },
      ];
      if (hermesApiKey && hermesApiKey !== MASK) {
        fields.push({ key: 'hermes_api_key', value: hermesApiKey });
      }
      for (const f of fields) {
        const res = await apiFetch('/models', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(f),
        });
        if (!res.ok) { const d = await res.json().catch(() => ({})); throw new Error(d.error || `保存 ${f.key} 失败`); }
      }
      showToast('Hermes 配置已保存', 'success');
      setHermesApiKey('');
      setRevealedHermesKey(null);
      fetchHermesConfig();
    } catch (e: any) { showToast(e?.message || '保存失败', 'error'); }
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
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">模型配置</h2>
          <button
            data-testid="model-add-btn"
            onClick={openAdd}
            className="px-4 py-2 text-sm rounded-lg bg-[var(--accent)] text-white hover:opacity-90 transition-opacity"
          >+ 新增模型</button>
        </div>

        {/* Toast */}
        {toast && (
          <div className="fixed top-5 right-5 z-[200] px-4 py-2 rounded-lg text-white text-sm font-medium shadow-lg"
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
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>模型</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>接口地址</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>密钥</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>系统提示词</th>
                  <th style={{ textAlign: 'right', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>上下文</th>
                  <th style={{ textAlign: 'right', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>最大输出</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>默认</th>
                  <th style={{ textAlign: 'right', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>操作</th>
                </tr>
              </thead>
              <tbody>
                {filtered.length === 0 && (
                  <tr><td colSpan={8} style={{ padding: '20px', textAlign: 'center', color: 'var(--text-secondary)' }} data-testid="model-list-empty">
                    暂无模型 — 点击右上角「+ 新增模型」创建
                  </td></tr>
                )}
                {filtered.map((m, i) => {
                  const isRevealed = !!revealedKeys[m.id];
                  const hasKey = !!m.api_key;
                  const keyDisplay = isRevealed ? revealedKeys[m.id] : (hasKey ? MASK : '未设置');
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
                            style={{ background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '4px', padding: '4px', cursor: 'pointer', color: 'var(--text-secondary)', display: 'inline-flex', alignItems: 'center' }}
                          ><EyeIcon open={isRevealed} /></button>
                        </div>
                      </td>
                      <td style={{ padding: '10px 12px', color: 'var(--text-secondary)', fontSize: '12px', maxWidth: '200px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {m.instruction || '-'}
                      </td>
                      <td style={{ padding: '10px 12px', color: 'var(--text-secondary)', textAlign: 'right' }}>{m.context_len?.toLocaleString() || '-'}</td>
                      <td style={{ padding: '10px 12px', color: 'var(--text-secondary)', textAlign: 'right' }}>{m.max_tokens?.toLocaleString() || '-'}</td>
                      <td style={{ padding: '10px 12px' }}>
                        {m.is_default ? (
                          <span data-testid={`model-list-default-${i}`} style={{ color: '#10b981', fontSize: '11px' }}>✓ 全局</span>
                        ) : (m.is_default_for && m.is_default_for.length > 0) ? (
                          <span style={{ color: '#5c7cfa', fontSize: '11px' }}>✓ {m.is_default_for.join('/')}</span>
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

        {/* Hermes 独立配置卡片 */}
        <div className="glass" style={{ padding: '24px', marginTop: '20px' }} data-testid="hermes-config-card">
          <h3 style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '16px' }}>
            Hermes 配置
          </h3>

          <Field label="Hermes URL">
            <input data-testid="hermes-url-input" value={hermesUrl} onChange={e => setHermesUrl(e.target.value)} placeholder="http://hermes:8081" style={inputStyle} />
          </Field>

          <Field label="默认模型">
            <input data-testid="hermes-model-input" value={hermesModel} onChange={e => setHermesModel(e.target.value)} placeholder="hermes-3-70b" style={inputStyle} />
          </Field>

          <Field label="API Key">
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <input
                data-testid="hermes-api-key-input"
                type={showHermesKey ? 'text' : 'password'}
                value={hermesApiKey || (revealedHermesKey ?? (hermesApiKeyExists ? MASK : ''))}
                onChange={e => setHermesApiKey(e.target.value)}
                placeholder={hermesApiKeyExists ? MASK : '输入 Hermes API Key'}
                style={{ ...inputStyle, flex: 1 }}
              />
              <button
                data-testid="hermes-api-key-eye-toggle"
                onClick={() => { if (hermesApiKey) { setShowHermesKey(!showHermesKey); } else { revealHermesKey(); } }}
                title="显示明文"
                style={{ background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '6px', padding: '6px 10px', cursor: 'pointer', color: 'var(--text-secondary)', display: 'inline-flex', alignItems: 'center' }}
              ><EyeIcon open={showHermesKey || revealedHermesKey !== null} /></button>
            </div>
          </Field>

          <div className="flex justify-end mt-2">
            <button
              data-testid="hermes-save-btn"
              onClick={saveHermes}
              className="px-4 py-2 text-sm rounded-lg bg-[var(--accent)] text-white hover:opacity-90"
            >保存</button>
          </div>
        </div>

        {/* Edit / Add Modal */}
        {(editingId !== null || showAddModal) && editForm && (
          <div className="fixed inset-0 z-[100] flex items-center justify-center" data-testid="model-edit-modal">
            <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" onClick={closeEdit} />
            <div className="relative bg-[var(--bg-secondary)] border border-[var(--border-glass)] p-6 rounded-2xl max-w-lg w-full mx-4 space-y-3 max-h-[90vh] overflow-y-auto shadow-2xl">
              <h3 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '8px' }}>
                {editingId !== null ? '编辑模型' : '新增模型'}
              </h3>

              <Field label="名称">
                <input data-testid="model-edit-name" value={editForm.name} onChange={e => setEditForm({ ...editForm, name: e.target.value })} placeholder="如 gpt-4o / deepseek-v4-pro" style={inputStyle} />
              </Field>

              <Field label="接口地址 (OpenAI 兼容)">
                <input data-testid="model-edit-base-url" value={editForm.base_url} onChange={e => setEditForm({ ...editForm, base_url: e.target.value })} placeholder="https://api.openai.com/v1" style={inputStyle} />
              </Field>

              <Field label="密钥">
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <input
                    data-testid="model-edit-api-key"
                    type={showEditKey ? 'text' : 'password'}
                    value={editForm.api_key}
                    onChange={e => setEditForm({ ...editForm, api_key: e.target.value })}
                    placeholder="留空保持原值"
                    style={{ ...inputStyle, flex: 1 }}
                  />
                  <button
                    data-testid="model-edit-api-key-eye"
                    onClick={() => setShowEditKey(!showEditKey)}
                    title={showEditKey ? '隐藏' : '显示'}
                    style={{ background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '6px', padding: '6px 10px', cursor: 'pointer', color: 'var(--text-secondary)', display: 'inline-flex', alignItems: 'center' }}
                  ><EyeIcon open={showEditKey} /></button>
                </div>
              </Field>

              <Field label="系统提示词">
                <textarea data-testid="model-edit-instruction" value={editForm.instruction} onChange={e => setEditForm({ ...editForm, instruction: e.target.value })} placeholder="系统提示词（可选）" style={{ ...inputStyle, minHeight: '80px', resize: 'vertical' }} />
              </Field>

              <div className="grid grid-cols-3 gap-3">
                <Field label="上下文长度">
                  <input data-testid="model-edit-context-len" type="number" step="1000" min="1" value={editForm.context_len} onChange={e => setEditForm({ ...editForm, context_len: e.target.value })} style={inputStyle} />
                </Field>
                <Field label="最大输出">
                  <input data-testid="model-edit-max-tokens" type="number" step="1000" min="1" value={editForm.max_tokens} onChange={e => setEditForm({ ...editForm, max_tokens: e.target.value })} style={inputStyle} />
                </Field>
                <Field label="Temperature">
                  <input data-testid="model-edit-temperature" type="number" step="0.1" min="0" max="2" value={editForm.temperature} onChange={e => setEditForm({ ...editForm, temperature: e.target.value })} style={inputStyle} />
                </Field>
              </div>

              <div className="flex gap-3 justify-end pt-2">
                <button onClick={closeEdit} className="px-4 py-2 text-sm rounded-lg border border-[var(--border-glass)] text-[var(--text-primary)] hover:bg-white/5">取消</button>
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
    <div style={{ marginBottom: '12px' }}>
      <label style={{ display: 'block', fontSize: '12px', color: 'var(--text-secondary)', marginBottom: '4px' }}>
        {label}
      </label>
      {children}
    </div>
  );
}