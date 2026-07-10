'use client';
import React from 'react';
import AppLayout from '../../../providers';
export default function UsersPage() {
  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-8" data-testid="admin-users-header">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]" data-testid="admin-users-title">用户管理</h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">用户数据管理</p>
        </div>
        <div className="glass p-12 text-center" data-testid="admin-users-empty">
          <p className="text-lg text-[var(--text-primary)] mb-2">用户管理</p>
          <p className="text-sm text-[var(--text-secondary)]">此功能将在后续版本中提供</p>
        </div>
      </div>
    </AppLayout>
  );
}
