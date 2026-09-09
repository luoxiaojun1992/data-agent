'use client';

import React from 'react';
import AppLayout from '../providers';

export default function HermesPage() {
  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-8" data-testid="hermes-page-header">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]" data-testid="hermes-page-title">
            Hermes 自由探索
          </h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">
            数据库自由查询 — 直接探索和分析你的数据
          </p>
        </div>

        <div className="glass p-12 text-center" data-testid="hermes-under-construction">
          <span className="text-5xl block mb-4">🚧</span>
          <p className="text-lg text-[var(--text-primary)] mb-2" data-testid="hermes-uc-title">
            功能建设中
          </p>
          <p className="text-sm text-[var(--text-secondary)]">
            敬请期待，Hermes 自由探索功能即将上线
          </p>
        </div>
      </div>
    </AppLayout>
  );
}
