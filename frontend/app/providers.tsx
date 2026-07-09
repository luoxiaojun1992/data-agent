'use client';

import React from 'react';
import { useRouter, usePathname } from 'next/navigation';
import Sidebar from './components/Sidebar';
import { useAuth } from '@/lib/api';

export default function AppLayout({ children }: { children: React.ReactNode }) {
  const { auth, logout } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  // Redirect to login if not authenticated
  React.useEffect(() => {
    if (!auth.token && pathname !== '/login') {
      router.push('/login?expired=true');
    }
  }, [auth.token, pathname, router]);

  if (!auth.token) {
    return <>{children}</>;
  }

  return (
    <div className="flex min-h-screen">
      <Sidebar username={auth.username} role={auth.role} onLogout={logout} />
      <main className="flex-1 ml-60 p-8">
        {children}
      </main>
    </div>
  );
}
