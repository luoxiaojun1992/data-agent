'use client';

import React from 'react';
import { useTheme } from '../../lib/theme';

export default function ThemeToggle() {
  const { theme, toggle } = useTheme();
  const isDark = theme === 'dark';
  return (
    <button
      type="button"
      onClick={toggle}
      data-testid="theme-toggle"
      aria-label={isDark ? '切换到浅色主题' : '切换到深色主题'}
      title={isDark ? '切换到浅色主题' : '切换到深色主题'}
      className="p-2 rounded-lg hover:bg-[var(--glass-bg)] text-[var(--text-secondary)] transition-colors"
    >
      {isDark ? '☀️' : '🌙'}
    </button>
  );
}
