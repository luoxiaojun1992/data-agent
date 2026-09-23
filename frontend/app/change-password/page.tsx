'use client';

import React, { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import AppLayout from '../providers';
import { useAuth } from '../../lib/api';

export default function ChangePasswordPage() {
  const { auth, apiFetch, logout } = useAuth();
  const t = useTranslations('pwd');
  const ta = useTranslations('auth');
  const router = useRouter();
  const [oldPwd, setOldPwd] = useState('');
  const [newPwd, setNewPwd] = useState('');
  const [confirmPwd, setConfirmPwd] = useState('');
  const [error, setError] = useState('');
  const [confirmError, setConfirmError] = useState('');
  const [success, setSuccess] = useState('');

  const showBanner = auth.needChangePw;

  const handleSubmit = async () => {
    setError('');
    setConfirmError('');

    if (newPwd !== confirmPwd) {
      setConfirmError(t('mismatch'));
      return;
    }

    try {
      const res = await apiFetch('/auth/change-password', {
        method: 'POST',
        body: JSON.stringify({ old_password: oldPwd, new_password: newPwd }),
      });
      if (res.ok) {
        setSuccess(ta('pwdChanged'));
        setTimeout(() => { logout(); router.push('/login'); }, 2000);
      } else {
        const d = await res.json();
        setError(d.error || t('changeFailed'));
      }
    } catch {
      setError(t('changeFailed'));
    }
  };

  return (
    <AppLayout>
      <div className="animate-fade-in" data-testid="pwd-page">
        <div className="mb-8">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">{t('title')}</h2>
        </div>

        {/* Success toast */}
        {success && (
          <div data-testid="pwd-change-success-toast" style={{ padding: '12px 16px', marginBottom: '20px',
            background: 'rgba(16,185,129,0.1)', borderRadius: '10px', color: '#10b981', fontSize: '13px' }}>
            {success}
          </div>
        )}

        {/* Initial password banner */}
        {showBanner && (
          <div data-testid="pwd-initial-banner" style={{ padding: '12px 16px', marginBottom: '20px',
            background: 'rgba(251,191,36,0.1)', borderRadius: '10px', color: '#FBBF24', fontSize: '13px' }}>
            ⚠️ {t('initialBanner')}
          </div>
        )}

        <div className="glass" style={{ padding: '24px', maxWidth: '440px' }}>
          <div style={{ marginBottom: '12px' }}>
            <label style={labelStyle}>{t('oldPwd')}</label>
            <input data-testid="pwd-old-input" type="password" value={oldPwd}
              onChange={(e) => setOldPwd(e.target.value)} style={inputStyle} />
          </div>
          <div style={{ marginBottom: '12px' }}>
            <label style={labelStyle}>{t('newPwd')}</label>
            <input data-testid="pwd-new-input" type="password" value={newPwd}
              onChange={(e) => setNewPwd(e.target.value)} style={inputStyle} />
          </div>
          <div style={{ marginBottom: '12px' }}>
            <label style={labelStyle}>{t('confirmPwd')}</label>
            <input data-testid="pwd-confirm-input" type="password" value={confirmPwd}
              onChange={(e) => setConfirmPwd(e.target.value)} style={inputStyle} />
            {confirmError && <p data-testid="pwd-confirm-error" style={{ color: '#ef4444', fontSize: '12px', marginTop: '4px' }}>{confirmError}</p>}
          </div>
          {error && <p data-testid="pwd-old-error" style={{ color: '#ef4444', fontSize: '12px', marginBottom: '8px' }}>{error}</p>}
          <button data-testid="pwd-change-btn" onClick={handleSubmit}
            style={{ width: '100%', padding: '10px', background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
              color: '#fff', border: 'none', borderRadius: '8px', fontSize: '14px', fontWeight: 600, cursor: 'pointer' }}>
            {t('submit')}
          </button>
        </div>
      </div>
    </AppLayout>
  );
}

const labelStyle: React.CSSProperties = { display: 'block', fontSize: '12px', color: '#666', marginBottom: '4px' };
const inputStyle: React.CSSProperties = { width: '100%', padding: '8px 12px', background: 'var(--surface-6)', border: '1px solid var(--surface-10)', borderRadius: '8px', fontSize: '14px', color: 'var(--text-primary)', outline: 'none', boxSizing: 'border-box' };
