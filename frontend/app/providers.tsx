'use client';

import React, { useState } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import Sidebar from './components/Sidebar';
import NotificationBell from './components/NotificationBell';
import { useAuth } from '@/lib/api';

export default function AppLayout({ children }: { children: React.ReactNode }) {
  const { auth, logout } = useAuth();
  const router = useRouter();
  const pathname = usePathname();
  const [sidebarOpen, setSidebarOpen] = useState(false);

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
        <Sidebar username={auth.username} role={auth.role} onLogout={logout} onToggle={() => setSidebarOpen(false)} />
      </div>

      <main className="flex-1 lg:ml-60 ml-0" data-testid="main-content">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '12px 24px 0 24px' }}>
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
        </div>
        <div className="p-8 pt-4">
          {children}
        </div>
      </main>
    </div>
  );
}
