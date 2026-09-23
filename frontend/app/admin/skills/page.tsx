'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useTranslations } from 'next-intl';
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
  requires_approval: boolean;
}

export default function SkillsAdminPage() {
  const t = useTranslations('adminSkills');
  const { apiFetch, auth } = useAuth();
  const [skills, setSkills] = useState<SkillItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const PAGE = 10;
  const [loading, setLoading] = useState(true);
  const [editingName, setEditingName] = useState<string | null>(null);
  const [editEnabled, setEditEnabled] = useState(false);
  const [editApproval, setEditApproval] = useState(false);
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
    setEditApproval(s.requires_approval);
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
      setEditError(t('jsonError'));
      return;
    }
    setEditError('');
    setSaving(true);
    try {
      const res = await apiFetch(`/admin/skills/${editingName}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: editEnabled, config_json: editConfig, requires_approval: editApproval }),
      });
      if (res.ok) {
        showToast(t('saved'), 'success');
        closeEdit();
        fetchSkills();
      } else {
        const d = await res.json().catch(() => ({}));
        setEditError(d.error || t('saveFailed'));
      }
    } catch {
      setEditError(t('saveFailed'));
    }
    setSaving(false);
  };

  const inputStyle: React.CSSProperties = {
    width: '100%',
    background: 'var(--surface-5)',
    border: '1px solid var(--surface-10)',
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
        <div style={{ padding: '24px', color: 'var(--text-secondary)' }}>{t('loading')}</div>
      </AppLayout>
    );
  }

  return (
    <AppLayout>
      <div style={{ padding: '0 0 24px 0' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
          <h2 style={{ fontSize: '18px', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>{t('title')}</h2>
        </div>

        <p style={{ fontSize: '13px', color: 'var(--text-secondary)', marginBottom: '16px' }}>
          {t('desc')}
        </p>

        <div className="glass" style={{ padding: 0 }}>
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', fontSize: '13px', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--surface-10)' }}>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500, width: '160px' }}>{t('colName')}</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>{t('colDisplayName')}</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>{t('colDescription')}</th>
                  <th style={{ textAlign: 'center', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500, width: '80px' }}>{t('colEnabled')}</th>
                  <th style={{ textAlign: 'right', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500, width: '120px' }}>{t('colActions')}</th>
                </tr>
              </thead>
              <tbody>
                {skills.map((s) => (
                  <tr key={s.name} style={{ borderBottom: '1px solid var(--surface-5)' }}
                    data-testid={`skill-row-${s.name}`}>
                    <td style={{ padding: '10px 12px' }}>
                      <code style={{ color: 'var(--text-primary)', fontSize: '12px' }}>{s.name}</code>
                      {s.requires_approval && (
                        <span title={t('requiresApproval')} style={{ marginLeft: '6px', fontSize: '12px' }}>🔒</span>
                      )}
                    </td>
                    <td style={{ padding: '10px 12px', color: 'var(--text-primary)', fontWeight: 500 }}>{s.display_name}</td>
                    <td style={{ padding: '10px 12px', color: 'var(--text-secondary)', fontSize: '12px', maxWidth: '300px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{s.description}</td>
                    <td style={{ padding: '10px 12px', textAlign: 'center' }}>
                      <span style={{
                        display: 'inline-block',
                        width: '36px',
                        height: '20px',
                        borderRadius: '10px',
                        background: s.enabled ? 'var(--accent)' : 'var(--surface-15)',
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
                          border: '1px solid var(--surface-10)',
                          borderRadius: '4px',
                          padding: '4px 12px',
                          color: 'var(--accent)',
                          cursor: 'pointer',
                          fontSize: '12px',
                        }}
                      >{t('configure')}</button>
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
              borderRadius: '16px',
              padding: '24px',
              width: '560px',
              maxHeight: '80vh',
              overflow: 'auto',
              boxShadow: '0 8px 32px rgba(0,0,0,0.5)',
            }} onClick={e => e.stopPropagation()}>
              <h3 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)', margin: '0 0 16px 0' }}>
                {t('configure')} <code style={{ color: 'var(--accent)' }}>{editingName}</code>
              </h3>

              <Field label={t('enabledStatus')}>
                <label style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer' }}>
                  <input type="checkbox" checked={editEnabled} onChange={e => setEditEnabled(e.target.checked)}
                    style={{ accentColor: 'var(--accent)' }} />
                  <span style={{ fontSize: '13px', color: 'var(--text-primary)' }}>{t('llmCanCall')}</span>
                </label>
              </Field>

              <Field label={t('approvalField')}>
                <label style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: 'pointer' }}>
                  <input type="checkbox" data-testid="skill-approval-toggle" checked={editApproval}
                    onChange={e => setEditApproval(e.target.checked)}
                    style={{ accentColor: 'var(--accent)' }} />
                  <span style={{ fontSize: '13px', color: 'var(--text-primary)' }}>{t('requiresApproval')}</span>
                </label>
              </Field>

              <Field label={t('configJson')}>
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
                  style={{ background: 'transparent', border: '1px solid var(--surface-10)', borderRadius: '6px', padding: '6px 16px', color: 'var(--text-secondary)', cursor: 'pointer', fontSize: '13px' }}>
                  {t('cancel')}</button>
                <button
                  data-testid="skill-save-btn"
                  onClick={saveConfig}
                  disabled={saving}
                  style={{ background: 'var(--accent)', border: 'none', borderRadius: '6px', padding: '6px 16px', color: '#fff', cursor: saving ? 'not-allowed' : 'pointer', fontSize: '13px', opacity: saving ? 0.6 : 1 }}>
                  {saving ? t('saving') : t('save')}</button>
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
