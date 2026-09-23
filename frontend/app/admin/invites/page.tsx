'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useTranslations } from 'next-intl';
import AppLayout from '../../providers';
import { useAuth } from '@/lib/api';
import Pagination from '../../components/Pagination';
import { primaryButtonStyle } from '../../components/ui';

interface InviteItem {
  invite_id: string;
  email: string;
  role: string;
  status: string;
  created_by: string;
  created_at: string;
  expires_at: string;
  accepted_at?: string;
  accepted_by?: string;
}

export default function InvitesPage() {
  return (
    <AppLayout>
      <InvitesContent />
    </AppLayout>
  );
}

function InvitesContent() {
  const t = useTranslations('adminInvites');
  const { auth, apiFetch } = useAuth();

  const [invites, setInvites] = useState<InviteItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  // Create invite form
  const [email, setEmail] = useState('');
  const [role, setRole] = useState('user');
  const [expireHours, setExpireHours] = useState(24);
  const [showForm, setShowForm] = useState(false);
  const [generatedURL, setGeneratedURL] = useState('');
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState('');

  const loadInvites = useCallback(async () => {
    if (!auth.token) return;
    setLoading(true);
    try {
      const res = await apiFetch(`/admin/invites?page=${page}&size=20`);
      if (res.ok) {
        const data = await res.json();
        setInvites(data.invites || []);
        setTotal(data.total || 0);
        setError('');
      } else {
        setInvites([]);
        setTotal(0);
        setError('');
      }
    } catch (e: any) {
      setInvites([]);
      setTotal(0);
      setError('');
    } finally {
      setLoading(false);
    }
  }, [apiFetch, page]);

  useEffect(() => {
    if (auth.hydrated) loadInvites();
  }, [auth.hydrated, loadInvites]);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setCreating(true);
    setCreateError('');
    try {
      const res = await apiFetch('/admin/invites', {
        method: 'POST',
        body: JSON.stringify({ email: email.trim() || undefined, role, expire_hours: expireHours }),
      });
      const data = await res.json();
      if (!res.ok) {
        setCreateError(data.error || t('createFailed', { status: res.status }));
        return;
      }
      setGeneratedURL(data.invite_url);
      setShowForm(false);
      setEmail('');
      loadInvites();
    } catch (e: any) {
      setCreateError(t('requestFailed', { msg: e?.message || e }));
    } finally {
      setCreating(false);
    }
  };

  const handleRevoke = async (inviteID: string) => {
    try {
      await apiFetch(`/admin/invites/${inviteID}`, { method: 'DELETE' });
      loadInvites();
    } catch (e: any) {
      console.error(e);
    }
  };

  const handleCopyURL = () => {
    navigator.clipboard.writeText(generatedURL);
  };

  const statusColor = (status: string) => {
    switch (status) {
      case 'pending': return '#B1E2FF';
      case 'accepted': return '#4ADE80';
      case 'expired': return '#7A7A7A';
      case 'revoked': return '#F87171';
      default: return '#7A7A7A';
    }
  };

  const statusLabel = (status: string) => {
    switch (status) {
      case 'pending': return t('statusPending');
      case 'accepted': return t('statusAccepted');
      case 'expired': return t('statusExpired');
      case 'revoked': return t('statusRevoked');
      default: return status;
    }
  };

  return (
    <div className="animate-fade-in">
      <div className="mb-8 flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold text-[var(--text-primary)]" data-testid="invites-page-header">{t('title')}</h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">{t('desc')}</p>
        </div>
        <button
          onClick={() => { setShowForm(!showForm); setGeneratedURL(''); setCreateError(''); }}
          style={primaryButtonStyle}
          data-testid="invites-create-btn"
        >
          {t('generateInvite')}
        </button>
      </div>

      {/* Generated URL display */}
      {generatedURL && (
        <div className="mb-6 p-4 rounded-xl bg-[#B1E2FF]/10 border border-[#B1E2FF]/20" data-testid="invites-url-display">
          <p className="text-sm text-[var(--text-secondary)] mb-2">{t('urlGenerated')}</p>
          <div className="flex items-center gap-2">
            <code className="flex-1 px-3 py-2 rounded-lg bg-black/20 text-[#B1E2FF] text-sm break-all" data-testid="invites-url-text">{generatedURL}</code>
            <button
              onClick={handleCopyURL}
              className="px-3 py-2 rounded-lg text-sm font-medium bg-[#B1E2FF]/20 text-[#B1E2FF] hover:bg-[#B1E2FF]/30 transition-all"
              data-testid="invites-copy-btn"
            >
              {t('copy')}
            </button>
          </div>
        </div>
      )}

      {/* Create form */}
      {showForm && (
        <div className="mb-6 glass p-6" style={{ background: 'var(--surface-3)', border: '1px solid var(--surface-8)', borderRadius: '12px' }} data-testid="invites-create-form">
          <form onSubmit={handleCreate} className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div data-testid="invites-email-field">
                <label className="block mb-1 text-xs text-[var(--text-secondary)]">{t('emailLabel')}</label>
                <input
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="newuser@company.com"
                  className="w-full px-3 py-2 rounded-lg bg-[var(--surface-5)] border border-[var(--surface-10)] text-[var(--text-primary)] text-sm placeholder-[var(--surface-30)] focus:outline-none focus:border-[#B1E2FF]"
                  data-testid="invites-email-input"
                />
              </div>
              <div data-testid="invites-role-field">
                <label className="block mb-1 text-xs text-[var(--text-secondary)]">{t('roleLabel')}</label>
                <select
                  value={role}
                  onChange={(e) => setRole(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg bg-[var(--surface-5)] border border-[var(--surface-10)] text-[var(--text-primary)] text-sm focus:outline-none focus:border-[#B1E2FF]"
                  data-testid="invites-role-select"
                >
                  <option value="user">{t('roleUserOption')}</option>
                  {auth.role === 'system_admin' && <option value="admin">{t('roleAdminOption')}</option>}
                </select>
              </div>
              <div data-testid="invites-expire-field">
                <label className="block mb-1 text-xs text-[var(--text-secondary)]">{t('expireLabel')}</label>
                <select
                  value={expireHours}
                  onChange={(e) => setExpireHours(Number(e.target.value))}
                  className="w-full px-3 py-2 rounded-lg bg-[var(--surface-5)] border border-[var(--surface-10)] text-[var(--text-primary)] text-sm focus:outline-none focus:border-[#B1E2FF]"
                  data-testid="invites-expire-select"
                >
                  <option value={24}>{t('hours24')}</option>
                  <option value={48}>{t('hours48')}</option>
                  <option value={168}>{t('days7')}</option>
                  <option value={720}>{t('days30')}</option>
                </select>
              </div>
            </div>
            {createError && <p className="text-sm text-red-400" data-testid="invites-create-error">{createError}</p>}
            <div className="flex gap-2">
              <button
                type="submit"
                disabled={creating}
                className="px-4 py-2 rounded-lg font-medium text-sm transition-all"
                style={{ background: 'linear-gradient(135deg, #B1E2FF, #9381FF)', color: '#000' }}
                data-testid="invites-submit-btn"
              >
                {creating ? t('generating') : t('confirmGenerate')}
              </button>
              <button
                type="button"
                onClick={() => setShowForm(false)}
                className="px-4 py-2 rounded-lg text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                data-testid="invites-cancel-btn"
              >
                {t('cancel')}
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Invite list */}
      {error && <p className="text-red-400 mb-4 text-sm">{error}</p>}

      {loading ? (
        <p className="text-[var(--text-secondary)] text-sm">{t('loading')}</p>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="w-full text-sm" data-testid="invites-table">
              <thead>
                <tr className="border-b border-[var(--surface-10)] text-left">
                  <th className="py-3 px-4 text-[var(--text-secondary)] font-medium">{t('colEmail')}</th>
                  <th className="py-3 px-4 text-[var(--text-secondary)] font-medium">{t('colRole')}</th>
                  <th className="py-3 px-4 text-[var(--text-secondary)] font-medium">{t('colStatus')}</th>
                  <th className="py-3 px-4 text-[var(--text-secondary)] font-medium">{t('colCreatedAt')}</th>
                  <th className="py-3 px-4 text-[var(--text-secondary)] font-medium">{t('colExpiresAt')}</th>
                  <th className="py-3 px-4 text-[var(--text-secondary)] font-medium">{t('colActions')}</th>
                </tr>
              </thead>
              <tbody>
                {invites.map((inv) => (
                  <tr key={inv.invite_id} className="border-b border-[var(--surface-5)] hover:bg-[var(--surface-5)]" data-testid="invites-row">
                    <td className="py-3 px-4 text-[var(--text-primary)]">{inv.email || '—'}</td>
                    <td className="py-3 px-4 text-[var(--text-secondary)]">{inv.role}</td>
                    <td className="py-3 px-4">
                      <span style={{ color: statusColor(inv.status) }}>{statusLabel(inv.status)}</span>
                    </td>
                    <td className="py-3 px-4 text-[var(--text-secondary)]">{new Date(inv.created_at).toLocaleDateString('zh-CN')}</td>
                    <td className="py-3 px-4 text-[var(--text-secondary)]">{new Date(inv.expires_at).toLocaleDateString('zh-CN')}</td>
                    <td className="py-3 px-4">
                      {inv.status === 'pending' && (
                        <button
                          onClick={() => handleRevoke(inv.invite_id)}
                          className="text-red-400 hover:text-red-300 text-xs"
                          data-testid="invites-revoke-btn"
                        >
                          {t('revoke')}
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
                {invites.length === 0 && (
                  <tr>
                    <td colSpan={6} className="py-8 text-center text-[var(--text-secondary)]">{t('empty')}</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          {total > 0 && (
            <Pagination page={page} total={total} pageSize={20} onChange={setPage} />
          )}
        </>
      )}
    </div>
  );
}
