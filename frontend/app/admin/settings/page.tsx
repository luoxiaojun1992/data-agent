'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '@/lib/api';

interface ConfigItem {
  key: string;
  value: string;
  source: 'default' | 'env' | 'stored';
  description?: string;
}

const BUILTIN_CONFIGS: Omit<ConfigItem, 'value' | 'source'>[] = [
  { key: 'MONGO_URI', description: 'MongoDB 连接 URI' },
  { key: 'REDIS_ADDR', description: 'Redis 地址 (host:port)' },
  { key: 'QDRANT_URL', description: 'Qdrant HTTP URL' },
  { key: 'EMBEDDING_BASE_URL', description: 'Embedding 模型 API 地址' },
  { key: 'EMBEDDING_MODEL', description: 'Embedding 模型名' },
  { key: 'EMBEDDING_VECTOR_DIM', description: 'Embedding 向量维度' },
  { key: 'INVITE_HMAC_SECRET', description: '邀请 HMAC 签名密钥' },
  { key: 'VAULT_ADDR', description: 'HashiCorp Vault 地址' },
  { key: 'JWT_SECRET', description: 'JWT 签名密钥' },
  { key: 'SERVER_READ_TIMEOUT', description: 'HTTP 读超时（秒）' },
  { key: 'SERVER_WRITE_TIMEOUT', description: 'HTTP 写超时（秒）' },
];

const PAGE_SIZE = 8;

export default function SettingsPage() {
  const { auth, apiFetch } = useAuth();

  const [items, setItems] = useState<ConfigItem[]>([]);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [editingKey, setEditingKey] = useState<string | null>(null);
  const [editValue, setEditValue] = useState('');
  const [saving, setSaving] = useState(false);
  const [toast, setToast] = useState<{ msg: string; type: 'success' | 'error' } | null>(null);

  const showToast = (msg: string, type: 'success' | 'error') => {
    setToast({ msg, type });
    setTimeout(() => setToast(null), 2500);
  };

  const fetchAll = useCallback(async () => {
    if (!auth.token) {
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      const res = await apiFetch('/sysconfig/system');
      if (res.ok) {
        const data = await res.json();
        const stored: ConfigItem[] = (data.configs || []).map((c: any) => ({
          key: c.key, value: c.value, source: 'stored' as const,
        }));
        const storedMap = new Map(stored.map(c => [c.key, c.value]));
        const merged: ConfigItem[] = BUILTIN_CONFIGS.map(b => ({
          key: b.key,
          description: b.description,
          value: storedMap.get(b.key) || '(使用默认值)',
          source: storedMap.has(b.key) ? 'stored' : 'default',
        }));
        for (const c of stored) {
          if (!BUILTIN_CONFIGS.find(b => b.key === c.key)) {
            merged.push({ key: c.key, value: c.value, source: 'stored' });
          }
        }
        setItems(merged);
      } else {
        setItems([]);
      }
    } catch {
      setItems([]);
    }
    setLoading(false);
  }, [apiFetch, auth.token]);

  useEffect(() => {
    if (auth.hydrated) fetchAll();
  }, [auth.hydrated, fetchAll]);

  const openEdit = (key: string, value: string) => {
    setEditingKey(key);
    setEditValue(value === '(使用默认值)' ? '' : value);
  };

  const saveEdit = async () => {
    if (!editingKey) return;
    setSaving(true);
    try {
      const res = await apiFetch('/sysconfig/system', {
        method: 'PUT',
        body: JSON.stringify({ key: editingKey, value: editValue }),
      });
      if (res.ok) {
        showToast('已保存', 'success');
        fetchAll();
      } else {
        const d = await res.json().catch(() => ({}));
        showToast(d.error || '保存失败', 'error');
      }
    } catch {
      showToast('保存失败', 'error');
    }
    setEditingKey(null);
    setSaving(false);
  };

  const inputStyle: React.CSSProperties = {
    width: '100%',
    background: 'rgba(255,255,255,0.05)',
    border: '1px solid rgba(255,255,255,0.1)',
    borderRadius: '6px',
    padding: '8px 12px',
    color: 'var(--text-primary)',
    fontSize: '13px',
    outline: 'none',
    fontFamily: 'monospace',
  };

  if (loading) {
    return (
      <AppLayout>
        <div style={{ padding: '24px', color: 'var(--text-secondary)' }}>加载中…</div>
      </AppLayout>
    );
  }

  const totalPages = Math.max(1, Math.ceil(items.length / PAGE_SIZE));
  const paged = items.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);

  return (
    <AppLayout>
      <div>
        <h2 style={{ fontSize: '18px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '8px' }}>系统设置</h2>
        <p style={{ fontSize: '13px', color: 'var(--text-secondary)', marginBottom: '20px' }}>
          管理全局配置参数。已保存的值覆盖默认值，默认值不能删除。
        </p>

        <div className="glass" style={{ padding: '0' }}>
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', fontSize: '13px', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid rgba(255,255,255,0.1)' }}>
                  <th style={{ textAlign: 'left', padding: '12px', color: 'var(--text-secondary)', fontWeight: 500, width: '220px' }}>Key</th>
                  <th style={{ textAlign: 'left', padding: '12px', color: 'var(--text-secondary)', fontWeight: 500 }}>Value</th>
                  <th style={{ textAlign: 'center', padding: '12px', color: 'var(--text-secondary)', fontWeight: 500, width: '90px' }}>来源</th>
                  <th style={{ textAlign: 'right', padding: '12px', color: 'var(--text-secondary)', fontWeight: 500, width: '80px' }}>操作</th>
                </tr>
              </thead>
              <tbody>
                {paged.map((c) => {
                  const isEditing = editingKey === c.key;
                  return (
                    <tr key={c.key} style={{ borderBottom: '1px solid rgba(255,255,255,0.05)' }} data-testid={`settings-row-${c.key}`}>
                      <td style={{ padding: '12px' }}>
                        <code style={{ fontSize: '12px', color: 'var(--accent)' }}>{c.key}</code>
                        {c.description && (
                          <div style={{ fontSize: '11px', color: 'var(--text-secondary)', marginTop: '2px' }}>{c.description}</div>
                        )}
                      </td>
                      <td style={{ padding: '12px', color: 'var(--text-primary)', maxWidth: '500px' }}>
                        {isEditing ? (
                          <input
                            value={editValue}
                            onChange={e => setEditValue(e.target.value)}
                            style={inputStyle}
                            autoFocus
                            placeholder={c.value === '(使用默认值)' ? '输入新值...' : ''}
                          />
                        ) : (
                          <span style={{ fontFamily: 'monospace', fontSize: '12px', color: c.source === 'stored' ? 'var(--text-primary)' : 'var(--text-secondary)' }}>
                            {c.value}
                          </span>
                        )}
                      </td>
                      <td style={{ padding: '12px', textAlign: 'center' }}>
                        <span style={{
                          display: 'inline-block', padding: '2px 8px', borderRadius: '10px', fontSize: '11px',
                          background: c.source === 'stored' ? 'rgba(92,124,250,0.15)' : 'rgba(255,255,255,0.06)',
                          color: c.source === 'stored' ? '#5c7cfa' : '#7A7A7A',
                        }}>
                          {c.source === 'stored' ? '已保存' : '默认'}
                        </span>
                      </td>
                      <td style={{ padding: '12px', textAlign: 'right' }}>
                        {isEditing ? (
                          <div style={{ display: 'flex', gap: '4px', justifyContent: 'flex-end' }}>
                            <button onClick={saveEdit} disabled={saving}
                              data-testid={`settings-save-${c.key}`}
                              style={{ padding: '4px 10px', background: 'var(--accent)', border: 'none', borderRadius: '4px', color: '#fff', fontSize: '12px', cursor: 'pointer' }}>
                              {saving ? '…' : '保存'}
                            </button>
                            <button onClick={() => setEditingKey(null)}
                              style={{ padding: '4px 10px', background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '4px', color: '#7A7A7A', fontSize: '12px', cursor: 'pointer' }}>
                              取消
                            </button>
                          </div>
                        ) : (
                          <button onClick={() => openEdit(c.key, c.value)}
                            data-testid={`settings-edit-${c.key}`}
                            style={{ padding: '4px 10px', background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '4px', color: 'var(--accent)', fontSize: '12px', cursor: 'pointer' }}>
                            编辑
                          </button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>

        {items.length > 0 && (
          <div data-testid="settings-pagination" style={{ display: 'flex', justifyContent: 'center', gap: '8px', marginTop: '16px' }}>
            <button onClick={() => setPage(p => Math.max(1, p - 1))} disabled={page === 1}
              style={{ padding: '6px 14px', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '8px', background: 'transparent', color: '#7A7A7A', fontSize: '13px', cursor: 'pointer' }}>上一页</button>
            <span style={{ padding: '8px 12px', fontSize: '13px', color: '#7A7A7A' }}>
              {page} / {totalPages}（共 {items.length} 条）
            </span>
            <button onClick={() => setPage(p => Math.min(totalPages, p + 1))} disabled={page >= totalPages}
              style={{ padding: '6px 14px', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '8px', background: 'transparent', color: '#7A7A7A', fontSize: '13px', cursor: 'pointer' }}>下一页</button>
          </div>
        )}
      </div>

      {toast && (
        <div style={{ position: 'fixed', top: '20px', right: '20px', zIndex: 9999,
          padding: '8px 16px', borderRadius: '6px',
          background: toast.type === 'success' ? 'rgba(34,197,94,0.9)' : 'rgba(239,68,68,0.9)',
          color: '#fff', fontSize: '13px',
        }}>{toast.msg}</div>
      )}
    </AppLayout>
  );
}