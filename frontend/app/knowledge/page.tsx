'use client';

import React from 'react';
import AppLayout from '../providers';

export default function KnowledgePage() {
  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-8">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">知识库</h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">企业知识文档管理与搜索</p>
        </div>

        <div className="glass p-12 text-center">
          <span className="text-5xl block mb-4">📚</span>
          <p className="text-lg text-[var(--text-primary)] mb-2">知识库管理</p>
          <p className="text-sm text-[var(--text-secondary)]">
            上传文档、构建知识库、智能搜索 — 全量 API 将在 Phase 3 完善
          </p>
        </div>
      </div>
    </AppLayout>
  );
}
