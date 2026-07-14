'use client';

import React, { useState, useEffect, useRef, useCallback } from 'react';

/** Default session idle timeout in seconds. Override with NEXT_PUBLIC_SESSION_IDLE_SECONDS. */
export const SESSION_IDLE_SECONDS =
  parseInt(process.env.NEXT_PUBLIC_SESSION_IDLE_SECONDS || '') || 600;

interface IdleTimeoutProps {
  /** Idle timeout in seconds before showing warning (default 600 = 10min) */
  idleSeconds?: number;
  /** Countdown duration in seconds before auto-logout (default 60) */
  countdownSeconds?: number;
  onLogout: () => void;
}

/**
 * IdleTimeout monitors user activity and shows a countdown warning modal
 * when the user has been idle for `idleSeconds`. If the user clicks "Continue",
 * the timer resets. If the countdown expires, it triggers logout.
 */
export default function IdleTimeout({
  idleSeconds = 600,
  countdownSeconds = 60,
  onLogout,
}: IdleTimeoutProps) {
  const [isIdle, setIsIdle] = useState(false);
  const [countdown, setCountdown] = useState(countdownSeconds);
  const idleTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const countdownTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const resetIdle = useCallback(() => {
    setIsIdle(false);
    setCountdown(countdownSeconds);

    if (idleTimerRef.current) clearTimeout(idleTimerRef.current);
    if (countdownTimerRef.current) clearInterval(countdownTimerRef.current);

    idleTimerRef.current = setTimeout(() => {
      setIsIdle(true);
      // Start countdown
      countdownTimerRef.current = setInterval(() => {
        setCountdown((prev) => {
          if (prev <= 1) {
            // Auto-logout
            if (countdownTimerRef.current) clearInterval(countdownTimerRef.current);
            onLogout();
            return 0;
          }
          return prev - 1;
        });
      }, 1000);
    }, idleSeconds * 1000);
  }, [idleSeconds, countdownSeconds, onLogout]);

  useEffect(() => {
    const events = ['mousemove', 'keydown', 'click', 'scroll', 'touchstart'];
    const handler = () => resetIdle();

    events.forEach((e) => window.addEventListener(e, handler, { passive: true }));
    resetIdle(); // Initial timer

    return () => {
      events.forEach((e) => window.removeEventListener(e, handler));
      if (idleTimerRef.current) clearTimeout(idleTimerRef.current);
      if (countdownTimerRef.current) clearInterval(countdownTimerRef.current);
    };
  }, [resetIdle]);

  if (!isIdle) return null;

  return (
    <div
      data-testid="session-timeout-warning"
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 9999,
        background: 'rgba(0, 0, 0, 0.6)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      <div
        className="glass"
        style={{
          padding: '32px',
          maxWidth: '400px',
          width: '90%',
          textAlign: 'center',
          borderRadius: '16px',
        }}
      >
        <h3 style={{ fontSize: '18px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '12px' }}>
          会话即将过期
        </h3>
        <p style={{ color: '#7A7A7A', fontSize: '14px', marginBottom: '20px' }}>
          您已 {idleSeconds} 秒未操作，{countdown} 秒后自动退出
        </p>
        <button
          data-testid="session-timeout-continue-btn"
          onClick={resetIdle}
          style={{
            width: '100%',
            padding: '12px',
            background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
            color: '#fff',
            border: 'none',
            borderRadius: '8px',
            fontSize: '14px',
            cursor: 'pointer',
          }}
        >
          继续使用
        </button>
      </div>
    </div>
  );
}
