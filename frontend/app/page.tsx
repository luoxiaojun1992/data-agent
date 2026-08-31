'use client';

import React, { useState, useEffect } from 'react';
import AppLayout from './providers';
import { useAuth } from '@/lib/api';

function getGreeting() {
  const h = new Date().getHours();
  if (h < 12) return '早上好';
  if (h < 18) return '下午好';
  return '晚上好';
}

type ChartProps = { testid: string; title: string; children: React.ReactNode };
function Chart({ testid, title, children }: ChartProps) {
  return (
    <div className="glass p-5" data-testid={testid}>
      <h4 className="text-xs font-semibold text-[var(--text-secondary)] mb-3 uppercase">{title}</h4>
      {children}
    </div>
  );
}

type Point = { time: string; value: number };

function formatLabel(time: string, gran: string): string {
  const d = new Date(time);
  if (Number.isNaN(d.getTime())) return '';
  if (gran === 'day') return `${String(d.getHours()).padStart(2, '0')}:00`;
  if (gran === 'year') return `${d.getFullYear()}/${d.getMonth() + 1}`;
  return `${d.getMonth() + 1}/${d.getDate()}`;
}

function TrendChart({ data, gran }: { data: Point[]; gran: string }) {
  const hasData = data.some(d => d.value > 0);
  if (!hasData) {
    return (
      <div className="flex items-center justify-center h-[100px] text-xs text-[var(--text-secondary)]">
        暂无数据
      </div>
    );
  }
  const max = Math.max(...data.map(d => d.value), 1);
  const showLabel = data.length <= 12 || gran === 'day';
  return (
    <div className="flex flex-col">
      {/* 柱子行：固定高度，柱体在行内底部对齐，不与标签行重叠 */}
      <div className="flex items-end gap-1" style={{ height: '80px' }}>
        {data.map((d, i) => (
          <div key={i} className="flex-1 rounded-t" style={{
            height: `${Math.max(4, (d.value / max) * 76)}px`,
            backgroundColor: 'var(--accent)',
            minHeight: '4px',
          }} />
        ))}
      </div>
      {/* 标签行：独立一行，固定间距，永不与柱体重合 */}
      {showLabel && (
        <div className="flex gap-1 mt-1">
          {data.map((d, i) => (
            <span key={i} className="flex-1 min-w-0 text-[8px] text-[var(--text-secondary)] truncate text-center">
              {gran === 'day' && data.length > 12 && i % 3 !== 0 ? '' : formatLabel(d.time, gran)}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

const GRANS = [
  { key: 'day', label: '日' },
  { key: 'week', label: '周' },
  { key: 'month', label: '月' },
  { key: 'year', label: '年' },
];

export default function MainPage() {
  const { apiFetch, auth } = useAuth();
  const [granularity, setGranularity] = useState('day');
  const [summary, setSummary] = useState<any>(null);
  const [trends, setTrends] = useState<any>(null);

  useEffect(() => {
    if (!auth.token) return;
    (async () => {
      try {
        const sr = await apiFetch('/dashboard');
        setSummary(await sr.json());
      } catch { /* ignore */ }
    })();
  }, [auth.token]);

  useEffect(() => {
    if (!auth.token) return;
    (async () => {
      try {
        const tr = await apiFetch(`/dashboard/trends?granularity=${granularity}`);
        setTrends(await tr.json());
      } catch { /* ignore */ }
    })();
  }, [auth.token, granularity]);

  const num = (v: any) => (typeof v === 'number' ? v : 0);

  const kpis = [
    { label: '知识库文档', value: num(summary?.kb_docs), icon: '📚', testid: 'dashboard-stat-kb' },
    { label: 'Token 消耗', value: num(summary?.token_tokens), icon: '🪙', testid: 'dashboard-stat-token' },
    { label: 'LLM 调用', value: num(summary?.llm_calls), icon: '🤖', testid: 'dashboard-stat-llm' },
    { label: 'API 调用', value: num(summary?.api_calls), icon: '🔌', testid: 'dashboard-stat-api' },
    { label: '产出物', value: num(summary?.artifact_created), icon: '📦', testid: 'dashboard-stat-artifact' },
    { label: '完成任务', value: num(summary?.task_completed), icon: '✅', testid: 'dashboard-stat-task' },
    { label: 'ROI', value: `${(num(summary?.roi) * 100).toFixed(1)}%`, icon: '📈', testid: 'dashboard-stat-roi' },
  ];

  const series: { key: string; title: string; testid: string }[] = [
    { key: 'token_tokens', title: 'Token 消耗趋势', testid: 'chart-token' },
    { key: 'llm_calls', title: 'LLM 调用趋势', testid: 'chart-llm' },
    { key: 'api_calls', title: 'API 调用趋势', testid: 'chart-api' },
    { key: 'artifact_created', title: '产出物趋势', testid: 'chart-artifact' },
    { key: 'task_completed', title: '任务完成趋势', testid: 'chart-task' },
    { key: 'roi', title: 'ROI 趋势', testid: 'chart-roi' },
  ];

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-6 flex items-center justify-between" data-testid="page-header">
          <div>
            <p className="text-lg font-semibold text-[var(--text-primary)]" data-testid="page-title">仪表盘</p>
            <p className="text-xs text-[var(--text-secondary)] mt-1" data-testid="dashboard-greeting">{getGreeting()}，欢迎回来 👋</p>
            <p className="text-xs text-[var(--text-secondary)] mt-1" data-testid="dashboard-date">
              {new Date().toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric', weekday: 'long' })}
            </p>
          </div>
          <div className="flex items-center gap-2" data-testid="dashboard-time-filter">
            {GRANS.map(g => (
              <button key={g.key} onClick={() => setGranularity(g.key)}
                data-testid={`filter-${g.key}`}
                className={`px-3 py-1 text-xs rounded-full transition-colors ${
                  granularity === g.key ? 'bg-[var(--accent)]/20 text-[var(--accent)]' : 'text-[var(--text-secondary)]'
                }`}
              >{g.label}</button>
            ))}
          </div>
        </div>

        {/* KPI cards */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
          {kpis.map((kpi) => (
            <div key={kpi.label} className="glass p-5 glass-hover" data-testid={kpi.testid}>
              <div className="flex items-center justify-between mb-3">
                <span className="text-2xl">{kpi.icon}</span>
              </div>
              <p className="text-2xl font-bold text-[var(--text-primary)]">{kpi.value}</p>
              <p className="text-sm text-[var(--text-secondary)] mt-1">{kpi.label}</p>
            </div>
          ))}
        </div>

        {/* Trend charts */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 mb-6">
          {series.map(s => (
            <Chart key={s.key} testid={s.testid} title={s.title}>
              <TrendChart data={(trends?.[s.key] || []) as Point[]} gran={granularity} />
            </Chart>
          ))}
        </div>

        <div className="text-center pb-6">
          <span data-testid="dashboard-realtime-badge"
            className="inline-flex items-center gap-2 text-xs text-[var(--text-secondary)] bg-[var(--glass-bg)] px-3 py-1.5 rounded-full"
          >
            <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" />
            数据实时更新
          </span>
        </div>
      </div>
    </AppLayout>
  );
}
