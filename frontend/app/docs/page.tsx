'use client';

import React from 'react';
import AppLayout from '../providers';

const documents = [
  { title: 'PRD — 产品需求文档', desc: '企业数据分析 Agent MVP 功能定义', path: '/docs/PRD-企业数据分析Agent-MVP.md' },
  { title: 'RFC — 技术方案', desc: '系统架构与后端技术选型', path: '/docs/RFC-企业数据分析Agent-技术方案.md' },
  { title: 'Roadmap — 开发计划', desc: 'MVP 阶段开发路线图', path: '/docs/Roadmap-企业数据分析Agent-MVP.md' },
  { title: 'UI 原型设计', desc: '前端 UI 设计与交互原型', path: '/docs/UI原型设计文档.md' },
  { title: '架构总览', desc: '系统架构概览与模块说明', path: '/docs/ARCHITECTURE.zh-CN.md' },
];

export default function DocsPage() {
  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-8">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">项目文档</h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">设计文档与系统说明</p>
        </div>

        <div className="space-y-3">
          {documents.map((doc) => (
            <a
              key={doc.path}
              href={doc.path}
              target="_blank"
              rel="noopener noreferrer"
              className="glass p-5 glass-hover flex items-center gap-4 no-underline"
            >
              <span className="text-2xl">📄</span>
              <div>
                <p className="text-sm font-medium text-[var(--text-primary)]">{doc.title}</p>
                <p className="text-xs text-[var(--text-secondary)] mt-0.5">{doc.desc}</p>
              </div>
            </a>
          ))}
        </div>
      </div>
    </AppLayout>
  );
}
