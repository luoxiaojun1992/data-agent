'use client';

import React, { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import AppLayout from '../providers';
import ChangePasswordModal from '../components/ChangePasswordModal';
import { useAuth } from '../../lib/api';

export default function ProfilePage() {
  const { auth, logout } = useAuth();
  const t = useTranslations('profile');
  const ta = useTranslations('auth');
  const tr = useTranslations('role');
  const tn = useTranslations('nav');
  const router = useRouter();
  const [showModal, setShowModal] = useState(false);
  const [successMsg, setSuccessMsg] = useState('');

  // 角色展示名映射（与侧边栏角色展示一致）。
  const roleText = (role?: string | null): string => {
    switch (role) {
      case 'system_admin': return tr('systemAdmin');
      case 'admin': return tr('admin');
      case 'user': return tr('user');
      default: return role || '—';
    }
  };

  const handleSuccess = () => {
    setSuccessMsg(ta('pwdChanged'));
    setTimeout(() => {
      logout();
      router.push('/login');
    }, 2000);
  };

  return (
    <AppLayout>
      <div className="animate-fade-in" data-testid="profile-page">
        <div className="mb-8">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">{t('title')}</h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">{t('desc')}</p>
        </div>

        {successMsg && (
          <div
            data-testid="profile-success-toast"
            className="mb-5 px-4 py-3 rounded-xl text-sm"
            style={{ background: 'rgba(16,185,129,0.1)', color: '#10b981' }}
          >
            {successMsg}
          </div>
        )}

        <div className="grid gap-6 max-w-2xl">
          {/* 用户信息卡片 */}
          <div
            className="glass rounded-2xl p-6 flex items-center gap-4"
            data-testid="profile-info-card"
          >
            <div
              className="w-14 h-14 rounded-full flex items-center justify-center text-xl font-semibold"
              style={{ background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)', color: '#fff' }}
              data-testid="profile-avatar"
            >
              {auth.username?.[0]?.toUpperCase() || '?'}
            </div>
            <div>
              <p className="text-base font-semibold text-[var(--text-primary)]" data-testid="profile-username">
                {auth.username || tn('notLoggedIn')}
              </p>
              <p className="text-sm text-[var(--text-secondary)] mt-0.5" data-testid="profile-role">
                {roleText(auth.role)}
              </p>
            </div>
          </div>

          {/* 修改密码卡片 */}
          <div
            className="glass rounded-2xl p-6 flex items-center justify-between cursor-pointer hover:bg-[var(--glass-hover)] transition-colors"
            data-testid="profile-pwd-card"
            onClick={() => setShowModal(true)}
          >
            <div>
              <p className="text-base font-medium text-[var(--text-primary)]">{t('changePwdTitle')}</p>
              <p className="text-sm text-[var(--text-secondary)] mt-0.5">{t('changePwdDesc')}</p>
            </div>
            <span className="text-[var(--text-secondary)]">›</span>
          </div>
        </div>
      </div>

      {showModal && (
        <ChangePasswordModal
          onClose={() => setShowModal(false)}
          onSuccess={handleSuccess}
        />
      )}
    </AppLayout>
  );
}
