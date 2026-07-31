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

export default function FeishuConfigPage() {
  const router = useRouter();
  const { auth, apiFetch } = useAuth();
  const [configs, setConfigs] = useState<FeishuConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [newCfg, setNewCfg] = useState({ name: '', app_id: '', app_secret: '', model_id: '' });

  const loadConfigs = useCallback(async () => {
    setLoading(true);
    try {
      const res = await apiFetch('/im/feishu/configs?page=1&page_size=20');
      if (res.ok) {
        const data = await res.json();
        setConfigs(data.configs || []);
      }
    } catch (e) {
      console.error('[feishu] loadConfigs failed:', e);
    }
    setLoading(false);
  }, [apiFetch]);

  useEffect(() => {
    if (!auth.hydrated || !auth.token) return;
    loadConfigs();
  }, [loadConfigs, auth.hydrated, auth.token]);

  const createConfig = async () => {
    if (!newCfg.name.trim() || !newCfg.app_id.trim() || !newCfg.app_secret.trim()) return;
    try {
      const res = await apiFetch('/im/feishu/configs', {
        method: 'POST',
        body: JSON.stringify(newCfg),
      });
      if (res.ok) {
        setShowModal(false);
        setNewCfg({ name: '', app_id: '', app_secret: '', model_id: '' });
        await loadConfigs();
      }
    } catch (e) {
      console.error('[feishu] create failed:', e);
    }
  };

  const toggleEnabled = async (id: string, enabled: boolean) => {
    await apiFetch('/im/feishu/configs/' + id, {
      method: 'PUT',
      body: JSON.stringify({ enabled }),
    });
    await loadConfigs();
  };

  const deleteConfig = async (id: string) => {
    if (!confirm('确定删除该配置？')) return;
    await apiFetch('/im/feishu/configs/' + id, { method: 'DELETE' });
    await loadConfigs();
  };

  const statusPill = (enabled: boolean) => enabled
    ? { label: '已启用', cls: 'text-emerald-400 bg-emerald-400/10' }
    : { label: '已停用', cls: 'text-gray-400 bg-gray-400/10' };

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-6 flex items-center justify-between" data-testid="feishu-header">
          <div>
            <button onClick={() => router.push('/im')} className="text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] mb-1">&larr; IM 集成</button>
            <h2 className="text-2xl font-bold text-[var(--text-primary)]">飞书机器人配置</h2>
            <p className="text-sm text-[var(--text-secondary)] mt-1">管理飞书机器人集成配置</p>
          </div>
          <button onClick={() => setShowModal(true)}
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
                  <div>
                    <p className="text-sm font-medium text-[var(--text-primary)]">{c.name}</p>
                    <p className="text-xs text-[var(--text-secondary)] mt-1">
                      App: {c.app_id.slice(0, 8)}... · Session: {c.session_id?.slice(0, 12)} · {new Date(c.created_at).toLocaleString()}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => toggleEnabled(c.id, !c.enabled)}
                      className={'px-2.5 py-1 text-xs rounded-lg ' + (c.enabled ? 'text-emerald-400 bg-emerald-400/10' : 'text-gray-400 bg-gray-400/10')}
                      data-testid={'feishu-toggle-' + c.id}
                    >
                      {c.enabled ? '已启用' : '已停用'}
                    </button>
                    <button
                      onClick={() => deleteConfig(c.id)}
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

      {showModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center" data-testid="feishu-create-modal">
          <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={() => setShowModal(false)} />
          <div className="relative glass p-6 rounded-2xl max-w-lg w-full mx-4">
            <h3 className="text-lg font-semibold text-[var(--text-primary)] mb-4">新增飞书配置</h3>
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
                <label className="block text-xs text-[var(--text-secondary)] mb-1">模型（可选，留空使用默认）</label>
                <input type="text" value={newCfg.model_id} onChange={e => setNewCfg(p => ({ ...p, model_id: e.target.value }))}
                  className="w-full px-3 py-2 text-sm rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)]"
                  data-testid="feishu-model-input" placeholder="留空即使用默认模型" />
              </div>
              <div className="flex gap-3 pt-2">
                <button onClick={() => setShowModal(false)}
                  className="flex-1 px-4 py-2 text-sm rounded-xl border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">取消</button>
                <button onClick={createConfig}
                  className="flex-1 px-4 py-2 text-sm rounded-xl bg-[var(--accent)] text-white hover:opacity-90 disabled:opacity-40"
                  data-testid="feishu-create-btn" disabled={!newCfg.name.trim() || !newCfg.app_id.trim() || !newCfg.app_secret.trim()}>创建配置</button>
              </div>
            </div>
          </div>
        </div>
      )}
    </AppLayout>
  );
}
