'use client';

import React, { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useAuth } from '@/lib/api';

export default function LoginPage() {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const { login } = useAuth();
  const router = useRouter();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      await login(username, password);
      router.push('/');
    } catch (err: any) {
      setError(err.message || '登录失败');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-[var(--bg-primary)]">
      {/* Aurora glow backdrop */}
      <div className="fixed inset-0 pointer-events-none">
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[600px] rounded-full bg-[var(--accent-glow)] opacity-10 blur-3xl" />
      </div>

      <div className="glass p-8 w-full max-w-md animate-fade-in">
        <div className="text-center mb-8">
          <span className="text-5xl mb-4 block">🔮</span>
          <h1 className="text-2xl font-bold text-[var(--text-primary)]">DataAgent</h1>
          <p className="text-sm text-[var(--text-secondary)] mt-2">企业数据分析平台</p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-5">
          <div>
            <label className="block text-sm text-[var(--text-secondary)] mb-1.5">用户名</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="输入用户名"
              className="w-full px-4 py-2.5 rounded-xl bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] placeholder-[var(--text-secondary)] focus:outline-none focus:border-[var(--accent)] focus:ring-1 focus:ring-[var(--accent)] transition-all"
              required
            />
          </div>
          <div>
            <label className="block text-sm text-[var(--text-secondary)] mb-1.5">密码</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="输入密码"
              className="w-full px-4 py-2.5 rounded-xl bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] placeholder-[var(--text-secondary)] focus:outline-none focus:border-[var(--accent)] focus:ring-1 focus:ring-[var(--accent)] transition-all"
              required
            />
          </div>

          {error && (
            <div className="p-3 rounded-xl bg-red-500/10 border border-red-500/20 text-red-400 text-sm">
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={loading}
            className="w-full py-3 bg-[var(--accent)] text-white rounded-xl font-medium hover:opacity-90 disabled:opacity-50 transition-all"
          >
            {loading ? '登录中...' : '登 录'}
          </button>
        </form>

        <p className="text-center text-xs text-[var(--text-secondary)] mt-6">
          首次使用？请使用管理员账号登录
        </p>
      </div>
    </div>
  );
}
