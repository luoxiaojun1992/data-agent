'use client';

import React from 'react';
import { useRouter, usePathname } from 'next/navigation';
import Sidebar from './components/Sidebar';
import NotificationBell from './components/NotificationBell';
import { useAuth } from '@/lib/api';

export default function AppLayout({ children }: { children: React.ReactNode }) {
  const { auth, logout } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

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
      <Sidebar username={auth.username} role={auth.role} onLogout={logout} />
      <main className="flex-1 ml-60" data-testid="main-content">
        <div style={{ display: 'flex', justifyContent: 'flex-end', padding: '12px 24px 0 24px' }}>
          <NotificationBell />
        </div>
        <div className="p-8 pt-4">
          {children}
        </div>
      </main>
    </div>
  );
}
