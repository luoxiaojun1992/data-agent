'use client';

import { useEffect, useState } from 'react';

// SPEC-079: 全局在线指示灯。挂在 RootLayout，所有页面（含登录/注册）统一
// 显示后端服务与依赖组件的真实健康状态，取代原先硬编码的绿色脉冲圆点。

type DepStatus = 'up' | 'down' | 'skipped';

interface Dependency {
  status: DepStatus;
  latency_ms?: number;
  error?: string;
}

interface HealthResponse {
  status: 'ok' | 'degraded';
  latency_ms?: number;
  dependencies?: Record<string, Dependency>;
}

type IndicatorState = 'ok' | 'degraded' | 'down';

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1';
const POLL_INTERVAL_MS = 15000;
const FETCH_TIMEOUT_MS = 3000;

// 渲染顺序：必探项在前、条件探项次之（对齐 SPEC-079 §5.1.1 探针清单）。
const DEP_ORDER = [
  'mongodb',
  'redis',
  'qdrant',
  'vault',
  'seaweedfs',
  'mysql',
  'arcadedb',
  'presidio',
];

const STATE_LABEL: Record<IndicatorState, string> = {
  ok: '在线',
  degraded: '服务降级',
  down: '服务离线',
};

const DOT_COLOR: Record<IndicatorState, string> = {
  ok: 'bg-emerald-400',
  degraded: 'bg-amber-400',
  down: 'bg-red-400',
};

const DEP_LABEL: Record<DepStatus, string> = {
  up: '在线',
  down: '离线',
  skipped: '未启用',
};

const DEP_COLOR: Record<DepStatus, string> = {
  up: 'text-emerald-400',
  down: 'text-red-400',
  skipped: 'text-gray-400',
};

export default function OnlineIndicator() {
  const [state, setState] = useState<IndicatorState>('down');
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setInterval> | undefined;

    const poll = async () => {
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);
      try {
        const res = await fetch(`${API_BASE}/health`, { signal: controller.signal });
        if (!res.ok) throw new Error(`http ${res.status}`);
        const data: HealthResponse = await res.json();
        if (cancelled) return;
        setHealth(data);
        setState(data.status === 'ok' ? 'ok' : 'degraded');
      } catch {
        if (cancelled) return;
        setHealth(null);
        setState('down');
      } finally {
        clearTimeout(timeout);
      }
    };

    poll();
    timer = setInterval(poll, POLL_INTERVAL_MS);

    return () => {
      cancelled = true;
      if (timer) clearInterval(timer);
    };
  }, []);

  const deps = health?.dependencies ?? {};
  const orderedDeps = DEP_ORDER.filter((name) => name in deps).map((name) => ({
    name,
    ...deps[name],
  }));

  return (
    <div className="fixed top-4 right-4 z-[60]" data-testid="global-online-indicator">
      <div
        className="relative flex items-center gap-2 rounded-full border border-[var(--border-glass)] bg-[var(--bg-secondary)] px-3 py-1.5 cursor-default"
        onMouseEnter={() => setOpen(true)}
        onMouseLeave={() => setOpen(false)}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        tabIndex={0}
      >
        <span
          className={`w-2 h-2 rounded-full ${DOT_COLOR[state]} ${state !== 'down' ? 'animate-pulse' : ''}`}
          data-testid="global-online-dot"
        />
        <span className="text-xs text-[var(--text-secondary)]">{STATE_LABEL[state]}</span>

        {open && (
          <div
            data-testid="global-online-tooltip"
            className="absolute top-full right-0 mt-2 z-[70] w-max min-w-[180px] rounded-xl border border-[var(--border-glass)] bg-[var(--bg-secondary)] px-4 py-3 shadow backdrop-blur"
          >
            {state === 'down' ? (
              <span className="text-xs text-[var(--text-secondary)]">后端服务不可达</span>
            ) : (
              <>
                {health?.latency_ms != null && (
                  <div
                    data-testid="tooltip-api-latency"
                    className="flex items-center justify-between gap-3 text-xs border-b border-[var(--border-glass)] pb-2 mb-2"
                  >
                    <span className="text-[var(--text-secondary)]">后端 API</span>
                    <span className="text-[var(--text-primary)]">{health.latency_ms}ms</span>
                  </div>
                )}
                {orderedDeps.map((d) => (
                  <div
                    key={d.name}
                    data-testid={`tooltip-dep-${d.name}`}
                    className="flex items-center justify-between gap-3 text-xs py-0.5"
                  >
                    <span className="text-[var(--text-secondary)]">{d.name}</span>
                    <span className="flex items-center gap-1.5">
                      <span className={DEP_COLOR[d.status]}>{DEP_LABEL[d.status]}</span>
                      {d.status === 'up' && d.latency_ms != null && (
                        <span className="text-[var(--text-secondary)]">{d.latency_ms}ms</span>
                      )}
                    </span>
                  </div>
                ))}
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
