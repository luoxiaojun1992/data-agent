'use client';
import React from 'react';
import AppLayout from '../../../providers';
export default function tasksPage() {
  return (<AppLayout><div className="animate-fade-in">
    <div className="mb-8" data-testid="admin-tasks-header"><h2 className="text-2xl font-bold text-[var(--text-primary)]" data-testid="admin-tasks-title">任务管理</h2></div>
    <div className="glass p-12 text-center" data-testid="admin-tasks-empty"><p className="text-lg text-[var(--text-primary)]">任务管理</p><p className="text-sm text-[var(--text-secondary)]">此功能后续版本提供</p></div>
  </div></AppLayout>);
}
