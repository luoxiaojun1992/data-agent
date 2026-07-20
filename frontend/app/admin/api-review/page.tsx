'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '../../../lib/api';

interface APIReview {
  id: string;
  name: string;
  file_name: string;
  version: string;
  endpoints: number;
  domain: string;
  rate_limit: number;
  submitter: string;
  reviewer?: string;
  reject_reason?: string;
  status: string;
  created_at: string;
  reviewed_at?: string;
}

export default function APIReviewPage() {
  const { auth, apiFetch } = useAuth();
  const [reviews, setReviews] = useState<APIReview[]>([]);
  const [showUpload, setShowUpload] = useState(false);
  const [showReject, setShowReject] = useState<string | null>(null);
  const [rejectReason, setRejectReason] = useState('');
  const [form, setForm] = useState({ name: '', file_name: '', domain: '', version: '3.0', endpoints: 0, rate_limit: 100 });
  const [toast, setToast] = useState<{ msg: string; type: 'success' | 'error' } | null>(null);
  const [error, setError] = useState('');

  const notify = (msg: string, type: 'success' | 'error') => {
    setToast({ msg, type });
    setTimeout(() => setToast(null), 3000);
  };

  const fetchReviews = useCallback(async () => {
    try {
      const res = await apiFetch('/admin/api-reviews');
      if (res.ok) setReviews(await res.json());
    } catch { /* ignore */ }
  }, [apiFetch]);

  useEffect(() => {
    if (auth.hydrated) fetchReviews();
  }, [auth.hydrated, fetchReviews]);

  const handleUpload = async () => {
    try {
      const res = await apiFetch('/admin/api-reviews', {
        method: 'POST',
        body: JSON.stringify(form),
      });
      if (res.ok) {
        notify('提交成功，等待审核', 'success');
        setShowUpload(false);
        fetchReviews();
      } else {
        const d = await res.json();
        notify(d.error || '提交失败', 'error');
      }
    } catch { notify('提交失败', 'error'); }
  };

  const handleApprove = async (id: string) => {
    try {
      const res = await apiFetch(`/admin/api-reviews/${id}/approve`, { method: 'PUT' });
      if (res.ok) { notify('已批准', 'success'); fetchReviews(); }
      else { const d = await res.json(); setError(d.error || '操作失败'); }
    } catch { notify('操作失败', 'error'); }
  };

  const handleReject = async (id: string) => {
    try {
      const res = await apiFetch(`/admin/api-reviews/${id}/reject`, {
        method: 'PUT',
        body: JSON.stringify({ reason: rejectReason }),
      });
      if (res.ok) { notify('已驳回', 'success'); setShowReject(null); setRejectReason(''); fetchReviews(); }
      else { const d = await res.json(); setError(d.error || ''); }
    } catch { notify('操作失败', 'error'); }
  };

  const currentUserID = auth.userId || '';

  return (
    <AppLayout>
      <div className="animate-fade-in" data-testid="api-page-header">
        <div className="mb-8" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">API 转换审核</h2>
          <div style={{ display: 'flex', gap: '8px' }}>
            <button data-testid="api-upload-btn" onClick={() => setShowUpload(true)} style={primaryBtn}>+ 上传 OpenAPI</button>
            <button data-testid="api-batch-upload-btn" onClick={() => setShowUpload(true)} style={secondaryBtn}>📦 批量上传</button>
          </div>
        </div>

        {toast && (
          <div style={{ position: 'fixed', top: 20, right: 20, zIndex: 9999,
            background: toast.type === 'success' ? 'rgba(16,185,129,0.9)' : 'rgba(239,68,68,0.9)',
            color: '#fff', padding: '12px 20px', borderRadius: '8px', fontSize: '14px' }}>{toast.msg}</div>
        )}

        {/* Upload Modal */}
        {showUpload && (
          <div data-testid="api-upload-modal" style={modalOverlay} onClick={(e) => { if (e.target === e.currentTarget) setShowUpload(false); }}>
            <div className="glass" style={{ padding: '24px', maxWidth: '440px', width: '90%' }}>
              <h3 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '16px' }}>上传 OpenAPI 文件</h3>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                <input data-testid="api-upload-file" placeholder="文件名 (如 crm-api.yaml)" value={form.file_name}
                  onChange={(e) => setForm({ ...form, file_name: e.target.value })} style={inputStyle} />
                <input placeholder="API 名称" value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })} style={inputStyle} />
                <input placeholder="域名 (如 crm.company.cn)" value={form.domain}
                  onChange={(e) => setForm({ ...form, domain: e.target.value })} style={inputStyle} />
                <div style={{ display: 'flex', gap: '8px' }}>
                  <input placeholder="端点数" type="number" value={form.endpoints || ''}
                    onChange={(e) => setForm({ ...form, endpoints: Number(e.target.value) })} style={{ ...inputStyle, flex: 1 }} />
                  <div style={{ flex: 1 }}>
                    <label style={{ fontSize: '11px', color: '#666' }}>频率限制 (次/分钟)</label>
                    <input data-testid="api-upload-rate-limit" type="number" value={form.rate_limit}
                      onChange={(e) => setForm({ ...form, rate_limit: Number(e.target.value) })} style={inputStyle} />
                  </div>
                </div>
              </div>
              <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end', marginTop: '16px' }}>
                <button onClick={() => setShowUpload(false)} style={secondaryBtn}>取消</button>
                <button data-testid="api-upload-submit" onClick={handleUpload} style={primaryBtn}>确认</button>
              </div>
            </div>
          </div>
        )}

        {/* Reject Modal */}
        {showReject && (
          <div data-testid="api-reject-confirm" style={modalOverlay} onClick={(e) => { if (e.target === e.currentTarget) setShowReject(null); }}>
            <div className="glass" style={{ padding: '24px', maxWidth: '400px', width: '90%' }}>
              <h3 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '12px' }}>驳回原因</h3>
              <textarea data-testid="api-reject-reason" placeholder="请输入驳回原因" value={rejectReason}
                onChange={(e) => setRejectReason(e.target.value)}
                style={{ ...inputStyle, height: '80px', resize: 'vertical' }} />
              <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end', marginTop: '12px' }}>
                <button onClick={() => setShowReject(null)} style={secondaryBtn}>取消</button>
                <button onClick={() => handleReject(showReject)} style={{ ...primaryBtn, background: '#ef4444' }}>确认驳回</button>
              </div>
            </div>
          </div>
        )}

        {/* Error */}
        {error && (
          <div style={{ padding: '8px 16px', marginBottom: '12px', background: 'rgba(239,68,68,0.1)', borderRadius: '8px', color: '#ef4444', fontSize: '13px' }}>
            {error}
          </div>
        )}

        {/* API Cards */}
        {reviews.map((r) => {
          const isOwn = r.submitter === currentUserID;
          return (
            <div key={r.id} data-testid={`api-card-${r.id}`} className="glass"
              style={{ padding: '20px 24px', marginBottom: '12px', display: 'flex', alignItems: 'center', gap: '16px' }}>
              <div style={{ width: '44px', height: '44px', borderRadius: '10px', background: 'rgba(59,130,246,0.12)',
                display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: '20px' }}>🔌</div>
              <div style={{ flex: 1 }}>
                <p data-testid="api-card-name" style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '4px' }}>{r.name || r.file_name}</p>
                <p data-testid="api-card-desc" style={{ fontSize: '12px', color: '#7A7A7A' }}>
                  OpenAPI {r.version} · {r.endpoints || '?'} 个端点 · {r.submitter?.slice(0, 8) || '—'} · {r.created_at ? new Date(r.created_at).toLocaleDateString('zh-CN') : '—'}
                </p>
                <p data-testid="api-card-meta" style={{ fontSize: '12px', color: '#7A7A7A' }}>{r.domain} · {r.rate_limit}/min</p>
                {r.reject_reason && <p style={{ fontSize: '12px', color: '#ef4444', marginTop: '4px' }}>驳回原因: {r.reject_reason}</p>}
              </div>
              <div>
                <span data-testid={`api-status-${r.status}`} style={statusPill(r.status)}>{statusLabel(r.status)}</span>
              </div>
              <div data-testid={`api-card-actions-${r.id}`} style={{ display: 'flex', gap: '6px' }}>
                {r.status === 'pending' && !isOwn && (
                  <>
                    <button data-testid={`api-approve-btn-${r.id}`} onClick={() => handleApprove(r.id)}
                      style={actionBtn('#10b981')}>批准</button>
                    <button data-testid={`api-reject-btn-${r.id}`} onClick={() => setShowReject(r.id)}
                      style={actionBtn('#ef4444')}>驳回</button>
                  </>
                )}
                {r.status === 'pending' && isOwn && (
                  <span style={{ fontSize: '12px', color: '#666' }}>等待审核</span>
                )}
              </div>
            </div>
          );
        })}
        {reviews.length === 0 && (
          <div className="glass p-12 text-center"><span className="text-sm text-[var(--text-secondary)]">暂无 API 转换请求</span></div>
        )}
      </div>
    </AppLayout>
  );
}

const statusLabel = (s: string) => {
  const m: Record<string, string> = { pending: '待审核', approved: '已批准', rejected: '已驳回' };
  return m[s] || s;
};
const statusPill = (s: string): React.CSSProperties => {
  const colors: Record<string, string> = { pending: '#FBBF24', approved: '#34D399', rejected: '#FB7185' };
  return { display: 'inline-block', padding: '3px 12px', borderRadius: '10px', fontSize: '12px', fontWeight: 500,
    background: `${colors[s] || '#666'}20`, color: colors[s] || '#666' };
};
const actionBtn = (color: string): React.CSSProperties => ({
  background: 'transparent', border: `1px solid ${color}40`, color, borderRadius: '6px', padding: '3px 12px', fontSize: '12px', cursor: 'pointer',
});
const modalOverlay: React.CSSProperties = { position: 'fixed', inset: 0, zIndex: 999, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center' };
const inputStyle: React.CSSProperties = { width: '100%', padding: '8px 12px', background: 'rgba(255,255,255,0.06)', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '8px', fontSize: '14px', color: 'var(--text-primary)', outline: 'none', boxSizing: 'border-box' };
const primaryBtn: React.CSSProperties = { padding: '8px 20px', background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)', color: '#fff', border: 'none', borderRadius: '8px', fontSize: '14px', fontWeight: 600, cursor: 'pointer' };
const secondaryBtn: React.CSSProperties = { padding: '8px 20px', background: 'rgba(255,255,255,0.06)', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '8px', fontSize: '14px', color: '#7A7A7A', cursor: 'pointer' };
