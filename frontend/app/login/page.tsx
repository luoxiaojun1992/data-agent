'use client';

import React, { useState, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useAuth } from '@/lib/api';

function isValidEmail(email: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
}

function LoginForm() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [emailError, setEmailError] = useState('');
  const [passwordError, setPasswordError] = useState('');
  const [generalError, setGeneralError] = useState('');
  const [loading, setLoading] = useState(false);
  const { login } = useAuth();
  const t = useTranslations('auth');
  const router = useRouter();
  const searchParams = useSearchParams();
  const sessionExpired = searchParams.get('expired') === 'true';
  const pwdChanged = searchParams.get('pwd_changed') === 'true';

  const validateForm = (): boolean => {
    let valid = true;
    setEmailError('');
    setPasswordError('');

    if (!email.trim()) {
      setEmailError(t('emailRequired'));
      valid = false;
    } else if (!isValidEmail(email)) {
      setEmailError(t('emailInvalid'));
      valid = false;
    }

    if (!password.trim()) {
      setPasswordError(t('passwordRequired'));
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
      setGeneralError(t('invalidCredentials'));
      setPassword('');
    } finally {
      setLoading(false);
    }
  };

  const handleEmailBlur = () => {
    if (email && !isValidEmail(email)) {
      setEmailError(t('emailInvalid'));
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
      {/* Toast stack (SPEC-079): 两个 toast 纵向堆叠、下移 top-14 避开右上角在线指示灯 */}
      <div
        className="fixed top-14 right-4 z-50 flex flex-col gap-2 items-end"
        data-testid="login-toast-stack"
      >
        {sessionExpired && (
          <div
            className="p-3 rounded-xl bg-red-500/10 border border-red-500/20 text-red-400 text-sm"
            data-testid="login-session-expired-toast"
          >
            {t('sessionExpired')}
          </div>
        )}

        {pwdChanged && (
          <div
            className="p-3 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 text-sm"
            data-testid="login-pwd-changed-toast"
          >
            {t('pwdChanged')}
          </div>
        )}

        {generalError && (
          <div
            className="p-3 rounded-xl bg-red-500/10 border border-red-500/20 text-red-400 text-sm"
            data-testid="login-error-toast"
          >
            {generalError}
          </div>
        )}
      </div>

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
          {t('title')}
        </h1>

        <form onSubmit={handleSubmit} className="space-y-5">
          {/* Email field */}
          <div>
            <label
              className="block mb-1.5 font-semibold"
              style={{ fontSize: '12px', color: '#7A7A7A' }}
              data-testid="login-email-label"
            >
              {t('email')}
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
              {t('password')}
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={t('passwordPlaceholder')}
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
                {t('loggingIn')}
              </span>
            ) : (
              t('login')
            )}
          </button>

          {/* SSO Divider */}
          <div className="flex items-center gap-3" data-testid="login-divider">
            <div className="flex-1 h-px" style={{ backgroundColor: 'rgba(255,255,255,0.10)' }} />
            <span className="text-sm" style={{ color: '#7A7A7A' }}>{t('or')}</span>
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
            {t('sso')}
          </button>
        </form>

        <p className="text-center text-xs mt-6" style={{ color: '#7A7A7A' }}>
          {t('firstTimeHint')}
        </p>
      </div>
    </div>
  );
}

export default function LoginPage() {
  const tc = useTranslations('common');
  return (
    <Suspense fallback={<div className="min-h-screen flex items-center justify-center bg-black"><p className="text-white">{tc('loading')}</p></div>}>
      <LoginForm />
    </Suspense>
  );
}
