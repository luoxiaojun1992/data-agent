'use client';

import React from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';

interface SidebarProps {
  username?: string | null;
  role?: string | null;
  onLogout: () => void;
  onToggle?: () => void;
}

const navItems = [
  { href: '/', label: '仪表盘', icon: '◉', testid: 'nav-dashboard', roles: ['user', 'admin', 'system_admin'] as string[] },
  { href: '/chat', label: 'Chat 对话', icon: '💬', testid: 'nav-chat', roles: ['user', 'admin', 'system_admin'] },
  { href: '/hermes', label: 'Hermes 探索', icon: '🔍', testid: 'nav-hermes', roles: ['user', 'admin', 'system_admin'] },
  { href: '/agent', label: 'Agent 任务', icon: '⚡', testid: 'nav-agent', roles: ['admin', 'system_admin'] },
  { href: '/knowledge', label: '知识库', icon: '📚', testid: 'nav-kb-mgmt', roles: ['user', 'admin', 'system_admin'] },
  { href: '/docs', label: '文档', icon: '📄', testid: 'nav-docs', roles: ['user', 'admin', 'system_admin'] },
  { href: '/admin', label: '管理后台', icon: '⚙', testid: 'nav-admin', roles: ['admin', 'system_admin'] },
];

export default function Sidebar({ username, role, onLogout, onToggle }: SidebarProps) {
  const pathname = usePathname();
  const userRole = role || 'user';

  const visibleItems = navItems.filter(item => item.roles.includes(userRole));

  return (
    <aside className="w-60 h-screen flex flex-col border-r border-[var(--border-glass)] bg-[var(--bg-secondary)] z-50" data-testid="sidebar">
      {/* Logo */}
      <div className="p-5 border-b border-[var(--border-glass)] flex items-center justify-between" data-testid="sidebar-logo">
        <Link href="/" className="flex items-center gap-3 no-underline">
          <span className="text-2xl" data-testid="sidebar-logo-icon">🔮</span>
          <div>
            <h1 className="text-base font-semibold text-[var(--text-primary)]" data-testid="sidebar-logo-text">DataAgent</h1>
            <p className="text-xs text-[var(--text-secondary)]">企业数据分析平台</p>
          </div>
        </Link>
        {/* Close button — visible only on mobile */}
        {onToggle && (
          <button
            className="lg:hidden p-1.5 rounded-lg hover:bg-[var(--glass-hover)] text-[var(--text-secondary)]"
            onClick={onToggle}
            data-testid="sidebar-close"
            aria-label="Close menu"
          >
            ✕
          </button>
        )}
      </div>

      {/* Navigation */}
      <nav className="flex-1 p-3 space-y-1 overflow-y-auto">
        {visibleItems.map((item) => {
          const isActive = pathname === item.href;
          return (
            <Link
              key={item.href}
              href={item.href}
              data-testid={item.testid}
              className={`flex items-center gap-3 px-4 py-2.5 rounded-xl text-sm no-underline transition-all duration-200 ${
                isActive
                  ? 'bg-[var(--glass-hover)] text-[var(--accent)] font-medium'
                  : 'text-[var(--text-secondary)] hover:bg-[var(--glass-bg)] hover:text-[var(--text-primary)]'
              }`}
            >
              <span className="text-lg">{item.icon}</span>
              <span>{item.label}</span>
            </Link>
          );
        })}
      </nav>

      {/* User section */}
      <div className="p-4 border-t border-[var(--border-glass)]" data-testid="nav-user-card">
        <div className="flex items-center gap-3 mb-3">
          <div className="w-8 h-8 rounded-full bg-[var(--accent)] flex items-center justify-center text-sm font-semibold" data-testid="user-avatar">
            {username?.[0]?.toUpperCase() || '?'}
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium text-[var(--text-primary)] truncate">
              {username || '未登录'}
            </p>
            <p className="text-xs text-[var(--text-secondary)]">{role || '—'}</p>
          </div>
        </div>
        <button
          onClick={onLogout}
          className="w-full py-2 text-sm text-[var(--text-secondary)] hover:text-red-400 hover:bg-red-400/10 rounded-lg transition-colors"
          data-testid="nav-logout-btn"
        >
          退出登录
        </button>
      </div>
    </aside>
  );
}
