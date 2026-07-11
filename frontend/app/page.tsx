'use client';

import React, { useState } from 'react';
import AppLayout from './providers';

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
  const [timeFilter, setTimeFilter] = useState('today');

  const stats = [
    { label: '活跃 Chat 会话', value: '12', icon: '💬', trend: '+3' },
    { label: 'Agent 任务', value: '45', icon: '⚡', trend: '+8' },
    { label: '知识库文档', value: '128', icon: '📚', trend: '+12' },
    { label: '系统可用率', value: '99.9%', icon: '🟢', trend: '稳定' },
  ];

  const callTrend = [
    { label: '00', value: 3 }, { label: '04', value: 2 }, { label: '08', value: 8 },
    { label: '12', value: 15 }, { label: '16', value: 22 }, { label: '20', value: 12 },
  ];

  const statusDist = [
    { label: '完成', value: 35, color: '#34D399' }, { label: '运行', value: 5, color: '#60A5FA' },
    { label: '失败', value: 3, color: '#F87171' }, { label: '等待', value: 2, color: '#FBBF24' },
  ];

  const durationDist = [
    { label: '<5s', value: 20 }, { label: '<30s', value: 12 }, { label: '<1m', value: 6 },
    { label: '<5m', value: 5 }, { label: '>5m', value: 2 },
  ];

  const reqDist = [
    { label: '0时', value: 2 }, { label: '4时', value: 1 }, { label: '8时', value: 10 },
    { label: '12时', value: 18 }, { label: '16时', value: 22 }, { label: '20时', value: 8 },
  ];

  const successTrend = [
    { label: 'Mon', value: 98 }, { label: 'Tue', value: 97 }, { label: 'Wed', value: 99 },
    { label: 'Thu', value: 96 }, { label: 'Fri', value: 99 }, { label: 'Sat', value: 100 }, { label: 'Sun', value: 100 },
  ];

  const tokenKpis = [
    { label: 'Token 消耗', value: '1.2M', trend: '+15%' },
    { label: 'AI 产出', value: '420个', trend: '+22%' },
    { label: 'ROI', value: '3.5x', trend: '↑0.3' },
  ];

  const tokenTrend = [
    { label: 'Mon', value: 150, color: '#60A5FA' }, { label: 'Tue', value: 180, color: '#60A5FA' },
    { label: 'Wed', value: 220, color: '#60A5FA' }, { label: 'Thu', value: 190, color: '#60A5FA' },
    { label: 'Fri', value: 250, color: '#60A5FA' },
  ];

  const outputStats = [
    { label: '报告', value: 15, color: '#34D399' }, { label: 'SQL', value: 28, color: '#60A5FA' },
    { label: '图表', value: 12, color: '#F472B6' },
  ];

  const roiTrend = [
    { label: 'W1', value: 2.1 }, { label: 'W2', value: 2.8 }, { label: 'W3', value: 3.2 }, { label: 'W4', value: 3.5 },
  ];

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
            <SimpleLine data={callTrend} />
          </Chart>
          <Chart testid="chart-status-pie" title="任务状态分布">
            <div className="flex items-end gap-2" style={{ height: '80px' }}>
              {statusDist.map((d, i) => (
                <div key={i} className="flex-1 flex flex-col items-center gap-1">
                  <div className="w-full rounded-t" style={{ height: `${(d.value / 35) * 70}px`, backgroundColor: d.color, opacity: 0.8 }} />
                  <span className="text-[8px] text-[var(--text-secondary)]">{d.label}</span>
                </div>
              ))}
            </div>
          </Chart>
        </div>

        {/* Charts Row 2 */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 mb-6">
          <Chart testid="chart-duration-dist" title="任务耗时分布">
            <SimpleBar data={durationDist} />
          </Chart>
          <Chart testid="chart-req-dist" title="24h 请求量分布">
            <SimpleLine data={reqDist} />
          </Chart>
        </div>

        {/* Charts Row 3 */}
        <div className="mb-6">
          <Chart testid="chart-success-trend" title="成功率趋势">
            <div className="overflow-hidden" style={{ height: '80px' }}>
              <svg viewBox="0 0 100 100" className="w-full" style={{ height: '100%' }}>
                <defs>
                  <linearGradient id="successGrad" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="#34D399" stopOpacity="0.3" />
                    <stop offset="100%" stopColor="#34D399" stopOpacity="0" />
                  </linearGradient>
                </defs>
                {(() => {
                  const max = 100; const pts = successTrend.map((d, i) => `${(i / 6) * 100},${100 - ((d.value - 90) / 10) * 90}`).join(' ');
                  return (
                    <>
                      <polygon points={`0,100 ${pts.replace(/,/g, ' ').split(' ').filter((_,i) => i%2===0).map((x, i) => `${x},${pts.split(' ')[i*2+1]}`).join(' ')} 100,100`} fill="url(#successGrad)" />
                      <polyline points={pts} fill="none" stroke="#34D399" strokeWidth="1.5" />
                    </>
                  );
                })()}
              </svg>
            </div>
          </Chart>
        </div>

        {/* Token / ROI Row */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
          {tokenKpis.map((kpi, i) => (
            <div key={kpi.label} className="glass p-5" data-testid={`dashboard-token-kpi-${i}`}>
              <p className="text-xs text-[var(--text-secondary)] uppercase mb-1">{kpi.label}</p>
              <div className="flex items-baseline justify-between">
                <span className="text-2xl font-bold text-[var(--text-primary)]" data-testid={`dashboard-token-value-${i}`}>{kpi.value}</span>
                <span className="text-xs text-emerald-400">{kpi.trend}</span>
              </div>
            </div>
          ))}
        </div>

        {/* Charts Row 4 */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 mb-6">
          <Chart testid="chart-token-trend" title="Token 消耗趋势">
            <SimpleBar data={tokenTrend} />
          </Chart>
          <Chart testid="chart-output-stats" title="产出统计">
            <SimpleBar data={outputStats} />
          </Chart>
          <Chart testid="chart-roi-dual" title="AI Agent ROI">
            <SimpleLine data={roiTrend.map(d => ({ label: d.label, value: d.value * 20 }))} />
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
