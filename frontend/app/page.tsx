'use client';

import React from 'react';
import AppLayout from './providers';

export default function MainPage() {
  return (
    <AppLayout>
      <div className="animate-fade-in">
        {/* Header */}
        <div className="mb-8" data-testid="page-header">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]" data-testid="page-title">仪表盘</h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">系统概览与快速访问</p>
        </div>

        {/* Stats cards */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
          {[
            { label: '活跃 Chat 会话', value: '12', icon: '💬', trend: '+3' },
            { label: 'Agent 任务', value: '45', icon: '⚡', trend: '+8' },
            { label: '知识库文档', value: '128', icon: '📚', trend: '+12' },
            { label: '系统可用率', value: '99.9%', icon: '🟢', trend: '稳定' },
          ].map((stat) => (
            <div key={stat.label} className="glass p-5 glass-hover transition-all">
              <div className="flex items-center justify-between mb-3">
                <span className="text-2xl">{stat.icon}</span>
                <span className="text-xs text-emerald-400 bg-emerald-400/10 px-2 py-0.5 rounded-full">
                  {stat.trend}
                </span>
              </div>
              <p className="text-2xl font-bold text-[var(--text-primary)]">{stat.value}</p>
              <p className="text-sm text-[var(--text-secondary)] mt-1">{stat.label}</p>
            </div>
          ))}
        </div>

        {/* Quick actions */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <div className="glass p-6">
            <h3 className="text-lg font-semibold text-[var(--text-primary)] mb-4">快速开始</h3>
            <div className="space-y-3">
              {[
                { title: 'Chat 数据分析', desc: '通过自然语言对话进行数据查询与分析', href: '/chat', icon: '💬' },
                { title: 'Agent 批量任务', desc: '创建自动化分析任务，批量处理数据', href: '/agent', icon: '⚡' },
                { title: '知识库管理', desc: '上传文档，构建企业知识库', href: '/knowledge', icon: '📚' },
              ].map((item) => (
                <a
                  key={item.href}
                  href={item.href}
                  className="flex items-center gap-4 p-4 rounded-xl bg-[var(--glass-bg)] hover:bg-[var(--glass-hover)] transition-all no-underline"
                >
                  <span className="text-2xl">{item.icon}</span>
                  <div>
                    <p className="text-sm font-medium text-[var(--text-primary)]">{item.title}</p>
                    <p className="text-xs text-[var(--text-secondary)]">{item.desc}</p>
                  </div>
                </a>
              ))}
            </div>
          </div>

          <div className="glass p-6">
            <h3 className="text-lg font-semibold text-[var(--text-primary)] mb-4">最近活动</h3>
            <div className="space-y-3">
              {[
                { action: '完成数据分析任务', time: '2 分钟前', user: '系统管理员' },
                { action: '上传知识库文档', time: '15 分钟前', user: '系统管理员' },
                { action: '创建 Chat 会话', time: '1 小时前', user: '系统管理员' },
                { action: '更新系统配置', time: '2 小时前', user: '系统管理员' },
              ].map((activity, i) => (
                <div key={i} className="flex items-center gap-3 text-sm">
                  <div className="w-1.5 h-1.5 rounded-full bg-[var(--accent)] shrink-0" />
                  <span className="text-[var(--text-primary)] flex-1">{activity.action}</span>
                  <span className="text-xs text-[var(--text-secondary)]">{activity.time}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </AppLayout>
  );
}
