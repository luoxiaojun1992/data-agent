'use client';

import React, { useState } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import Sidebar from './components/Sidebar';
import NotificationBell from './components/NotificationBell';
import IdleTimer from './components/IdleTimer';
import ScrollToTop from './components/ScrollToTop';
import ChangePasswordModal from './components/ChangePasswordModal';
import { useAuth } from '@/lib/api';

export default function AppLayout({ children }: { children: React.ReactNode }) {
  const { auth, logout } = useAuth();
  const router = useRouter();
  const pathname = usePathname();
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [showForcePwd, setShowForcePwd] = useState(false);

  // Close sidebar on route change (mobile UX)
  React.useEffect(() => { setSidebarOpen(false); }, [pathname]);

  // Redirect to login if not authenticated (only after localStorage is read)
  React.useEffect(() => {
    if (auth.hydrated && !auth.token && pathname !== '/login') {
      router.push('/login?expired=true');
    }
  }, [auth.hydrated, auth.token, pathname, router]);

  // Show nothing until localStorage is read to prevent flash-redirect
  if (!auth.hydrated) {
    return null;
  }

  if (!auth.token) {
    return <>{children}</>;
  }

  return (
    <div className="flex min-h-screen">
      {/* Mobile overlay backdrop */}
      {sidebarOpen && (
        <div
          className="lg:hidden fixed inset-0 bg-black/50 z-40"
          onClick={() => setSidebarOpen(false)}
          data-testid="sidebar-overlay"
        />
      )}

      {/* Sidebar: always visible on lg+, togglable on mobile */}
      <div
        className={`fixed lg:static z-50 transition-transform duration-300 ease-in-out ${
          sidebarOpen ? 'translate-x-0' : '-translate-x-full'
        } lg:translate-x-0`}
      >
        <Sidebar username={auth.username} role={auth.role} permissions={auth.permissions} onLogout={logout} onToggle={() => setSidebarOpen(false)} collapsed={sidebarCollapsed} onCollapseToggle={() => setSidebarCollapsed(!sidebarCollapsed)} />
      </div>

      <main className={`flex-1 ${sidebarCollapsed ? 'lg:ml-16' : 'lg:ml-60'} ml-0`} data-testid="main-content">
        {/* 右侧预留 96px（24 边距 + 66 指示灯宽 + 6 间距）给全局在线指示灯（SPEC-079），避免遮挡通知铃铛 */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '12px 96px 0 24px' }}>
          {/* Hamburger button — visible only on mobile */}
          <button
            className="lg:hidden p-2 rounded-lg hover:bg-[var(--glass-bg)] text-[var(--text-primary)]"
            onClick={() => setSidebarOpen(true)}
            data-testid="sidebar-hamburger"
            aria-label="Open menu"
          >
            ☰
          </button>
          <div style={{ flex: 1 }} />
          <NotificationBell />
          <IdleTimer />
        </div>
        <div className="p-8 pt-4">
          {/* 首次登录未改密提示横幅（SPEC-083）：后端 need_change_pw=true 时横向提示修改 */}
          {auth.needChangePw && (
            <div
              className="mb-5 px-4 py-3 rounded-xl flex items-center justify-between gap-4 flex-wrap"
              style={{ background: 'rgba(245,158,11,0.10)', border: '1px solid rgba(245,158,11,0.30)' }}
              data-testid="change-password-banner"
            >
              <span className="text-sm" style={{ color: '#fbbf24' }}>
                为保障账号安全，请尽快修改初始密码
              </span>
              <button
                type="button"
                onClick={() => setShowForcePwd(true)}
                className="px-3 py-1.5 rounded-lg text-sm font-medium transition-colors"
                style={{ background: 'rgba(245,158,11,0.20)', color: '#fbbf24' }}
                data-testid="change-password-banner-btn"
              >
                去修改
              </button>
            </div>
          )}
          {children}
        </div>
      </main>
      <ScrollToTop />
      {showForcePwd && (
        <ChangePasswordModal
          notice="为保障账号安全，请修改初始密码"
          onClose={() => setShowForcePwd(false)}
          onSuccess={() => {
            setShowForcePwd(false);
            logout();
            router.push('/login?pwd_changed=true');
          }}
        />
      )}
    </div>
  );
}
