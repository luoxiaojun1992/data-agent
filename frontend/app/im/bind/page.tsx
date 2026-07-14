'use client';

import React, { useState, useEffect } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';

export default function ImBindPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const token = searchParams?.get('token') || '';

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [status, setStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle');
  const [errorMsg, setErrorMsg] = useState('');
  const [expired, setExpired] = useState(false);

  // Check token validity on mount
  useEffect(() => {
    if (!token) {
      setExpired(true);
      return;
    }
    const checkToken = async () => {
      try {
        const res = await fetch(`/api/v1/im/bind/check/${token}`);
        if (!res.ok) {
          const d = await res.json();
          if (res.status === 410 || d.error?.includes('过期')) {
            setExpired(true);
          }
        }
      } catch { /* if API not available, let user try binding */ }
    };
    checkToken();
  }, [token]);

  if (expired) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-[#0f172a]">
        <div className="text-center p-8" data-testid="im-bind-expired">
          <p className="text-[#94a3b8] text-lg">绑定链接已过期</p>
          <p className="text-[#64748b] text-sm mt-2">请在飞书中重新发送 /帮助 获取新链接</p>
        </div>
      </div>
    );
  }

  const handleSubmit = async () => {
    if (!email.trim() || !password || !token) return;
    setStatus('loading');
    try {
      const res = await fetch('/api/v1/im/bind/confirm', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token, email, password }),
      });
      if (res.ok) {
        setStatus('success');
        setTimeout(() => setStatus('idle'), 5000);
      } else {
        const d = await res.json();
        setErrorMsg(d.error || '绑定失败');
        setStatus('error');
      }
    } catch {
      setErrorMsg('网络错误，请重试');
      setStatus('error');
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-[#0f172a]">
      <div className="w-full max-w-sm p-8" data-testid="im-bind-page">
        <div className="text-center mb-8">
          <h1 className="text-2xl font-bold text-[var(--text-primary)]">DataAgent</h1>
          <p className="text-[var(--text-secondary)] text-sm mt-1">飞书账号绑定</p>
        </div>

        {status === 'success' ? (
          <div className="text-center p-6 rounded-xl" style={{ background: 'rgba(16,185,129,0.1)' }}>
            <p className="text-[#10b981] text-lg font-semibold">绑定成功</p>
            <p className="text-[var(--text-secondary)] text-sm mt-2">请返回飞书继续使用</p>
          </div>
        ) : (
          <div className="space-y-4">
            <div>
              <label className="block text-[var(--text-secondary)] text-xs mb-1">邮箱</label>
              <input
                data-testid="im-bind-email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="请输入系统账号邮箱"
                className="w-full px-4 py-2.5 rounded-lg bg-transparent border border-[var(--border-glass)] text-[var(--text-primary)] placeholder-[var(--text-secondary)] text-sm focus:outline-none focus:border-[#5c7cfa]"
              />
            </div>
            <div>
              <label className="block text-[var(--text-secondary)] text-xs mb-1">密码</label>
              <input
                data-testid="im-bind-password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="请输入密码"
                className="w-full px-4 py-2.5 rounded-lg bg-transparent border border-[var(--border-glass)] text-[var(--text-primary)] placeholder-[var(--text-secondary)] text-sm focus:outline-none focus:border-[#5c7cfa]"
              />
            </div>
            {status === 'error' && (
              <p className="text-[#ef4444] text-xs">{errorMsg}</p>
            )}
            <button
              data-testid="im-bind-submit"
              onClick={handleSubmit}
              disabled={status === 'loading'}
              className="w-full py-2.5 rounded-lg text-sm font-semibold text-white transition-opacity"
              style={{ background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)' }}
            >
              {status === 'loading' ? '绑定中...' : '绑定'}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
