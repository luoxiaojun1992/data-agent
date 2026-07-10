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

        <div className="glass p-8 text-center" data-testid="hermes-search-area">
          <span className="text-5xl block mb-4">🔍</span>
          <p className="text-lg text-[var(--text-primary)] mb-2">连接数据库并开始探索</p>
          <p className="text-sm text-[var(--text-secondary)] mb-6">
            输入数据库连接信息，Hermes 将帮助你发现数据洞察
          </p>
          <div className="max-w-lg mx-auto">
            <textarea
              placeholder="在此输入自然语言查询，例如：查询 sales 表中过去 30 天的收入趋势..."
              rows={3}
              className="w-full px-4 py-3 rounded-xl bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] placeholder-[var(--text-secondary)] resize-none focus:outline-none focus:border-[var(--accent)] transition-all"
              data-testid="hermes-query-input"
            />
            <button
              className="mt-3 px-6 py-2 bg-[var(--accent)] text-white rounded-xl font-medium hover:opacity-90 transition-all"
              data-testid="hermes-submit-btn"
            >
              执行查询
            </button>
          </div>
        </div>
      </div>
    </AppLayout>
  );
}
