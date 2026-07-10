'use client';

import React from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';

interface SidebarProps {
  username?: string | null;
  role?: string | null;
  onLogout: () => void;
}

const navItems = [
  { href: '/', label: '仪表盘', icon: '◉' },
  { href: '/chat', label: 'Chat 对话', icon: '💬' },
  { href: '/agent', label: 'Agent 任务', icon: '⚡' },
  { href: '/knowledge', label: '知识库', icon: '📚' },
  { href: '/docs', label: '文档', icon: '📄' },
  { href: '/admin', label: '管理后台', icon: '⚙' },
];

export default function Sidebar({ username, role, onLogout }: SidebarProps) {
  const pathname = usePathname();

  return (
    <aside className="w-60 h-screen fixed left-0 top-0 flex flex-col border-r border-[var(--border-glass)] bg-[var(--bg-secondary)] z-40">
      {/* Logo */}
      <div className="p-5 border-b border-[var(--border-glass)]">
        <Link href="/" className="flex items-center gap-3 no-underline">
          <span className="text-2xl">🔮</span>
          <div>
            <h1 className="text-base font-semibold text-[var(--text-primary)]">DataAgent</h1>
            <p className="text-xs text-[var(--text-secondary)]">企业数据分析平台</p>
          </div>
        </Link>
      </div>

      {/* Navigation */}
      <nav className="flex-1 p-3 space-y-1 overflow-y-auto">
        {navItems.map((item) => {
          const isActive = pathname === item.href;
          return (
            <Link
              key={item.href}
              href={item.href}
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
          <div className="w-8 h-8 rounded-full bg-[var(--accent)] flex items-center justify-center text-sm font-semibold">
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
