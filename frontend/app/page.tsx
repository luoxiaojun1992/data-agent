'use client';

import React, { useState, useEffect } from 'react';
import { useTranslations, useLocale } from 'next-intl';
import AppLayout from './providers';
import { useAuth } from '@/lib/api';

function getGreetingKey() {
  const h = new Date().getHours();
  if (h < 12) return 'morning';
  if (h < 18) return 'afternoon';
  return 'evening';
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
  const tc = useTranslations('common');
  const hasData = data.some(d => d.value > 0);
  if (!hasData) {
    return (
      <div className="flex items-center justify-center h-[100px] text-xs text-[var(--text-secondary)]">
        {tc('noData')}
      </div>
    );
  }
  const max = Math.max(...data.map(d => d.value), 1);
  // 标签按桶数自动降采样，最多约 12 个，避免拥挤。
  const labelEvery = Math.max(1, Math.ceil(data.length / 12));
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
      {/* 标签行：斜向 45°（向右上角），锚定各桶中心，不重叠不截断 */}
      <div className="relative mt-2" style={{ height: '32px' }}>
        {data.map((d, i) => (
          <span
            key={i}
            className="absolute bottom-0 text-[8px] leading-none text-[var(--text-secondary)] whitespace-nowrap"
            style={{
              left: `${((i + 0.5) * 100) / data.length}%`,
              transform: 'rotate(-45deg)',
              transformOrigin: 'bottom left',
            }}
          >
            {i % labelEvery === 0 ? formatLabel(d.time, gran) : ''}
          </span>
        ))}
      </div>
    </div>
  );
}

const GRANS = [
  { key: 'day', labelKey: 'day' },
  { key: 'week', labelKey: 'week' },
  { key: 'month', labelKey: 'month' },
  { key: 'year', labelKey: 'year' },
];

export default function MainPage() {
  const { apiFetch, auth } = useAuth();
  const t = useTranslations('dashboard');
  const tn = useTranslations('nav');
  const locale = useLocale();
  const [granularity, setGranularity] = useState('day');
  const [summary, setSummary] = useState<any>(null);
  const [trends, setTrends] = useState<any>(null);
  // SPEC-100: 传递浏览器时区，后端按调用方时区做日历分桶。
  const timezone = typeof Intl !== 'undefined'
    ? Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai'
    : 'Asia/Shanghai';

  useEffect(() => {
    if (!auth.token) return;
    (async () => {
      try {
        const sr = await apiFetch(`/dashboard?granularity=${granularity}&timezone=${encodeURIComponent(timezone)}`);
        setSummary(await sr.json());
      } catch { /* ignore */ }
    })();
  }, [auth.token, granularity, timezone]);

  useEffect(() => {
    if (!auth.token) return;
    (async () => {
      try {
        const tr = await apiFetch(`/dashboard/trends?granularity=${granularity}&timezone=${encodeURIComponent(timezone)}`);
        setTrends(await tr.json());
      } catch { /* ignore */ }
    })();
  }, [auth.token, granularity, timezone]);

  const num = (v: any) => (typeof v === 'number' ? v : 0);

  const kpis = [
    { labelKey: 'kbDocs', value: num(summary?.kb_docs), icon: '📚', testid: 'dashboard-stat-kb' },
    { labelKey: 'tokenUsage', value: num(summary?.token_tokens), icon: '🪙', testid: 'dashboard-stat-token' },
    { labelKey: 'llmCalls', value: num(summary?.llm_calls), icon: '🤖', testid: 'dashboard-stat-llm' },
    { labelKey: 'apiCalls', value: num(summary?.api_calls), icon: '🔌', testid: 'dashboard-stat-api' },
    { labelKey: 'artifacts', value: num(summary?.artifact_created), icon: '📦', testid: 'dashboard-stat-artifact' },
    { labelKey: 'tasksDone', value: num(summary?.task_completed), icon: '✅', testid: 'dashboard-stat-task' },
    { labelKey: 'roi', value: num(summary?.roi).toFixed(2), icon: '📈', testid: 'dashboard-stat-roi' },
  ];

  const series: { key: string; titleKey: string; testid: string }[] = [
    { key: 'token_tokens', titleKey: 'tokenTrend', testid: 'chart-token' },
    { key: 'llm_calls', titleKey: 'llmTrend', testid: 'chart-llm' },
    { key: 'api_calls', titleKey: 'apiTrend', testid: 'chart-api' },
    { key: 'artifact_created', titleKey: 'artifactTrend', testid: 'chart-artifact' },
    { key: 'task_completed', titleKey: 'taskTrend', testid: 'chart-task' },
    { key: 'roi', titleKey: 'roiTrend', testid: 'chart-roi' },
  ];

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-6 flex items-center justify-between" data-testid="page-header">
          <div>
            <p className="text-lg font-semibold text-[var(--text-primary)]" data-testid="page-title">{tn('dashboard')}</p>
            <p className="text-xs text-[var(--text-secondary)] mt-1" data-testid="dashboard-greeting">{t(getGreetingKey())}{t('welcome')}</p>
            <p className="text-xs text-[var(--text-secondary)] mt-1" data-testid="dashboard-date">
              {new Date().toLocaleDateString(locale === 'en' ? 'en-US' : 'zh-CN', { year: 'numeric', month: 'long', day: 'numeric', weekday: 'long' })}
            </p>
          </div>
          <div className="flex items-center gap-2" data-testid="dashboard-time-filter">
            {GRANS.map(g => (
              <button key={g.key} onClick={() => setGranularity(g.key)}
                data-testid={`filter-${g.key}`}
                className={`px-3 py-1 text-xs rounded-full transition-colors ${
                  granularity === g.key ? 'bg-[var(--accent)]/20 text-[var(--accent)]' : 'text-[var(--text-secondary)]'
                }`}
              >{t(g.labelKey)}</button>
            ))}
          </div>
        </div>

        {/* KPI cards */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
          {kpis.map((kpi) => (
            <div key={kpi.testid} className="glass p-5 glass-hover" data-testid={kpi.testid}>
              <div className="flex items-center justify-between mb-3">
                <span className="text-2xl">{kpi.icon}</span>
              </div>
              <p className="text-2xl font-bold text-[var(--text-primary)]">{kpi.value}</p>
              <p className="text-sm text-[var(--text-secondary)] mt-1">{t(kpi.labelKey)}</p>
            </div>
          ))}
        </div>

        {/* Trend charts */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 mb-6">
          {series.map(s => (
            <Chart key={s.key} testid={s.testid} title={t(s.titleKey)}>
              <TrendChart data={(trends?.[s.key] || []) as Point[]} gran={granularity} />
            </Chart>
          ))}
        </div>

        <div className="text-center pb-6">
          {/* 在线状态统一由全局 OnlineIndicator 组件显示（SPEC-079），此处硬编码徽章已移除 */}
        </div>
      </div>
    </AppLayout>
  );
}
