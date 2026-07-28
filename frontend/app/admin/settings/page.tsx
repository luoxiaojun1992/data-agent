'use client';

import React, { useState, useEffect } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '@/lib/api';

interface ConfigItem {
  key: string;
  value: string;
}

const NAMESPACES = ['system'];

export default function SettingsPage() {
  const { apiFetch } = useAuth();

  const [configs, setConfigs] = useState<Record<string, ConfigItem[]>>({});
  const [loading, setLoading] = useState(true);
  const [editingKey, setEditingKey] = useState<string | null>(null);
  const [editValue, setEditValue] = useState('');
  const [editNs, setEditNs] = useState('');
  const [saving, setSaving] = useState(false);
  const [toast, setToast] = useState<{ msg: string; type: 'success' | 'error' } | null>(null);

  const showToast = (msg: string, type: 'success' | 'error') => {
    setToast({ msg, type });
    setTimeout(() => setToast(null), 2500);
  };

  const fetchAll = async () => {
    setLoading(true);
    const all: Record<string, ConfigItem[]> = {};
    for (const ns of NAMESPACES) {
      try {
        const res = await apiFetch(`/sysconfig/${ns}`);
        if (res.ok) {
          const data = await res.json();
          all[ns] = data.configs || [];
        }
      } catch { all[ns] = []; }
    }
    setConfigs(all);
    setLoading(false);
  };

  useEffect(() => {
    fetchAll();
  }, []);

  const openEdit = (ns: string, key: string, value: string) => {
    setEditingKey(`${ns}:${key}`);
    setEditNs(ns);
    setEditValue(value);
  };

  const saveEdit = async () => {
    if (!editingKey) return;
    setSaving(true);
    try {
      const res = await apiFetch(`/sysconfig/${editNs}`, {
        method: 'PUT',
        body: JSON.stringify({ key: editingKey.split(':')[1], value: editValue }),
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
    width: '100%', background: 'rgba(255,255,255,0.05)',
    border: '1px solid rgba(255,255,255,0.1)', borderRadius: '6px',
    padding: '8px 12px', color: 'var(--text-primary)', fontSize: '13px', outline: 'none',
    fontFamily: 'monospace',
  };

  if (loading) {
    return (
      <AppLayout>
        <div style={{ padding: '24px', color: 'var(--text-secondary)' }}>加载中…</div>
      </AppLayout>
    );
  }

  return (
    <AppLayout>
      <div>
        <h2 style={{ fontSize: '18px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '8px' }}>系统设置</h2>
        <p style={{ fontSize: '13px', color: 'var(--text-secondary)', marginBottom: '20px' }}>管理系统全局配置参数</p>

        {NAMESPACES.map(ns => (
          <div key={ns} className="glass" style={{ padding: '20px', marginBottom: '16px' }}>
            <h3 style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '12px', textTransform: 'capitalize' }}>{ns}</h3>
            {configs[ns]?.length === 0 ? (
              <p style={{ fontSize: '13px', color: 'var(--text-secondary)' }}>暂无配置</p>
            ) : (
              <div style={{ overflowX: 'auto' }}>
                <table style={{ width: '100%', fontSize: '13px', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ borderBottom: '1px solid rgba(255,255,255,0.1)' }}>
                      <th style={{ textAlign: 'left', padding: '8px 12px', color: 'var(--text-secondary)', fontWeight: 500, width: '200px' }}>Key</th>
                      <th style={{ textAlign: 'left', padding: '8px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>Value</th>
                      <th style={{ textAlign: 'right', padding: '8px 12px', color: 'var(--text-secondary)', fontWeight: 500, width: '80px' }}>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {configs[ns]?.map((c, i) => (
                      <tr key={c.key} style={{ borderBottom: '1px solid rgba(255,255,255,0.05)' }}>
                        <td style={{ padding: '8px 12px' }}>
                          <code style={{ fontSize: '12px', color: 'var(--accent)' }}>{c.key}</code>
                        </td>
                        <td style={{ padding: '8px 12px', color: 'var(--text-primary)', maxWidth: '400px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {editingKey === `${ns}:${c.key}` ? (
                            <input
                              value={editValue}
                              onChange={e => setEditValue(e.target.value)}
                              style={{ ...inputStyle, width: '300px' }}
                              autoFocus
                            />
                          ) : (
                            c.value || <span style={{ color: 'var(--text-secondary)', fontStyle: 'italic' }}>空</span>
                          )}
                        </td>
                        <td style={{ padding: '8px 12px', textAlign: 'right' }}>
                          {editingKey === `${ns}:${c.key}` ? (
                            <div style={{ display: 'flex', gap: '4px', justifyContent: 'flex-end' }}>
                              <button onClick={saveEdit} disabled={saving}
                                style={{ padding: '4px 10px', background: 'var(--accent)', border: 'none', borderRadius: '4px', color: '#fff', fontSize: '12px', cursor: 'pointer' }}>
                                {saving ? '…' : '保存'}
                              </button>
                              <button onClick={() => setEditingKey(null)}
                                style={{ padding: '4px 10px', background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '4px', color: '#7A7A7A', fontSize: '12px', cursor: 'pointer' }}>
                                取消
                              </button>
                            </div>
                          ) : (
                            <button onClick={() => openEdit(ns, c.key, c.value)}
                              style={{ padding: '4px 10px', background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '4px', color: 'var(--accent)', fontSize: '12px', cursor: 'pointer' }}>
                              编辑
                            </button>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        ))}
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
