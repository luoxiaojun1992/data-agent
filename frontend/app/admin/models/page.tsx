'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '../../../lib/api';

const MASK = '••••••••••';

// Canonical use cases exposed by backend (per-use-case defaults; one default per use case).
const USE_CASES: { value: string; label: string }[] = [
  { value: 'chat', label: 'Chat' },
  { value: 'task', label: 'Task' },
  { value: 'enhance', label: 'Enhance 增强' },
  { value: 'compaction', label: 'Compaction 压缩' },
  { value: 'kb_chunking', label: 'KB Chunking 索引' },
];

type ModelEntry = {
  id?: string;
  name?: string;
  type?: 'llm' | 'embedding';
  use_cases?: string[];
  is_default?: boolean;
  is_default_for?: string[];
  base_url?: string;
  instruction?: string;
  context_len?: number;
  max_tokens?: number;
  temperature?: number;
  api_key?: string;
  embedding_dim?: number;
};

function isEmbedding(m: ModelEntry): boolean {
  return (m.type || '').toLowerCase() === 'embedding';
}

// SVG icon for eye toggle (eye / eye-off)
// open=true  → key is currently visible → plain open eye (you can see it)
// open=false → key is currently masked   → eye with slash (closed/hidden)
function EyeIcon({ open }: { open: boolean }) {
  if (open) {
    return (
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
        <circle cx="12" cy="12" r="3" />
      </svg>
    );
  }
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
      <line x1="1" y1="1" x2="23" y2="23" />
    </svg>
  );
}

export default function ModelsPage() {
  const { auth, apiFetch } = useAuth();

  // List + UI state
  const [llmList, setLLMList] = useState<ModelEntry[]>([]);
  const [embeddingList, setEmbeddingList] = useState<ModelEntry[]>([]);
  const [llmTotal, setLLMTotal] = useState(0);
  const [llmPage, setLLMPage] = useState(1);
  const [embeddingTotal, setEmbeddingTotal] = useState(0);
  const [embeddingPage, setEmbeddingPage] = useState(1);
  const PAGE = 10;
  const [showAddModal, setShowAddModal] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editForm, setEditForm] = useState<any>(null);
  const [editActualKey, setEditActualKey] = useState<string | null>(null); // fetched plaintext, null=not loaded
  const [editKeyLoaded, setEditKeyLoaded] = useState(false);
  const [showEditKey, setShowEditKey] = useState(false);
  const [search, setSearch] = useState('');
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  // Hermes config state (独立卡片)
  const [hermesUrl, setHermesUrl] = useState('http://hermes:8081');
  const [hermesApiKey, setHermesApiKey] = useState('');
  const [hermesApiKeyExists, setHermesApiKeyExists] = useState(false);
  // `revealedHermesKey` doubles as the visibility flag: when non-null, the
  // stored Vault key is visible on screen; when null, the field is masked
  // (or empty). Hide / show toggles flip this single state.
  const [revealedHermesKey, setRevealedHermesKey] = useState<string | null>(null);

  const showToast = (msg: string, type: 'success' | 'error') => {
    setToast({ message: msg, type });
    setTimeout(() => setToast(null), 3000);
  };

  const fetchLLMList = useCallback(async () => {
    try {
      const size = search ? 100 : PAGE;
      const res = await apiFetch(`/models/list?page=${llmPage}&page_size=${size}`);
      if (res.ok) {
        const data = await res.json();
        setLLMList(data.models || []);
        setLLMTotal(data.total || 0);
      }
    } catch (err) {
      console.error('fetchLLMList:', err);
    }
  }, [apiFetch, llmPage, search]);

  const fetchEmbeddingList = useCallback(async () => {
    try {
      const size = search ? 100 : PAGE;
      const res = await apiFetch(`/admin/models/embedding?page=${embeddingPage}&page_size=${size}`);
      if (res.ok) {
        const data = await res.json();
        setEmbeddingList(data.models || []);
        setEmbeddingTotal(data.total || 0);
      }
    } catch (err) {
      console.error('fetchEmbeddingList:', err);
    }
  }, [apiFetch, embeddingPage, search]);

  const fetchHermesConfig = useCallback(async () => {
    try {
      const res = await apiFetch('/admin/models');
      if (res.ok) {
        const data = await res.json();
        const m = data.models || {};
        if (m.hermes_url) setHermesUrl(m.hermes_url);
        setHermesApiKeyExists(!!m.hermes_api_key_exists);
      }
    } catch { /* defaults */ }
  }, [apiFetch]);

  useEffect(() => {
    if (auth.hydrated) { fetchLLMList(); fetchEmbeddingList(); fetchHermesConfig(); }
  }, [auth.hydrated, fetchLLMList, fetchEmbeddingList, fetchHermesConfig]);

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

  // Hermes API key display: input.type tracks whether plaintext is on screen.
  // Plaintext = either the user typed a new value OR we decrypted from Vault.
  // When plaintext is showing, clicking the eye hides; when masked, click reveals.
  const hermesHasPlaintext = !!(hermesApiKey || revealedHermesKey);
  const hermesDisplay = hermesApiKey
    || revealedHermesKey
    || (hermesApiKeyExists ? MASK : '');
  const toggleHermesVisibility = () => {
    if (hermesHasPlaintext) {
      // Hide: clear both typed input and fetched plaintext
      setHermesApiKey('');
      setRevealedHermesKey(null);
      return;
    }
    // Nothing on screen — fetch from Vault when a stored key exists
    if (hermesApiKeyExists) revealHermesKey();
  };

  const deleteModel = async (id: string | undefined) => {
    if (!id) return;
    if (!confirm('确定删除该模型？')) return;
    try {
      const res = await apiFetch(`/admin/models/${id}`, { method: 'DELETE' });
      if (res.ok) { showToast('已删除', 'success'); fetchLLMList(); fetchEmbeddingList(); }
      else showToast('删除失败', 'error');
    } catch { showToast('删除失败', 'error'); }
  };

  const setDefaultModel = async (id: string, useCases: string[] = []) => {
    try {
      const res = await apiFetch(`/admin/models/${id}/default`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ use_cases: useCases }),
      });
      if (res.ok) {
        const msg = useCases.length ? `已设为默认 (${useCases.join('/')})` : '已设为默认';
        showToast(msg, 'success');
        fetchLLMList();
        fetchEmbeddingList();
      }
      else showToast('设置失败', 'error');
    } catch { showToast('设置失败', 'error'); }
  };

  const openEdit = async (m: ModelEntry) => {
    if (!m.id) return;
    setEditingId(m.id);
    setShowEditKey(false);
    setEditActualKey(null);
    setEditKeyLoaded(false);
    setEditForm({
      id: m.id,
      name: m.name || '',
      base_url: m.base_url || '',
      type: m.type || 'llm',
      instruction: m.instruction || '',
      api_key: MASK,
      context_len: String((m.context_len ?? 0) > 0 ? (m.context_len ?? 0) : 128000),
      max_tokens: String((m.max_tokens ?? 0) > 0 ? (m.max_tokens ?? 0) : 128000),
      temperature: String(m.temperature ?? 0.7),
      is_default: !!m.is_default,
      is_default_for: m.is_default_for || [],
      embedding_dim: String(m.embedding_dim ?? 768),
    });
    // Fetch the plaintext API key from Vault so the eye button in the modal
    // can actually toggle between plaintext and mask.
    try {
      const res = await apiFetch(`/admin/models/${m.id}/api-key`);
      if (res.ok) {
        const data = await res.json();
        setEditActualKey(data.plaintext || '');
        setEditKeyLoaded(true);
      }
    } catch { /* leave as MASK-only */ }
  };

  const closeEdit = () => {
    setEditingId(null);
    setEditForm(null);
    setShowEditKey(false);
    setEditActualKey(null);
    setEditKeyLoaded(false);
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
        context_len: parseInt(editForm.context_len, 10) || 128000,
        max_tokens: parseInt(editForm.max_tokens, 10) || 16000,
        temperature: parseFloat(editForm.temperature) || 0.7,
        embedding_dim: parseInt(editForm.embedding_dim, 10) || 768,
        is_default_for: editForm.is_default_for || [],
      };
      if (editForm.api_key && editForm.api_key !== MASK) body.api_key = editForm.api_key;
      const res = await apiFetch(`/admin/models/${editForm.id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (res.ok) {
        showToast('已保存', 'success');
        closeEdit();
        fetchLLMList();
        fetchEmbeddingList();
      }
      else { const d = await res.json().catch(() => ({})); showToast(d.error || '保存失败', 'error'); }
    } catch { showToast('保存失败', 'error'); }
  };

  const closeAdd = () => {
    setShowAddModal(false);
    setEditForm(null);
  };

  const openAdd = () => {
    setEditingId(null);
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
      is_default_for: [] as string[],
      embedding_dim: '768',
    });
    setEditActualKey(null);
    setEditKeyLoaded(false);
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
        embedding_dim: parseInt(editForm.embedding_dim, 10) || 768,
        is_default_for: editForm.is_default_for || [],
      };
      if (editForm.api_key && editForm.api_key !== MASK) body.api_key = editForm.api_key;
      const res = await apiFetch('/admin/models', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (res.ok) {
        showToast('已添加', 'success');
        closeAdd();
        fetchLLMList();
        fetchEmbeddingList();
      } else { const d = await res.json().catch(() => ({})); showToast(d.error || '添加失败', 'error'); }
    } catch { showToast('添加失败', 'error'); }
  };

  const saveHermes = async () => {
    try {
      const fields: { key: string; value: string }[] = [
        { key: 'hermes_url', value: hermesUrl },
      ];
      if (hermesApiKey && hermesApiKey !== MASK) {
        fields.push({ key: 'hermes_api_key', value: hermesApiKey });
      }
      for (const f of fields) {
        const res = await apiFetch('/admin/models', {
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

  const matches = (m: ModelEntry) =>
    !search ||
    m.name?.toLowerCase().includes(search.toLowerCase()) ||
    m.id?.toLowerCase().includes(search.toLowerCase());
  const filteredLLM = llmList.filter(matches);
  const filteredEmbedding = embeddingList.filter(matches);

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

        {/* LLM Model List Table */}
        <h3 className="text-sm font-semibold text-[var(--text-primary)] mb-2 mt-4">LLM 模型</h3>
        <div className="glass mb-6" style={{ padding: 0 }} data-testid="model-list-card">
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
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>默认 Use Case</th>
                  <th style={{ textAlign: 'right', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>操作</th>
                </tr>
              </thead>
              <tbody>
                {filteredLLM.length === 0 && (
                  <tr><td colSpan={8} style={{ padding: '20px', textAlign: 'center', color: 'var(--text-secondary)' }} data-testid="model-list-empty">
                    暂无模型 — 点击右上角「+ 新增模型」创建
                  </td></tr>
                )}
                {filteredLLM.map((m, i) => {
                  if (!m.id) return null;
                  const rowId = m.id;
                  const keyDisplay = m.api_key || '未设置';
                  return (
                    <tr key={rowId} style={{ borderBottom: '1px solid rgba(255,255,255,0.05)' }} data-testid={`model-list-row-${i}`}>
                      <td style={{ padding: '10px 12px' }}>
                        <div style={{ color: 'var(--text-primary)', fontWeight: 500 }}>{m.name}</div>
                        <div style={{ color: 'var(--text-secondary)', fontFamily: 'monospace', fontSize: '10px' }}>{rowId}</div>
                      </td>
                      <td style={{ padding: '10px 12px', color: 'var(--text-secondary)', fontFamily: 'monospace', fontSize: '11px' }}>{m.base_url || '-'}</td>
                      <td style={{ padding: '10px 12px' }}>
                        <code style={{ fontSize: '11px', color: 'var(--text-secondary)', maxWidth: '180px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {keyDisplay}
                        </code>
                      </td>
                      <td style={{ padding: '10px 12px', color: 'var(--text-secondary)', fontSize: '12px', maxWidth: '200px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {m.instruction || '-'}
                      </td>
                      <td style={{ padding: '10px 12px', color: 'var(--text-secondary)', textAlign: 'right' }}>{m.context_len?.toLocaleString() || '-'}</td>
                      <td style={{ padding: '10px 12px', color: 'var(--text-secondary)', textAlign: 'right' }}>{m.max_tokens?.toLocaleString() || '-'}</td>
                      <td style={{ padding: '10px 12px' }}>
                        <UseCaseChips
                          current={(m.is_default_for || []) as string[]}
                          isGlobal={!!m.is_default}
                          modelId={rowId}
                          onClear={() => setDefaultModel(rowId, USE_CASES.map(u => u.value))}
                          onToggle={(uc) => {
                            const set = new Set<string>(m.is_default_for || []);
                            if (set.has(uc)) set.delete(uc); else set.add(uc);
                            setDefaultModel(rowId, Array.from(set));
                          }}
                        />
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

        {!search && llmTotal > 0 && (
          <div style={{ display: 'flex', justifyContent: 'center', gap: '8px', marginTop: '12px' }}>
            <button onClick={() => setLLMPage(p => Math.max(1, p - 1))} disabled={llmPage === 1}
              style={{ padding: '4px 12px', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '6px', background: 'transparent', color: '#7A7A7A', fontSize: '12px', cursor: 'pointer' }}>上一页</button>
            <span style={{ padding: '4px 8px', fontSize: '12px', color: '#7A7A7A' }}>{llmPage} / {Math.ceil(llmTotal / PAGE)}（共 {llmTotal} 条）</span>
            <button onClick={() => setLLMPage(p => Math.min(Math.ceil(llmTotal / PAGE), p + 1))} disabled={llmPage >= Math.ceil(llmTotal / PAGE)}
              style={{ padding: '4px 12px', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '6px', background: 'transparent', color: '#7A7A7A', fontSize: '12px', cursor: 'pointer' }}>下一页</button>
          </div>
        )}

        {/* Embedding Model List */}
        <h3 className="text-sm font-semibold text-[var(--text-primary)] mb-2 mt-6">Embedding 模型</h3>
        <div className="glass mb-6" style={{ padding: 0 }} data-testid="embedding-list-card">
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', fontSize: '13px', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid rgba(255,255,255,0.1)' }}>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>模型</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>接口地址</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>密钥</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>默认</th>
                  <th style={{ textAlign: 'right', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>操作</th>
                </tr>
              </thead>
              <tbody>
                {filteredEmbedding.length === 0 && (
                  <tr><td colSpan={5} style={{ padding: '20px', textAlign: 'center', color: 'var(--text-secondary)' }} data-testid="embedding-list-empty">暂无 embedding 模型 — 新增模型时选 Embedding 类型</td></tr>
                )}
                {filteredEmbedding.map((m, i) => {
                  if (!m.id) return null;
                  const rowId = m.id;
                  const keyDisplay = m.api_key || '未设置';
                  return (
                    <tr key={rowId} style={{ borderBottom: '1px solid rgba(255,255,255,0.05)' }} data-testid={`embedding-list-row-${i}`}>
                      <td style={{ padding: '10px 12px' }}>
                        <div style={{ color: 'var(--text-primary)', fontWeight: 500 }}>{m.name}</div>
                        <div style={{ color: 'var(--text-secondary)', fontFamily: 'monospace', fontSize: '10px' }}>{rowId}</div>
                      </td>
                      <td style={{ padding: '10px 12px', color: 'var(--text-secondary)', fontFamily: 'monospace', fontSize: '11px' }}>{m.base_url || '-'}</td>
                      <td style={{ padding: '10px 12px' }}>
                        <code style={{ fontSize: '11px', color: 'var(--text-secondary)', maxWidth: '180px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{keyDisplay}</code>
                      </td>
                      <td style={{ padding: '10px 12px' }}>
                        {m.is_default ? (
                          embeddingList.filter(o => o.id !== rowId).length > 0 ? (
                            <select
                              data-testid={`embedding-list-switch-${i}`}
                              defaultValue=""
                              onChange={e => { if (e.target.value) setDefaultModel(e.target.value); e.target.value = ''; }}
                              style={{ background: 'transparent', border: '1px solid rgba(16,185,129,0.4)', borderRadius: '4px', padding: '2px 6px', color: '#10b981', fontSize: '11px', cursor: 'pointer' }}
                            >
                              <option value="" style={{ color: '#000' }}>✓ 默认 · 切换</option>
                              {embeddingList.filter(o => o.id !== rowId).map(o => (
                                <option key={o.id} value={o.id!} style={{ color: '#000' }}>{o.name || o.id}</option>
                              ))}
                            </select>
                          ) : (
                            <span style={{ color: '#10b981', fontSize: '11px' }}>✓ 默认</span>
                          )
                        ) : (
                          <button
                          data-testid={`embedding-list-set-default-${i}`}
                            onClick={() => setDefaultModel(rowId)}
                          style={{ background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '4px', padding: '2px 8px', color: 'var(--text-secondary)', cursor: 'pointer', fontSize: '11px' }}
                          >设为默认</button>
                        )}
                      </td>
                      <td style={{ padding: '10px 12px', textAlign: 'right' }}>
                        <button
                          data-testid={`embedding-list-edit-${i}`}
                          onClick={() => openEdit(m)}
                          style={{ background: 'transparent', border: 'none', color: 'var(--accent)', cursor: 'pointer', fontSize: '12px', marginRight: '8px' }}
                        >编辑</button>
                        <button
                          data-testid={`embedding-list-delete-${i}`}
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

        {!search && embeddingTotal > 0 && (
          <div style={{ display: 'flex', justifyContent: 'center', gap: '8px', marginTop: '12px' }}>
            <button onClick={() => setEmbeddingPage(p => Math.max(1, p - 1))} disabled={embeddingPage === 1}
              style={{ padding: '4px 12px', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '6px', background: 'transparent', color: '#7A7A7A', fontSize: '12px', cursor: 'pointer' }}>上一页</button>
            <span style={{ padding: '4px 8px', fontSize: '12px', color: '#7A7A7A' }}>{embeddingPage} / {Math.ceil(embeddingTotal / PAGE)}（共 {embeddingTotal} 条）</span>
            <button onClick={() => setEmbeddingPage(p => Math.min(Math.ceil(embeddingTotal / PAGE), p + 1))} disabled={embeddingPage >= Math.ceil(embeddingTotal / PAGE)}
              style={{ padding: '4px 12px', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '6px', background: 'transparent', color: '#7A7A7A', fontSize: '12px', cursor: 'pointer' }}>下一页</button>
          </div>
        )}

        {/* Hermes 独立配置卡片 */}
        <div className="glass" style={{ padding: '24px', marginTop: '20px' }} data-testid="hermes-config-card">
          <h3 style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '16px' }}>
            Hermes 配置
          </h3>

          <Field label="Hermes URL">
            <input data-testid="hermes-url-input" value={hermesUrl} onChange={e => setHermesUrl(e.target.value)} placeholder="http://hermes:8081" style={inputStyle} />
          </Field>

          <Field label="API Key">
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <input
                data-testid="hermes-api-key-input"
                type={hermesHasPlaintext ? 'text' : 'password'}
                value={hermesDisplay}
                onChange={e => {
                  setHermesApiKey(e.target.value);
                  // User started typing — drop the fetched plaintext so we
                  // don't save the stale fetched value when they hit 保存.
                  if (revealedHermesKey !== null) setRevealedHermesKey(null);
                }}
                placeholder={hermesApiKeyExists ? MASK : '输入 Hermes API Key'}
                style={{ ...inputStyle, flex: 1 }}
              />
              <button
                data-testid="hermes-api-key-eye-toggle"
                onClick={toggleHermesVisibility}
                title={hermesHasPlaintext ? '隐藏' : '查看明文'}
                style={{ background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '6px', padding: '6px 10px', cursor: 'pointer', color: 'var(--text-secondary)', display: 'inline-flex', alignItems: 'center' }}
              ><EyeIcon open={hermesHasPlaintext} /></button>
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

              <Field label="模型类型">
                <div style={{ display: 'flex', gap: '8px' }}>
                  {([
                    { value: 'llm', label: 'LLM' },
                    { value: 'embedding', label: 'Embedding' },
                  ] as const).map((opt) => {
                    const active = editForm.type === opt.value;
                    // Type is locked once a model is created — disabling the
                    // inactive tab prevents accidental type-change corruption.
                    const locked = editingId !== null && !active;
                    return (
                      <button
                        key={opt.value}
                        type="button"
                        data-testid={`model-edit-type-${opt.value}`}
                        disabled={locked}
                        onClick={() => !locked && setEditForm({ ...editForm, type: opt.value })}
                        style={{
                          flex: 1,
                          padding: '6px 12px',
                          fontSize: '12px',
                          borderRadius: '6px',
                          border: '1px solid',
                          borderColor: active ? 'var(--accent)' : 'rgba(255,255,255,0.15)',
                          background: active ? 'var(--accent)' : 'transparent',
                          color: active ? '#fff' : 'var(--text-primary)',
                          cursor: locked ? 'not-allowed' : 'pointer',
                          opacity: locked ? 0.4 : 1,
                        }}
                      >
                        {opt.label}
                      </button>
                    );
                  })}
                </div>
              </Field>

              <Field label="接口地址 (OpenAI 兼容)">
                <input data-testid="model-edit-base-url" value={editForm.base_url} onChange={e => setEditForm({ ...editForm, base_url: e.target.value })} placeholder="https://api.openai.com/v1" style={inputStyle} />
              </Field>

              <Field label="密钥">
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <input
                    data-testid="model-edit-api-key"
                    type={showEditKey ? 'text' : 'password'}
                    // Show fetched plaintext when toggled; otherwise MASK.
                    // editForm.api_key (user typing) takes precedence over both.
                    value={
                      showEditKey && editKeyLoaded && editActualKey !== null
                        ? editActualKey
                        : editForm.api_key
                    }
                    onChange={e => {
                      setEditForm({ ...editForm, api_key: e.target.value });
                      // User started typing — drop the fetched plaintext so
                      // we never silently re-save the old key.
                      if (editActualKey !== null) setEditActualKey(null);
                    }}
                    placeholder="留空保持原值"
                    style={{ ...inputStyle, flex: 1, fontFamily: showEditKey ? 'monospace' : 'inherit' }}
                  />
                  <button
                    data-testid="model-edit-api-key-eye"
                    onClick={() => setShowEditKey(!showEditKey)}
                    title={showEditKey ? '隐藏' : '显示明文'}
                    disabled={!editKeyLoaded}
                    style={{ background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '6px', padding: '6px 10px', cursor: editKeyLoaded ? 'pointer' : 'not-allowed', color: editKeyLoaded ? 'var(--text-secondary)' : 'rgba(255,255,255,0.2)', display: 'inline-flex', alignItems: 'center', opacity: editKeyLoaded ? 1 : 0.5 }}
                  ><EyeIcon open={showEditKey} /></button>
                </div>
                {!editKeyLoaded && (
                  <p className="text-[10px] text-[var(--text-secondary)] mt-1">加载中…</p>
                )}
              </Field>

              {editForm.type === 'embedding' && (
                <Field label="向量维度">
                  <input data-testid="model-edit-embedding-dim" type="number" step="128" min="1" value={editForm.embedding_dim} onChange={e => setEditForm({ ...editForm, embedding_dim: e.target.value })} placeholder="768" style={inputStyle} />
                </Field>
              )}

              {editForm.type !== 'embedding' && (
                <>
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
                </>
              )}

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

function UseCaseChips({
  current,
  isGlobal,
  modelId,
  onToggle,
  onClear,
}: {
  current: string[];
  isGlobal: boolean;
  modelId?: string;
  onToggle: (uc: string) => void;
  onClear: () => void;
}) {
  const selected = new Set(current || []);
  return (
    <div className="flex flex-wrap gap-1" data-testid={`model-list-usecases-${modelId || ''}`}>
      {USE_CASES.map((uc) => {
        const active = selected.has(uc.value);
        return (
          <button
            key={uc.value}
            type="button"
            data-testid={`model-list-uc-${modelId || ''}-${uc.value}`}
            onClick={() => onToggle(uc.value)}
            style={{
              fontSize: '10px',
              padding: '2px 8px',
              borderRadius: '999px',
              border: '1px solid',
              borderColor: active ? 'var(--accent)' : 'rgba(255,255,255,0.15)',
              background: active ? 'var(--accent)' : 'transparent',
              color: active ? '#fff' : 'var(--text-secondary)',
              cursor: 'pointer',
            }}
          >
            {uc.label}
          </button>
        );
      })}
      {(isGlobal || selected.size > 0) && (
        <button
          type="button"
          data-testid={`model-list-uc-clear-${modelId || ''}`}
          onClick={onClear}
          style={{ fontSize: '10px', padding: '2px 6px', borderRadius: '999px', background: 'transparent', border: '1px dashed rgba(255,255,255,0.2)', color: 'var(--text-secondary)', cursor: 'pointer' }}
          title="全选全部 use case"
        >全选</button>
      )}
    </div>
  );
}