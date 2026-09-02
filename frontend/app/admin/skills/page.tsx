'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '@/lib/api';
import Pagination from '../../components/Pagination';
import { modalOverlayStyle } from '../../components/ui';

interface SkillItem {
  name: string;
  display_name: string;
  description: string;
  enabled: boolean;
  config_json: string;
}

export default function SkillsAdminPage() {
  const { apiFetch, auth } = useAuth();
  const [skills, setSkills] = useState<SkillItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const PAGE = 10;
  const [loading, setLoading] = useState(true);
  const [editingName, setEditingName] = useState<string | null>(null);
  const [editEnabled, setEditEnabled] = useState(false);
  const [editConfig, setEditConfig] = useState('');
  const [editError, setEditError] = useState('');
  const [saving, setSaving] = useState(false);
  const [toast, setToast] = useState<{ msg: string; type: 'success' | 'error' } | null>(null);

  const showToast = (msg: string, type: 'success' | 'error') => {
    setToast({ msg, type });
    setTimeout(() => setToast(null), 2500);
  };

  const fetchSkills = useCallback(async () => {
    try {
      const res = await apiFetch(`/admin/skills?page=${page}&page_size=${PAGE}`);
      if (res.ok) {
        const data = await res.json();
        setSkills(data.skills || []);
        setTotal(data.total || 0);
      }
    } catch (e) {
      console.error('fetchSkills:', e);
    }
    setLoading(false);
  }, [apiFetch, page]);

  useEffect(() => {
    if (!auth.hydrated || !auth.token) return;
    fetchSkills();
  }, [fetchSkills, auth.hydrated, auth.token]);

  const openEdit = (s: SkillItem) => {
    setEditingName(s.name);
    setEditEnabled(s.enabled);
    setEditConfig(s.config_json || '{}');
    setEditError('');
  };

  const closeEdit = () => {
    setEditingName(null);
    setEditConfig('');
    setEditError('');
  };

  const saveConfig = async () => {
    if (!editingName) return;
    // Validate JSON
    try {
      JSON.parse(editConfig);
    } catch {
      setEditError('JSON 格式错误');
      return;
    }
    setEditError('');
    setSaving(true);
    try {
      const res = await apiFetch(`/admin/skills/${editingName}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: editEnabled, config_json: editConfig }),
      });
      if (res.ok) {
        showToast('已保存', 'success');
        closeEdit();
        fetchSkills();
      } else {
        const d = await res.json().catch(() => ({}));
        setEditError(d.error || '保存失败');
      }
    } catch {
      setEditError('保存失败');
    }
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

  const Field = ({ label, children }: { label: string; children: React.ReactNode }) => (
    <div style={{ marginBottom: '16px' }}>
      <label style={{ display: 'block', fontSize: '12px', color: 'var(--text-secondary)', marginBottom: '4px' }}>{label}</label>
      {children}
    </div>
  );

  if (loading) {
    return (
      <AppLayout>
        <div style={{ padding: '24px', color: 'var(--text-secondary)' }}>加载中…</div>
      </AppLayout>
    );
  }

  return (
    <AppLayout>
      <div style={{ padding: '0 0 24px 0' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
          <h2 style={{ fontSize: '18px', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>Skill 管理</h2>
        </div>

        <p style={{ fontSize: '13px', color: 'var(--text-secondary)', marginBottom: '16px' }}>
          管理 Agent 可用的技能工具。点击技能名称可编辑其配置（JSON 格式）。
        </p>

        <div className="glass" style={{ padding: 0 }}>
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', fontSize: '13px', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid rgba(255,255,255,0.1)' }}>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500, width: '160px' }}>名称</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>显示名</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>描述</th>
                  <th style={{ textAlign: 'center', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500, width: '80px' }}>启用</th>
                  <th style={{ textAlign: 'right', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500, width: '120px' }}>操作</th>
                </tr>
              </thead>
              <tbody>
                {skills.map((s) => (
                  <tr key={s.name} style={{ borderBottom: '1px solid rgba(255,255,255,0.05)' }}
                    data-testid={`skill-row-${s.name}`}>
                    <td style={{ padding: '10px 12px' }}>
                      <code style={{ color: 'var(--text-primary)', fontSize: '12px' }}>{s.name}</code>
                    </td>
                    <td style={{ padding: '10px 12px', color: 'var(--text-primary)', fontWeight: 500 }}>{s.display_name}</td>
                    <td style={{ padding: '10px 12px', color: 'var(--text-secondary)', fontSize: '12px', maxWidth: '300px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{s.description}</td>
                    <td style={{ padding: '10px 12px', textAlign: 'center' }}>
                      <span style={{
                        display: 'inline-block',
                        width: '36px',
                        height: '20px',
                        borderRadius: '10px',
                        background: s.enabled ? 'var(--accent)' : 'rgba(255,255,255,0.15)',
                        position: 'relative',
                        verticalAlign: 'middle',
                      }}>
                        <span style={{
                          position: 'absolute',
                          top: '2px',
                          left: s.enabled ? '18px' : '2px',
                          width: '16px',
                          height: '16px',
                          borderRadius: '50%',
                          background: '#fff',
                          transition: 'left 0.2s',
                        }} />
                      </span>
                    </td>
                    <td style={{ padding: '10px 12px', textAlign: 'right' }}>
                      <button
                        data-testid={`skill-edit-${s.name}`}
                        onClick={() => openEdit(s)}
                        style={{
                          background: 'transparent',
                          border: '1px solid rgba(255,255,255,0.1)',
                          borderRadius: '4px',
                          padding: '4px 12px',
                          color: 'var(--accent)',
                          cursor: 'pointer',
                          fontSize: '12px',
                        }}
                      >配置</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        {total > 0 && (
          <Pagination page={page} total={total} pageSize={PAGE} onChange={setPage} />
        )}

        {/* Edit Modal */}
        {editingName && (
          <div style={{ ...modalOverlayStyle, zIndex: 999 }} onClick={closeEdit}>
            <div style={{
              background: 'var(--bg-secondary)',
              border: '1px solid var(--border-glass)',
              borderRadius: '12px',
              padding: '24px',
              width: '560px',
              maxHeight: '80vh',
              overflow: 'auto',
              boxShadow: '0 8px 32px rgba(0,0,0,0.4)',
            }} onClick={e => e.stopPropagation()}>
              <h3 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)', margin: '0 0 16px 0' }}>
                配置 <code style={{ color: 'var(--accent)' }}>{editingName}</code>
              </h3>

              <Field label="启用状态">
                <label style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer' }}>
                  <input type="checkbox" checked={editEnabled} onChange={e => setEditEnabled(e.target.checked)}
                    style={{ accentColor: 'var(--accent)' }} />
                  <span style={{ fontSize: '13px', color: 'var(--text-primary)' }}>LLM 可调用此工具</span>
                </label>
              </Field>

              <Field label="配置 JSON">
                <textarea
                  data-testid="skill-edit-config"
                  value={editConfig}
                  onChange={e => { setEditConfig(e.target.value); setEditError(''); }}
                  style={{ ...inputStyle, minHeight: '200px', resize: 'vertical' }}
                  placeholder='{"dsn":"user:pass@tcp(host:3306)/db"}'
                />
                {editError && (
                  <p style={{ color: '#ef4444', fontSize: '12px', marginTop: '4px' }}>{editError}</p>
                )}
              </Field>

              <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end', marginTop: '8px' }}>
                <button onClick={closeEdit}
                  style={{ background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '6px', padding: '6px 16px', color: 'var(--text-secondary)', cursor: 'pointer', fontSize: '13px' }}>
                  取消</button>
                <button
                  data-testid="skill-save-btn"
                  onClick={saveConfig}
                  disabled={saving}
                  style={{ background: 'var(--accent)', border: 'none', borderRadius: '6px', padding: '6px 16px', color: '#fff', cursor: saving ? 'not-allowed' : 'pointer', fontSize: '13px', opacity: saving ? 0.6 : 1 }}>
                  {saving ? '保存中…' : '保存'}</button>
              </div>
            </div>
          </div>
        )}

        {/* Toast */}
        {toast && (
          <div style={{
            position: 'fixed', top: '20px', right: '20px', zIndex: 9999,
            padding: '8px 16px', borderRadius: '6px',
            background: toast.type === 'success' ? 'rgba(34,197,94,0.9)' : 'rgba(239,68,68,0.9)',
            color: '#fff', fontSize: '13px',
          }}>{toast.msg}</div>
        )}
      </div>
    </AppLayout>
  );
}
