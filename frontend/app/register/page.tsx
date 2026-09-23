'use client';

import React, { useState, useEffect, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { getApiHost } from '../../lib/api-host';

function RegisterForm() {
  const searchParams = useSearchParams();
  const token = searchParams.get('token');
  const router = useRouter();
  const t = useTranslations('register');
  const tp = useTranslations('pwd');

  const [step, setStep] = useState<'loading' | 'invalid' | 'form'>('loading');
  const [prefillEmail, setPrefillEmail] = useState('');
  const [prefillRole, setPrefillRole] = useState('');
  const [errorMsg, setErrorMsg] = useState('');

  const [username, setUsername] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [usernameError, setUsernameError] = useState('');
  const [displayNameError, setDisplayNameError] = useState('');
  const [passwordError, setPasswordError] = useState('');
  const [confirmError, setConfirmError] = useState('');
  const [submitError, setSubmitError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  // Verify token on mount
  useEffect(() => {
    if (!token) {
      setStep('invalid');
      setErrorMsg(t('missingToken'));
      return;
    }

    getApiHost()
      .then((base) => fetch(`${base}/auth/register?token=${encodeURIComponent(token)}`))
      .then((res) => res.json())
      .then((data) => {
        if (data.valid) {
          setPrefillEmail(data.email || '');
          setPrefillRole(data.role || 'user');
          setStep('form');
        } else {
          setStep('invalid');
          setErrorMsg(t('invalidToken'));
        }
      })
      .catch(() => {
        setStep('invalid');
        setErrorMsg(t('verifyFailed'));
      });
  }, [token, t]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setUsernameError('');
    setDisplayNameError('');
    setPasswordError('');
    setConfirmError('');
    setSubmitError('');

    // Validate
    let valid = true;
    if (!username.trim() || username.trim().length < 2) {
      setUsernameError(t('usernameTooShort'));
      valid = false;
    }
    if (!displayName.trim()) {
      setDisplayNameError(t('displayNameRequired'));
      valid = false;
    }
    if (!password || password.length < 6) {
      setPasswordError(t('passwordTooShort'));
      valid = false;
    }
    if (password !== confirmPassword) {
      setConfirmError(tp('mismatch'));
      valid = false;
    }
    if (!valid) return;

    setSubmitting(true);
    try {
      const base = await getApiHost();
      const res = await fetch(`${base}/auth/complete-registration`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          token,
          username: username.trim(),
          password,
          display_name: displayName.trim(),
        }),
      });
      const data = await res.json();

      if (!res.ok) {
        setSubmitError(data.error || t('registerFailed'));
        return;
      }

      // Store token and redirect
      if (data.access_token) {
        localStorage.setItem('auth_token', data.access_token);
        localStorage.setItem('auth_user', JSON.stringify({
          userId: data.user_id,
          username: data.username,
          role: data.role,
        }));
      }
      router.push('/chat');
    } catch {
      setSubmitError(t('networkError'));
    } finally {
      setSubmitting(false);
    }
  };

  if (step === 'loading') {
    return (
      <div className="min-h-screen flex items-center justify-center bg-black" data-testid="register-loading">
        <p className="text-white/60">{t('verifying')}</p>
      </div>
    );
  }

  if (step === 'invalid') {
    return (
      <div className="min-h-screen flex items-center justify-center bg-black" data-testid="register-invalid">
        <div className="glass p-8 w-full max-w-md text-center" style={{ background: 'rgba(255,255,255,0.05)', border: '1px solid rgba(255,255,255,0.10)', borderRadius: '16px' }}>
          <div className="text-red-400 text-4xl mb-4" data-testid="register-invalid-icon">⚠</div>
          <h2 className="text-white text-lg font-semibold mb-2" data-testid="register-invalid-title">{t('invalidTitle')}</h2>
          <p className="text-white/60 text-sm mb-6" data-testid="register-invalid-msg">{errorMsg}</p>
          <a
            href="/login"
            className="inline-block px-6 py-2 rounded-xl text-sm font-medium"
            style={{ background: 'linear-gradient(135deg, #B1E2FF, #9381FF)', color: '#000' }}
            data-testid="register-goto-login-btn"
          >
            {t('backToLogin')}
          </a>
        </div>
      </div>
    );
  }

  // Registration form
  return (
    <div className="min-h-screen flex items-center justify-center bg-black" data-testid="register-form">
      <div className="glass p-8 w-full max-w-md" style={{ background: 'rgba(255,255,255,0.05)', border: '1px solid rgba(255,255,255,0.10)', borderRadius: '16px', width: '420px' }}>
        <div className="flex items-center justify-center gap-3 mb-6" data-testid="register-logo">
          <div className="flex items-center justify-center rounded-lg" style={{ width: '36px', height: '36px', background: 'linear-gradient(135deg, #B1E2FF, #9381FF)' }}>
            <span className="text-white text-sm font-bold">DA</span>
          </div>
          <span className="text-lg font-semibold text-white">DataAgent</span>
        </div>

        <h1 className="text-center mb-2 font-semibold text-white text-xl" data-testid="register-title">{t('title')}</h1>
        {prefillEmail && (
          <p className="text-center mb-6 text-sm text-white/50" data-testid="register-email-display">{t('invitedEmail', { email: prefillEmail })}</p>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div data-testid="register-username-field">
            <label className="block mb-1.5 font-semibold text-xs text-white/50">{t('username')}</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder={t('usernamePlaceholder')}
              className="w-full px-4 py-2.5 rounded-xl bg-white/5 border border-white/10 text-white placeholder-white/30 focus:outline-none focus:border-[#B1E2FF] transition-all"
              data-testid="register-username-input"
            />
            {usernameError && <p className="mt-1 text-sm text-red-400" data-testid="register-username-error">{usernameError}</p>}
          </div>

          <div data-testid="register-displayname-field">
            <label className="block mb-1.5 font-semibold text-xs text-white/50">{t('displayName')}</label>
            <input
              type="text"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder={t('displayNamePlaceholder')}
              className="w-full px-4 py-2.5 rounded-xl bg-white/5 border border-white/10 text-white placeholder-white/30 focus:outline-none focus:border-[#B1E2FF] transition-all"
              data-testid="register-displayname-input"
            />
            {displayNameError && <p className="mt-1 text-sm text-red-400" data-testid="register-displayname-error">{displayNameError}</p>}
          </div>

          <div data-testid="register-password-field">
            <label className="block mb-1.5 font-semibold text-xs text-white/50">{t('password')}</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={t('passwordPlaceholder')}
              className="w-full px-4 py-2.5 rounded-xl bg-white/5 border border-white/10 text-white placeholder-white/30 focus:outline-none focus:border-[#B1E2FF] transition-all"
              data-testid="register-password-input"
            />
            {passwordError && <p className="mt-1 text-sm text-red-400" data-testid="register-password-error">{passwordError}</p>}
          </div>

          <div data-testid="register-confirm-field">
            <label className="block mb-1.5 font-semibold text-xs text-white/50">{t('confirmPassword')}</label>
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder={t('confirmPlaceholder')}
              className="w-full px-4 py-2.5 rounded-xl bg-white/5 border border-white/10 text-white placeholder-white/30 focus:outline-none focus:border-[#B1E2FF] transition-all"
              data-testid="register-confirm-input"
            />
            {confirmError && <p className="mt-1 text-sm text-red-400" data-testid="register-confirm-error">{confirmError}</p>}
          </div>

          {submitError && (
            <div className="p-3 rounded-xl bg-red-500/10 border border-red-500/20 text-red-400 text-sm" data-testid="register-submit-error">
              {submitError}
            </div>
          )}

          <button
            type="submit"
            disabled={submitting}
            className="w-full py-3 rounded-xl font-bold transition-all mt-2"
            style={{ background: 'linear-gradient(135deg, #B1E2FF, #9381FF)', color: '#000', fontSize: '15px' }}
            data-testid="register-submit-btn"
          >
            {submitting ? t('submitting') : t('title')}
          </button>
        </form>

        <p className="text-center text-xs mt-6 text-white/40">
          {t('haveAccount')}<a href="/login" className="text-[#B1E2FF] ml-1" data-testid="register-login-link">{t('backToLogin')}</a>
        </p>
      </div>
    </div>
  );
}

export default function RegisterPage() {
  const tc = useTranslations('common');
  return (
    <Suspense fallback={
      <div className="min-h-screen flex items-center justify-center bg-black">
        <p className="text-white/60">{tc('loading')}</p>
      </div>
    }>
      <RegisterForm />
    </Suspense>
  );
}
