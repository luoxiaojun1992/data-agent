'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import AppLayout from '../../providers';
import { useAuth } from '@/lib/api';

interface FeishuConfig {
  id: string;
  name: string;
  app_id: string;
  model_id: string;
  session_id: string;
  enabled: boolean;
  created_at: string;
}

interface ModelEntry {
  id: string;
  name: string;
  display_name?: string;
}

// Eye toggle SVG icon
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

export default function FeishuConfigPage() {
  const router = useRouter();
  const { auth, apiFetch } = useAuth();
  const [configs, setConfigs] = useState<FeishuConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [models, setModels] = useState<ModelEntry[]>([]);

  // Create modal
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [newCfg, setNewCfg] = useState({ name: '', app_id: '', app_secret: '', model_id: '' });

  // Edit modal
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editCfg, setEditCfg] = useState({ name: '', app_id: '', app_secret: '', enabled: true, model_id: '' });
  const [showSecret, setShowSecret] = useState(false);

  const loadModels = useCallback(async () => {
    try {
      const res = await apiFetch('/models/list?page=1&page_size=50');
      if (res.ok) {
        const data = await res.json();
        // /models/list already returns only LLM (Type==llm per SPEC-062)
        setModels(data.models || []);
      }
    } catch (e) { console.error('[feishu] loadModels failed:', e); }
  }, [apiFetch]);

  const loadConfigs = useCallback(async () => {
    setLoading(true);
    try {
      const res = await apiFetch('/im/feishu/configs?page=1&page_size=50');
      if (res.ok) {
        const data = await res.json();
        setConfigs(data.configs || []);
      }
    } catch (e) { console.error('[feishu] loadConfigs failed:', e); }
    setLoading(false);
  }, [apiFetch]);

  useEffect(() => {
    if (!auth.hydrated || !auth.token) return;
    loadConfigs();
    loadModels();
  }, [auth.hydrated, auth.token, loadConfigs, loadModels]);

  const createConfig = async () => {
    if (!newCfg.name.trim() || !newCfg.app_id.trim() || !newCfg.app_secret.trim()) return;
    try {
      const res = await apiFetch('/im/feishu/configs', { method: 'POST', body: JSON.stringify(newCfg) });
      if (res.ok) {
        setShowCreateModal(false);
        setNewCfg({ name: '', app_id: '', app_secret: '', model_id: '' });
        await loadConfigs();
      }
    } catch (e) { console.error('[feishu] create failed:', e); }
  };

  const openEdit = async (c: FeishuConfig) => {
    setEditingId(c.id);
    setShowSecret(false);
    // Load full config to get the real app_secret
    try {
      const res = await apiFetch('/im/feishu/configs/' + c.id);
      if (res.ok) {
        const data = await res.json();
        setEditCfg({ name: data.name, app_id: data.app_id, app_secret: data.app_secret || '', enabled: data.enabled, model_id: data.model_id });
        return;
      }
    } catch (e) { console.error('[feishu] load config failed:', e); }
    setEditCfg({ name: c.name, app_id: c.app_id, app_secret: '', enabled: c.enabled, model_id: c.model_id });
  };

  const saveEdit = async () => {
    if (!editingId) return;
    try {
      const body: any = {};
      if (editCfg.name) body.name = editCfg.name;
      if (editCfg.app_id) body.app_id = editCfg.app_id;
      if (editCfg.app_secret) body.app_secret = editCfg.app_secret;
      body.enabled = editCfg.enabled;
      const res = await apiFetch('/im/feishu/configs/' + editingId, { method: 'PUT', body: JSON.stringify(body) });
      if (res.ok) {
        setEditingId(null);
        await loadConfigs();
      }
    } catch (e) { console.error('[feishu] saveEdit failed:', e); }
  };

  const deleteConfig = async (id: string) => {
    if (!confirm('确定删除该配置？删除后将断开 WebSocket 连接。')) return;
    await apiFetch('/im/feishu/configs/' + id, { method: 'DELETE' });
    await loadConfigs();
  };

  const toggleEnabled = async (id: string, enabled: boolean) => {
    await apiFetch('/im/feishu/configs/' + id, { method: 'PUT', body: JSON.stringify({ enabled }) });
    await loadConfigs();
  };

  const formatDate = (d: string) => new Date(d).toLocaleString();

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-6 flex items-center justify-between" data-testid="feishu-header">
          <div>
            <button onClick={() => router.push('/im')} className="text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] mb-1">&larr; IM 集成</button>
            <h2 className="text-2xl font-bold text-[var(--text-primary)]">飞书机器人配置</h2>
            <p className="text-sm text-[var(--text-secondary)] mt-1">管理飞书机器人集成配置</p>
          </div>
          <button onClick={() => setShowCreateModal(true)}
            className="px-4 py-2 bg-[var(--accent)] text-white rounded-xl text-sm font-medium hover:opacity-90"
            data-testid="feishu-add-btn"
          >+ 新增配置</button>
        </div>

        {loading ? (
          <div className="text-center py-12 text-[var(--text-secondary)]" data-testid="feishu-loading">加载中...</div>
        ) : configs.length === 0 ? (
          <div className="glass p-12 text-center" data-testid="feishu-empty">
            <span className="text-5xl block mb-4">{'\ud83d\udc26'}</span>
            <p className="text-lg text-[var(--text-primary)] mb-2">暂无飞书配置</p>
            <p className="text-sm text-[var(--text-secondary)]">点击「+ 新增配置」创建飞书机器人集成</p>
          </div>
        ) : (
          <div className="space-y-3" data-testid="feishu-list">
            {configs.map((c) => (
              <div key={c.id} className="glass p-4" data-testid={'feishu-row-' + c.id}>
                <div className="flex items-center justify-between">
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <p className="text-sm font-medium text-[var(--text-primary)]">{c.name}</p>
                      <span className={'text-xs px-2 py-0.5 rounded-full ' + (c.enabled ? 'bg-emerald-400/10 text-emerald-400' : 'bg-gray-400/10 text-gray-400')}>
                        {c.enabled ? '已启用' : '已停用'}
                      </span>
                    </div>
                    <p className="text-xs text-[var(--text-secondary)] mt-1">
                      App: {c.app_id.slice(0, 8)}... · Session: {c.session_id?.slice(0, 12)} · {formatDate(c.created_at)}
                    </p>
                  </div>
                  <div className="flex items-center gap-2 flex-shrink-0">
                    {/* Enable/disable toggle */}
                    <label className="relative inline-flex items-center cursor-pointer" title={c.enabled ? '点击停用' : '点击启用'}>
                      <input type="checkbox" checked={c.enabled} onChange={(e) => toggleEnabled(c.id, e.target.checked)}
                        className="sr-only peer" />
                      <div className="w-9 h-5 bg-gray-500/30 rounded-full peer peer-checked:bg-emerald-400/50 peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-0.5 after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all" />
                    </label>
                    <button onClick={() => openEdit(c)}
                      className="px-2.5 py-1 text-xs rounded-lg border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                      data-testid={'feishu-edit-' + c.id}
                    >编辑</button>
                    <button onClick={() => deleteConfig(c.id)}
                      className="px-2 py-1 text-xs rounded-lg text-red-400 hover:bg-red-400/10"
                      data-testid={'feishu-delete-' + c.id}
                    >删除</button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Create Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center" data-testid="feishu-create-modal">
          <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={() => setShowCreateModal(false)} />
          <div className="relative glass p-6 rounded-2xl max-w-lg w-full mx-4">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">新增飞书配置</h3>
              <button onClick={() => setShowCreateModal(false)} className="text-[var(--text-secondary)] hover:text-[var(--text-primary)]">&times;</button>
            </div>
            <div className="space-y-3">
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">名称</label>
                <input type="text" value={newCfg.name} onChange={e => setNewCfg(p => ({ ...p, name: e.target.value }))}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)]"
                  data-testid="feishu-name-input" placeholder="例如：我的飞书机器人" />
              </div>
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">App ID</label>
                <input type="text" value={newCfg.app_id} onChange={e => setNewCfg(p => ({ ...p, app_id: e.target.value }))}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)]"
                  data-testid="feishu-appid-input" placeholder="cli_xxxxxxxx" />
              </div>
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">App Secret</label>
                <input type="password" value={newCfg.app_secret} onChange={e => setNewCfg(p => ({ ...p, app_secret: e.target.value }))}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)]"
                  data-testid="feishu-secret-input" placeholder="••••••••" />
              </div>
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">模型（可选，默认使用 Chat 模型）</label>
                <select value={newCfg.model_id} onChange={e => setNewCfg(p => ({ ...p, model_id: e.target.value }))}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)]"
                  data-testid="feishu-model-select">
                  <option value="">Chat 默认模型</option>
                  {models.map(m => (
                    <option key={m.id} value={m.id}>{m.display_name || m.name}</option>
                  ))}
                </select>
              </div>
              <div className="flex gap-3 pt-2">
                <button onClick={() => setShowCreateModal(false)}
                  className="flex-1 px-4 py-2 text-sm rounded-xl border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">取消</button>
                <button onClick={createConfig}
                  className="flex-1 px-4 py-2 text-sm rounded-xl bg-[var(--accent)] text-white hover:opacity-90 disabled:opacity-40"
                  data-testid="feishu-create-btn" disabled={!newCfg.name.trim() || !newCfg.app_id.trim() || !newCfg.app_secret.trim()}>创建配置</button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Edit Modal */}
      {editingId && (
        <div className="fixed inset-0 z-50 flex items-center justify-center" data-testid="feishu-edit-modal">
          <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={() => { setEditingId(null); setShowSecret(false); }} />
          <div className="relative glass p-6 rounded-2xl max-w-lg w-full mx-4">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold text-[var(--text-primary)]">编辑飞书配置</h3>
              <button onClick={() => { setEditingId(null); setShowSecret(false); }} className="text-[var(--text-secondary)] hover:text-[var(--text-primary)]">&times;</button>
            </div>
            <div className="space-y-3">
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">名称</label>
                <input type="text" value={editCfg.name} onChange={e => setEditCfg(p => ({ ...p, name: e.target.value }))}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)]" />
              </div>
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">App ID</label>
                <input type="text" value={editCfg.app_id} onChange={e => setEditCfg(p => ({ ...p, app_id: e.target.value }))}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)]" />
              </div>
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">App Secret</label>
                <div className="flex gap-2">
                  <input
                    type={showSecret ? 'text' : 'password'}
                    value={editCfg.app_secret}
                    onChange={e => setEditCfg(p => ({ ...p, app_secret: e.target.value }))}
                    placeholder="留空不修改"
                    className="flex-1 px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)]" />
                  <button onClick={() => setShowSecret(!showSecret)}
                    className="px-2 py-2 rounded-lg text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                    title={showSecret ? '隐藏' : '显示'}
                  ><EyeIcon open={showSecret} /></button>
                </div>
              </div>
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">模型</label>
                <select value={editCfg.model_id || ''} disabled
                  className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)]/50 border border-[var(--border-glass)] text-[var(--text-secondary)] cursor-not-allowed">
                  <option value="">Chat 默认模型</option>
                  {models.map(m => (
                    <option key={m.id} value={m.id}>{m.display_name || m.name}</option>
                  ))}
                </select>
                <p className="text-xs text-[var(--text-secondary)] mt-0.5">模型创建后不可修改</p>
              </div>
              <div className="flex items-center justify-between">
                <label className="text-sm text-[var(--text-primary)]">启用状态</label>
                <label className="relative inline-flex items-center cursor-pointer">
                  <input type="checkbox" checked={editCfg.enabled} onChange={e => setEditCfg(p => ({ ...p, enabled: e.target.checked }))}
                    className="sr-only peer" />
                  <div className="w-11 h-6 bg-gray-500/30 rounded-full peer peer-checked:bg-emerald-400/50 peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-0.5 after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all" />
                </label>
              </div>
              {!editCfg.enabled && (
                <p className="text-xs text-amber-400/80">停用或删除配置将断开 WebSocket 连接</p>
              )}
              <div className="flex gap-3 pt-2">
                <button onClick={() => { setEditingId(null); setShowSecret(false); }}
                  className="flex-1 px-4 py-2 text-sm rounded-xl border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">取消</button>
                <button onClick={saveEdit}
                  className="flex-1 px-4 py-2 text-sm rounded-xl bg-[var(--accent)] text-white hover:opacity-90"
                  data-testid="feishu-save-btn">保存</button>
              </div>
            </div>
          </div>
        </div>
      )}
    </AppLayout>
  );
}
