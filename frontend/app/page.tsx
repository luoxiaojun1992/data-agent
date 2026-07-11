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

function SimpleBar({ data }: { data: { label: string; value: number; color?: string }[] }) {
  const max = Math.max(...data.map(d => d.value), 1);
  return (
    <div className="flex items-end gap-2" style={{ height: '100px' }}>
      {data.map((d, i) => (
        <div key={i} className="flex-1 flex flex-col items-center gap-1">
          <div className="w-full rounded-t" style={{
            height: `${Math.max(8, (d.value / max) * 80)}px`,
            backgroundColor: d.color || 'var(--accent)',
            opacity: 0.7,
          }} />
          <span className="text-[9px] text-[var(--text-secondary)]">{d.label}</span>
        </div>
      ))}
    </div>
  );
}

function SimpleLine({ data }: { data: { label: string; value: number }[] }) {
  const max = Math.max(...data.map(d => d.value), 1), min = 0;
  const w = 100 / (data.length - 1 || 1);
  const points = data.map((d, i) => `${i * w},${100 - ((d.value - min) / (max - min || 1)) * 90}`).join(' ');
  return (
    <svg viewBox="0 0 100 100" className="w-full" style={{ height: '80px' }}>
      <polyline points={points} fill="none" stroke="var(--accent)" strokeWidth="2" />
      {data.map((d, i) => (
        <circle key={i} cx={i * w} cy={100 - ((d.value - min) / (max - min || 1)) * 90} r="1.5" fill="var(--accent)" />
      ))}
    </svg>
  );
}

export default function MainPage() {
  const { apiFetch } = useAuth();
  const [timeFilter, setTimeFilter] = useState('today');
  const [stats, setStats] = useState<any>(null);

  useEffect(() => {
    (async () => {
      try {
        const res = await apiFetch('/dashboard');
        setStats(await res.json());
      } catch { /* ignore */ }
    })();
  }, []);

  const kpis = stats?.kpis || [
    { label: '活跃 Chat 会话', value: '—', icon: '💬', trend: '—' },
    { label: 'Agent 任务', value: '—', icon: '⚡', trend: '—' },
    { label: '知识库文档', value: '—', icon: '📚', trend: '—' },
    { label: '系统可用率', value: '99.9%', icon: '🟢', trend: '稳定' },
  ];

  // Status distribution from real task data
  const statusDist = [
    { label: '完成', value: taskStats.completed, color: '#34D399' },
    { label: '运行', value: taskStats.running, color: '#60A5FA' },
    { label: '失败', value: taskStats.failed, color: '#F87171' },
    { label: '等待', value: taskStats.pending, color: '#FBBF24' },
  // Status distribution from real task data
  const statusDist = [
    { label: '完成', value: taskStats.completed, color: '#34D399' },
    { label: '运行', value: taskStats.running, color: '#60A5FA' },
    { label: '失败', value: taskStats.failed, color: '#F87171' },
    { label: '等待', value: taskStats.pending, color: '#FBBF24' },
  ];

  // Time-series charts placeholder — needs /api/v1/dashboard/trends endpoint
  const emptyChart = (title: string) => (
    <div className="flex items-center justify-center py-8 text-xs text-[var(--text-secondary)]">
      📊 {title}（时间序列数据端点待建）
    </div>
  );

    </div>
  );

  return (
    <AppLayout>
      <div className="animate-fade-in">
        {/* Greeting + Time Filter */}
        <div className="mb-6 flex items-center justify-between">
          <div>
            <p className="text-lg font-semibold text-[var(--text-primary)]" data-testid="dashboard-greeting">
              {getGreeting()}，欢迎回来 👋
            </p>
            <p className="text-xs text-[var(--text-secondary)] mt-1" data-testid="dashboard-date">
              {new Date().toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric', weekday: 'long' })}
            </p>
          </div>
          <div className="flex items-center gap-2" data-testid="dashboard-time-filter">
            {['today', 'week', 'month'].map(f => (
              <button key={f} onClick={() => setTimeFilter(f)}
                data-testid={`filter-${f}`}
                className={`px-3 py-1 text-xs rounded-full transition-colors ${
                  timeFilter === f ? 'bg-[var(--accent)]/20 text-[var(--accent)]' : 'text-[var(--text-secondary)]'
                }`}
              >{f === 'today' ? '今日' : f === 'week' ? '本周' : '本月'}</button>
            ))}
          </div>
        </div>

        {/* Stats cards */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
          {stats.map((stat, i) => (
            <div key={stat.label} className="glass p-5 glass-hover" data-testid={`dashboard-stat-${i}`}>
              <div className="flex items-center justify-between mb-3">
                <span className="text-2xl">{stat.icon}</span>
                <span className="text-xs text-emerald-400 bg-emerald-400/10 px-2 py-0.5 rounded-full">{stat.trend}</span>
              </div>
              <p className="text-2xl font-bold text-[var(--text-primary)]">{stat.value}</p>
              <p className="text-sm text-[var(--text-secondary)] mt-1">{stat.label}</p>
            </div>
          ))}
        </div>

        {/* Charts Row 1 */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 mb-6">
          <Chart testid="chart-call-trend" title="Agent 调用量趋势">
            {emptyChart('24h 调用量')}
          </Chart>
          <Chart testid="chart-status-pie" title="任务状态分布">
            <SimpleBar data={statusDist} />
          </Chart>
        </div>

        {/* Charts Row 2 */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 mb-6">
          <Chart testid="chart-duration-dist" title="任务耗时分布">
            {emptyChart('耗时分布')}
          </Chart>
          <Chart testid="chart-req-dist" title="24h 请求量分布">
            {emptyChart('请求分布')}
          </Chart>
        </div>

        {/* Charts Row 3 */}
        <div className="mb-6">
          <Chart testid="chart-success-trend" title="成功率趋势">
            {emptyChart('成功率')}
          </Chart>
        </div>

        {/* Token / ROI Row */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
          {['Token 消耗', 'AI 产出', 'ROI'].map((label, i) => (
            <div key={label} className="glass p-5" data-testid={`dashboard-token-kpi-${i}`}>
              <p className="text-xs text-[var(--text-secondary)] uppercase mb-1">{label}</p>
              <span className="text-2xl font-bold text-[var(--text-primary)]" data-testid={`dashboard-token-value-${i}`}>—</span>
              <p className="text-xs text-[var(--text-secondary)] mt-1">时间序列端点待建</p>
            </div>
          ))}
        </div>

        {/* Charts Row 4 */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 mb-6">
          <Chart testid="chart-token-trend" title="Token 消耗趋势">
            {emptyChart('Token 消耗')}
          </Chart>
          <Chart testid="chart-output-stats" title="产出统计">
            {emptyChart('产出统计')}
          </Chart>
          <Chart testid="chart-roi-dual" title="AI Agent ROI">
            {emptyChart('ROI 分析')}
          </Chart>
        </div>

        {/* Real-time badge */}
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
