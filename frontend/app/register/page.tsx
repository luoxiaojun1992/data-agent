'use client';

import React, { useState, useEffect, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';

const API_BASE = process.env.NEXT_PUBLIC_API_URL || '';

function RegisterForm() {
  const searchParams = useSearchParams();
  const token = searchParams.get('token');
  const router = useRouter();

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
      setErrorMsg('缺少邀请链接参数。请使用管理员发送的邀请链接访问。');
      return;
    }

    fetch(`${API_BASE}/api/v1/auth/register?token=${encodeURIComponent(token)}`)
      .then((res) => res.json())
      .then((data) => {
        if (data.valid) {
          setPrefillEmail(data.email || '');
          setPrefillRole(data.role || 'user');
          setStep('form');
        } else {
          setStep('invalid');
          setErrorMsg('邀请链接无效、已过期或已被使用。请联系管理员获取新的邀请链接。');
        }
      })
      .catch(() => {
        setStep('invalid');
        setErrorMsg('无法验证邀请链接，请稍后重试。');
      });
  }, [token]);

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
      setUsernameError('用户名至少 2 个字符');
      valid = false;
    }
    if (!displayName.trim()) {
      setDisplayNameError('请输入显示名称');
      valid = false;
    }
    if (!password || password.length < 6) {
      setPasswordError('密码至少 6 个字符');
      valid = false;
    }
    if (password !== confirmPassword) {
      setConfirmError('两次输入的密码不一致');
      valid = false;
    }
    if (!valid) return;

    setSubmitting(true);
    try {
      const res = await fetch(`${API_BASE}/api/v1/auth/complete-registration`, {
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
        setSubmitError(data.error || '注册失败，请重试');
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
      setSubmitError('网络错误，请稍后重试');
    } finally {
      setSubmitting(false);
    }
  };

  if (step === 'loading') {
    return (
      <div className="min-h-screen flex items-center justify-center bg-black" data-testid="register-loading">
        <p className="text-white/60">验证邀请链接中...</p>
      </div>
    );
  }

  if (step === 'invalid') {
    return (
      <div className="min-h-screen flex items-center justify-center bg-black" data-testid="register-invalid">
        <div className="glass p-8 w-full max-w-md text-center" style={{ background: 'rgba(255,255,255,0.05)', border: '1px solid rgba(255,255,255,0.10)', borderRadius: '16px' }}>
          <div className="text-red-400 text-4xl mb-4" data-testid="register-invalid-icon">⚠</div>
          <h2 className="text-white text-lg font-semibold mb-2" data-testid="register-invalid-title">邀请链接无效</h2>
          <p className="text-white/60 text-sm mb-6" data-testid="register-invalid-msg">{errorMsg}</p>
          <a
            href="/login"
            className="inline-block px-6 py-2 rounded-xl text-sm font-medium"
            style={{ background: 'linear-gradient(135deg, #B1E2FF, #9381FF)', color: '#000' }}
            data-testid="register-goto-login-btn"
          >
            返回登录
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

        <h1 className="text-center mb-2 font-semibold text-white text-xl" data-testid="register-title">完成注册</h1>
        {prefillEmail && (
          <p className="text-center mb-6 text-sm text-white/50" data-testid="register-email-display">邀请邮箱: {prefillEmail}</p>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div data-testid="register-username-field">
            <label className="block mb-1.5 font-semibold text-xs text-white/50">用户名</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="登录时使用的用户名"
              className="w-full px-4 py-2.5 rounded-xl bg-white/5 border border-white/10 text-white placeholder-white/30 focus:outline-none focus:border-[#B1E2FF] transition-all"
              data-testid="register-username-input"
            />
            {usernameError && <p className="mt-1 text-sm text-red-400" data-testid="register-username-error">{usernameError}</p>}
          </div>

          <div data-testid="register-displayname-field">
            <label className="block mb-1.5 font-semibold text-xs text-white/50">显示名称</label>
            <input
              type="text"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="您的姓名"
              className="w-full px-4 py-2.5 rounded-xl bg-white/5 border border-white/10 text-white placeholder-white/30 focus:outline-none focus:border-[#B1E2FF] transition-all"
              data-testid="register-displayname-input"
            />
            {displayNameError && <p className="mt-1 text-sm text-red-400" data-testid="register-displayname-error">{displayNameError}</p>}
          </div>

          <div data-testid="register-password-field">
            <label className="block mb-1.5 font-semibold text-xs text-white/50">密码</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="至少 6 个字符"
              className="w-full px-4 py-2.5 rounded-xl bg-white/5 border border-white/10 text-white placeholder-white/30 focus:outline-none focus:border-[#B1E2FF] transition-all"
              data-testid="register-password-input"
            />
            {passwordError && <p className="mt-1 text-sm text-red-400" data-testid="register-password-error">{passwordError}</p>}
          </div>

          <div data-testid="register-confirm-field">
            <label className="block mb-1.5 font-semibold text-xs text-white/50">确认密码</label>
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder="再次输入密码"
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
            {submitting ? '注册中...' : '完成注册'}
          </button>
        </form>

        <p className="text-center text-xs mt-6 text-white/40">
          已有账号？<a href="/login" className="text-[#B1E2FF] ml-1" data-testid="register-login-link">返回登录</a>
        </p>
      </div>
    </div>
  );
}

export default function RegisterPage() {
  return (
    <Suspense fallback={
      <div className="min-h-screen flex items-center justify-center bg-black">
        <p className="text-white/60">加载中...</p>
      </div>
    }>
      <RegisterForm />
    </Suspense>
  );
}
