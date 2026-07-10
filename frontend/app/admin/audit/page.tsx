'use client';
import React from 'react';
import AppLayout from '../../../providers';
export default function auditPage() {
  return (<AppLayout><div className="animate-fade-in">
    <div className="mb-8" data-testid="admin-audit-header"><h2 className="text-2xl font-bold text-[var(--text-primary)]" data-testid="admin-audit-title">审计日志</h2></div>
    <div className="glass p-12 text-center" data-testid="admin-audit-empty"><p className="text-lg text-[var(--text-primary)]">审计日志</p><p className="text-sm text-[var(--text-secondary)]">此功能后续版本提供</p></div>
  </div></AppLayout>);
}
