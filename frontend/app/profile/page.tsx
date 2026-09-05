'use client';

import React, { useState } from 'react';
import { useRouter } from 'next/navigation';
import AppLayout from '../providers';
import ChangePasswordModal from '../components/ChangePasswordModal';
import { useAuth } from '../../lib/api';

// 角色展示名映射（与侧边栏角色展示一致）。
const roleLabel = (role?: string | null): string => {
  switch (role) {
    case 'system_admin': return '系统管理员';
    case 'admin': return '普通管理员';
    case 'user': return '普通用户';
    default: return role || '—';
  }
};

export default function ProfilePage() {
  const { auth, logout } = useAuth();
  const router = useRouter();
  const [showModal, setShowModal] = useState(false);
  const [successMsg, setSuccessMsg] = useState('');

  const handleSuccess = () => {
    setSuccessMsg('密码修改成功，请使用新密码重新登录');
    setTimeout(() => {
      logout();
      router.push('/login');
    }, 2000);
  };

  return (
    <AppLayout>
      <div className="animate-fade-in" data-testid="profile-page">
        <div className="mb-8">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">用户中心</h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">查看个人信息与账号安全设置</p>
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
                {auth.username || '未登录'}
              </p>
              <p className="text-sm text-[var(--text-secondary)] mt-0.5" data-testid="profile-role">
                {roleLabel(auth.role)}
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
              <p className="text-base font-medium text-[var(--text-primary)]">修改密码</p>
              <p className="text-sm text-[var(--text-secondary)] mt-0.5">更新当前账号的登录密码</p>
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
