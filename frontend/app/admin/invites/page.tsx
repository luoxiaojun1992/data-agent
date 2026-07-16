'use client';

import React, { useState, useEffect } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '@/lib/api';

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
  const { apiFetch } = useAuth();

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

  const loadInvites = async () => {
    setLoading(true);
    setError('');
    try {
      const res = await apiFetch(`/api/v1/admin/invites?page=${page}&size=20`);
      const data = await res.json();
      setInvites(data.invites || []);
      setTotal(data.total || 0);
    } catch (e: any) {
      setError('加载邀请列表失败');
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadInvites();
  }, [page]);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setCreating(true);
    setCreateError('');
    try {
      const res = await apiFetch('/api/v1/admin/invites', {
        method: 'POST',
        body: JSON.stringify({ email: email.trim() || undefined, role, expire_hours: expireHours }),
      });
      const data = await res.json();
      if (!res.ok) {
        setCreateError(data.error || '创建邀请失败');
        return;
      }
      setGeneratedURL(data.invite_url);
      setShowForm(false);
      setEmail('');
      loadInvites();
    } catch {
      setCreateError('网络错误');
    } finally {
      setCreating(false);
    }
  };

  const handleRevoke = async (inviteID: string) => {
    try {
      await apiFetch(`/api/v1/admin/invites/${inviteID}`, { method: 'DELETE' });
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
      case 'pending': return '待使用';
      case 'accepted': return '已注册';
      case 'expired': return '已过期';
      case 'revoked': return '已撤销';
      default: return status;
    }
  };

  return (
    <div className="animate-fade-in">
      <div className="mb-8 flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold text-[var(--text-primary)]" data-testid="invites-page-header">邀请管理</h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">生成和管理邀请注册链接</p>
        </div>
        <button
          onClick={() => { setShowForm(!showForm); setGeneratedURL(''); setCreateError(''); }}
          className="px-4 py-2 rounded-xl font-medium text-sm transition-all"
          style={{ background: 'linear-gradient(135deg, #B1E2FF, #9381FF)', color: '#000' }}
          data-testid="invites-create-btn"
        >
          + 生成邀请
        </button>
      </div>

      {/* Generated URL display */}
      {generatedURL && (
        <div className="mb-6 p-4 rounded-xl bg-[#B1E2FF]/10 border border-[#B1E2FF]/20" data-testid="invites-url-display">
          <p className="text-sm text-[var(--text-secondary)] mb-2">邀请链接已生成：</p>
          <div className="flex items-center gap-2">
            <code className="flex-1 px-3 py-2 rounded-lg bg-black/20 text-[#B1E2FF] text-sm break-all" data-testid="invites-url-text">{generatedURL}</code>
            <button
              onClick={handleCopyURL}
              className="px-3 py-2 rounded-lg text-sm font-medium bg-[#B1E2FF]/20 text-[#B1E2FF] hover:bg-[#B1E2FF]/30 transition-all"
              data-testid="invites-copy-btn"
            >
              复制
            </button>
          </div>
        </div>
      )}

      {/* Create form */}
      {showForm && (
        <div className="mb-6 glass p-6" style={{ background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.08)', borderRadius: '12px' }} data-testid="invites-create-form">
          <form onSubmit={handleCreate} className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div data-testid="invites-email-field">
                <label className="block mb-1 text-xs text-[var(--text-secondary)]">邮箱（可选）</label>
                <input
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="newuser@company.com"
                  className="w-full px-3 py-2 rounded-lg bg-white/5 border border-white/10 text-[var(--text-primary)] text-sm placeholder-white/30 focus:outline-none focus:border-[#B1E2FF]"
                  data-testid="invites-email-input"
                />
              </div>
              <div data-testid="invites-role-field">
                <label className="block mb-1 text-xs text-[var(--text-secondary)]">角色</label>
                <select
                  value={role}
                  onChange={(e) => setRole(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg bg-white/5 border border-white/10 text-[var(--text-primary)] text-sm focus:outline-none focus:border-[#B1E2FF]"
                  data-testid="invites-role-select"
                >
                  <option value="user">普通用户 (user)</option>
                  <option value="admin">管理员 (admin)</option>
                </select>
              </div>
              <div data-testid="invites-expire-field">
                <label className="block mb-1 text-xs text-[var(--text-secondary)]">有效期</label>
                <select
                  value={expireHours}
                  onChange={(e) => setExpireHours(Number(e.target.value))}
                  className="w-full px-3 py-2 rounded-lg bg-white/5 border border-white/10 text-[var(--text-primary)] text-sm focus:outline-none focus:border-[#B1E2FF]"
                  data-testid="invites-expire-select"
                >
                  <option value={24}>24 小时</option>
                  <option value={48}>48 小时</option>
                  <option value={168}>7 天</option>
                  <option value={720}>30 天</option>
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
                {creating ? '生成中...' : '确认生成'}
              </button>
              <button
                type="button"
                onClick={() => setShowForm(false)}
                className="px-4 py-2 rounded-lg text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                data-testid="invites-cancel-btn"
              >
                取消
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Invite list */}
      {error && <p className="text-red-400 mb-4 text-sm">{error}</p>}

      {loading ? (
        <p className="text-[var(--text-secondary)] text-sm">加载中...</p>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="w-full text-sm" data-testid="invites-table">
              <thead>
                <tr className="border-b border-white/10 text-left">
                  <th className="py-3 px-4 text-[var(--text-secondary)] font-medium">邮箱</th>
                  <th className="py-3 px-4 text-[var(--text-secondary)] font-medium">角色</th>
                  <th className="py-3 px-4 text-[var(--text-secondary)] font-medium">状态</th>
                  <th className="py-3 px-4 text-[var(--text-secondary)] font-medium">创建时间</th>
                  <th className="py-3 px-4 text-[var(--text-secondary)] font-medium">过期时间</th>
                  <th className="py-3 px-4 text-[var(--text-secondary)] font-medium">操作</th>
                </tr>
              </thead>
              <tbody>
                {invites.map((inv) => (
                  <tr key={inv.invite_id} className="border-b border-white/5 hover:bg-white/5" data-testid="invites-row">
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
                          撤销
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
                {invites.length === 0 && (
                  <tr>
                    <td colSpan={6} className="py-8 text-center text-[var(--text-secondary)]">暂无邀请记录</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {total > 20 && (
            <div className="flex justify-between items-center mt-4">
              <span className="text-xs text-[var(--text-secondary)]">共 {total} 条</span>
              <div className="flex gap-2">
                <button
                  disabled={page <= 1}
                  onClick={() => setPage(page - 1)}
                  className="px-3 py-1 rounded text-xs text-[var(--text-secondary)] disabled:opacity-30"
                >
                  上一页
                </button>
                <button
                  disabled={page * 20 >= total}
                  onClick={() => setPage(page + 1)}
                  className="px-3 py-1 rounded text-xs text-[var(--text-secondary)] disabled:opacity-30"
                >
                  下一页
                </button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
