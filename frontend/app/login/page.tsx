'use client';

import React, { useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useAuth } from '@/lib/api';

function isValidEmail(email: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
}

export default function LoginPage() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [emailError, setEmailError] = useState('');
  const [passwordError, setPasswordError] = useState('');
  const [generalError, setGeneralError] = useState('');
  const [loading, setLoading] = useState(false);
  const { login } = useAuth();
  const router = useRouter();
  const searchParams = useSearchParams();
  const sessionExpired = searchParams.get('expired') === 'true';

  const validateForm = (): boolean => {
    let valid = true;
    setEmailError('');
    setPasswordError('');

    if (!email.trim()) {
      setEmailError('请输入邮箱地址');
      valid = false;
    } else if (!isValidEmail(email)) {
      setEmailError('请输入有效的邮箱地址');
      valid = false;
    }

    if (!password.trim()) {
      setPasswordError('请输入密码');
      valid = false;
    }

    return valid;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setGeneralError('');

    if (!validateForm()) return;

    setLoading(true);
    try {
      await login(email, password);
      router.push('/');
    } catch (err: any) {
      setGeneralError('邮箱或密码错误');
      setPassword('');
    } finally {
      setLoading(false);
    }
  };

  const handleEmailBlur = () => {
    if (email && !isValidEmail(email)) {
      setEmailError('请输入有效的邮箱地址');
    } else {
      setEmailError('');
    }
  };

  return (
    <div
      className="min-h-screen flex items-center justify-center"
      style={{ backgroundColor: '#000000' }}
      data-testid="login-card"
    >
      {/* Error toast */}
      {sessionExpired && (
        <div
          className="fixed top-4 right-4 p-3 rounded-xl bg-red-500/10 border border-red-500/20 text-red-400 text-sm z-50"
          data-testid="login-session-expired-toast"
        >
          登录已过期，请重新登录
        </div>
      )}

      {generalError && (
        <div
          className="fixed top-4 right-4 p-3 rounded-xl bg-red-500/10 border border-red-500/20 text-red-400 text-sm z-50"
          data-testid="login-error-toast"
        >
          {generalError}
        </div>
      )}

      {/* Aurora glow backdrop */}
      <div className="fixed inset-0 pointer-events-none">
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[600px] rounded-full bg-[var(--accent-glow)] opacity-10 blur-3xl" />
      </div>

      <div
        className="glass p-8 w-full max-w-md animate-fade-in"
        style={{
          background: 'rgba(255,255,255,0.05)',
          border: '1px solid rgba(255,255,255,0.10)',
          borderRadius: '16px',
          width: '420px',
        }}
      >
        {/* Logo + Brand */}
        <div className="flex items-center justify-center gap-3 mb-6" data-testid="login-logo">
          <div
            className="flex items-center justify-center rounded-lg"
            style={{
              width: '36px',
              height: '36px',
              background: 'linear-gradient(135deg, #B1E2FF, #9381FF)',
            }}
            data-testid="login-logo-icon"
          >
            <span className="text-white text-sm font-bold">DA</span>
          </div>
          <span
            className="text-lg font-semibold"
            style={{ color: '#FFFFFF', fontSize: '18px' }}
            data-testid="login-logo-name"
          >
            DataAgent
          </span>
        </div>

        <h1
          className="text-center mb-6 font-semibold"
          style={{ color: '#FFFFFF', fontSize: '20px' }}
          data-testid="login-title"
        >
          登录企业数据分析平台
        </h1>

        <form onSubmit={handleSubmit} className="space-y-5">
          {/* Email field */}
          <div>
            <label
              className="block mb-1.5 font-semibold"
              style={{ fontSize: '12px', color: '#7A7A7A' }}
              data-testid="login-email-label"
            >
              邮箱地址
            </label>
            <input
              type="text"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              onBlur={handleEmailBlur}
              placeholder="name@company.com"
              className="w-full px-4 py-2.5 rounded-xl bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] placeholder-[var(--text-secondary)] focus:outline-none focus:border-[#B1E2FF] focus:ring-1 focus:ring-[#B1E2FF] transition-all"
              data-testid="login-email-input"
            />
            {emailError && (
              <p className="mt-1 text-sm text-red-400" data-testid="login-email-error">
                {emailError}
              </p>
            )}
          </div>

          {/* Password field */}
          <div>
            <label
              className="block mb-1.5 font-semibold"
              style={{ fontSize: '12px', color: '#7A7A7A' }}
              data-testid="login-password-label"
            >
              密码
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="输入密码"
              className="w-full px-4 py-2.5 rounded-xl bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] placeholder-[var(--text-secondary)] focus:outline-none focus:border-[var(--accent)] focus:ring-1 focus:ring-[var(--accent)] transition-all"
              data-testid="login-password-input"
            />
            {passwordError && (
              <p className="mt-1 text-sm text-red-400" data-testid="login-password-error">
                {passwordError}
              </p>
            )}
          </div>

          {/* Login button */}
          <button
            type="submit"
            disabled={loading}
            className="w-full py-3 rounded-xl font-bold transition-all"
            style={{
              background: 'linear-gradient(135deg, #B1E2FF, #9381FF)',
              color: '#000000',
              fontSize: '15px',
            }}
            onMouseOver={(e) => {
              e.currentTarget.style.opacity = '0.9';
              e.currentTarget.style.transform = 'translateY(-1px)';
            }}
            onMouseOut={(e) => {
              e.currentTarget.style.opacity = '1';
              e.currentTarget.style.transform = 'translateY(0)';
            }}
            data-testid="login-btn"
          >
            {loading ? (
              <span className="flex items-center justify-center gap-2">
                <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
                登录中...
              </span>
            ) : (
              '登录'
            )}
          </button>

          {/* SSO Divider */}
          <div className="flex items-center gap-3" data-testid="login-divider">
            <div className="flex-1 h-px" style={{ backgroundColor: 'rgba(255,255,255,0.10)' }} />
            <span className="text-sm" style={{ color: '#7A7A7A' }}>或</span>
            <div className="flex-1 h-px" style={{ backgroundColor: 'rgba(255,255,255,0.10)' }} />
          </div>

          {/* SSO Button */}
          <button
            type="button"
            className="w-full py-2.5 rounded-xl border font-medium transition-all"
            style={{
              background: 'transparent',
              borderColor: 'rgba(255,255,255,0.15)',
              color: '#7A7A7A',
              fontSize: '14px',
            }}
            onMouseOver={(e) => {
              e.currentTarget.style.background = 'rgba(255,255,255,0.05)';
              e.currentTarget.style.color = '#FFFFFF';
            }}
            onMouseOut={(e) => {
              e.currentTarget.style.background = 'transparent';
              e.currentTarget.style.color = '#7A7A7A';
            }}
            data-testid="login-sso-btn"
          >
            企业 SSO 单点登录
          </button>
        </form>

        <p className="text-center text-xs mt-6" style={{ color: '#7A7A7A' }}>
          首次使用？请使用管理员账号登录
        </p>
      </div>
    </div>
  );
}
