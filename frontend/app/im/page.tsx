'use client';

import React from 'react';
import AppLayout from '../providers';
import { useRouter } from 'next/navigation';

export default function IMPage() {
  const router = useRouter();

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-6" data-testid="im-header">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">IM 集成</h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">连接即时通讯平台，通过对话与 AI 交互</p>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {/* Feishu Card */}
          <button
            onClick={() => router.push('/im/feishu')}
            className="glass p-6 rounded-2xl text-left hover:bg-[var(--glass-hover)] transition-colors"
            data-testid="im-feishu-card"
          >
            <div className="text-3xl mb-3">🐦</div>
            <h3 className="text-lg font-semibold text-[var(--text-primary)] mb-1">飞书</h3>
            <p className="text-sm text-[var(--text-secondary)]">接入飞书机器人，在飞书中与 AI 对话分析数据</p>
          </button>
        </div>
      </div>
    </AppLayout>
  );
}
