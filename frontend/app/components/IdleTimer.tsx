'use client';

import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useAuth } from '@/lib/api';

const DEFAULT_IDLE_TIMEOUT = 1800; // 30 minutes
const DEFAULT_COUNTDOWN = 60;

export default function IdleTimer() {
  const { auth, logout } = useAuth();
  const [showWarning, setShowWarning] = useState(false);
  const [countdown, setCountdown] = useState(DEFAULT_COUNTDOWN);
  const idleTimerRef = useRef<NodeJS.Timeout | null>(null);
  const countdownRef = useRef<NodeJS.Timeout | null>(null);

  // Read timeout/countdown from URL param or window global (for E2E testing)
  const getIdleTimeout = useCallback(() => {
    if (typeof window !== 'undefined') {
      const w = window as any;
      if (w.__IDLE_TIMEOUT__) return w.__IDLE_TIMEOUT__;
      const params = new URLSearchParams(window.location.search);
      const e2eTimeout = params.get('idle_timeout');
      if (e2eTimeout) return parseInt(e2eTimeout, 10);
    }
    return DEFAULT_IDLE_TIMEOUT;
  }, []);

  const getCountdownSeconds = useCallback(() => {
    if (typeof window !== 'undefined') {
      const w = window as any;
      if (w.__COUNTDOWN__) return w.__COUNTDOWN__;
      const params = new URLSearchParams(window.location.search);
      const e2eCountdown = params.get('countdown');
      if (e2eCountdown) return parseInt(e2eCountdown, 10);
    }
    return DEFAULT_COUNTDOWN;
  }, []);

  const resetIdleTimer = useCallback(() => {
    if (idleTimerRef.current) clearTimeout(idleTimerRef.current);
    const timeout = getIdleTimeout() * 1000;
    const cd = getCountdownSeconds();
    idleTimerRef.current = setTimeout(() => {
      setShowWarning(true);
      setCountdown(cd);
    }, timeout);
  }, [getIdleTimeout, getCountdownSeconds]);

  const handleContinue = useCallback(() => {
    setShowWarning(false);
    setCountdown(getCountdownSeconds());
    if (countdownRef.current) clearInterval(countdownRef.current);
    resetIdleTimer();
  }, [resetIdleTimer, getCountdownSeconds]);

  const handleLogout = useCallback(() => {
    if (countdownRef.current) clearInterval(countdownRef.current);
    setShowWarning(false);
    logout();
    window.location.href = '/login?expired=true';
  }, [logout]);

  // Countdown effect
  useEffect(() => {
    if (!showWarning) return;
    countdownRef.current = setInterval(() => {
      setCountdown(prev => {
        if (prev <= 1) {
          handleLogout();
          return 0;
        }
        return prev - 1;
      });
    }, 1000);
    return () => { if (countdownRef.current) clearInterval(countdownRef.current); };
  }, [showWarning, handleLogout]);

  // Start idle timer + activity listeners
  useEffect(() => {
    if (!auth.hydrated || !auth.token) return;

    resetIdleTimer();
    const events = ['mousedown', 'mousemove', 'keydown', 'scroll', 'touchstart'];
    const handler = () => { if (!showWarning) resetIdleTimer(); };
    events.forEach(e => document.addEventListener(e, handler, { passive: true }));

    return () => {
      if (idleTimerRef.current) clearTimeout(idleTimerRef.current);
      if (countdownRef.current) clearInterval(countdownRef.current);
      events.forEach(e => document.removeEventListener(e, handler));
    };
  }, [auth.hydrated, auth.token, resetIdleTimer, showWarning]);

  if (!showWarning) return null;

  return (
    <div
      className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-sm"
      data-testid="session-timeout-warning"
    >
      <div className="glass p-8 rounded-2xl max-w-md w-full mx-4 text-center">
        <h3 className="text-lg font-bold text-[var(--text-primary)] mb-2">
          会话即将过期
        </h3>
        <p className="text-sm text-[var(--text-secondary)] mb-4">
          您已长时间未操作，会话将在 {countdown} 秒后过期。
        </p>
        <div className="flex gap-3 justify-center">
          <button
            className="px-6 py-2 bg-[var(--accent)] text-white rounded-xl font-medium hover:opacity-90 transition-all"
            onClick={handleContinue}
            data-testid="session-timeout-continue-btn"
          >
            继续使用
          </button>
          <button
            className="px-6 py-2 border border-[var(--border-glass)] text-[var(--text-secondary)] rounded-xl hover:bg-white/10 transition-all"
            onClick={handleLogout}
            data-testid="session-timeout-logout-btn"
          >
            退出登录
          </button>
        </div>
      </div>
    </div>
  );
}
