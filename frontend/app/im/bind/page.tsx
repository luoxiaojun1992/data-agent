'use client';

import React, { Suspense, useState, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import AppLayout from '../../providers';
import { useAuth } from '@/lib/api';

function BindPage() {
  const { auth, apiFetch } = useAuth();
  const router = useRouter();

  const [appId, setAppId] = useState('');
  const [appSecret, setAppSecret] = useState('');
  const [status, setStatus] = useState<'idle' | 'saving' | 'success' | 'error'>('idle');
  const [errorMsg, setErrorMsg] = useState('');

  const fetchConfig = useCallback(async () => {
    try {
      const res = await apiFetch('/im/bind');
      if (res.ok) {
        const data = await res.json();
        if (data.feishu_app_id) setAppId(data.feishu_app_id);
        if (data.feishu_app_secret) setAppSecret(data.feishu_app_secret);
      }
    } catch {}
  }, [apiFetch]);

  useEffect(() => { fetchConfig(); }, [fetchConfig]);

  const handleSave = async () => {
    if (!appId.trim() || !appSecret.trim()) return;
    setStatus('saving');
    try {
      const res = await apiFetch('/im/bind', {
        method: 'PUT',
        body: JSON.stringify({ feishu_app_id: appId, feishu_app_secret: appSecret }),
      });
      if (res.ok) {
        setStatus('success');
        setTimeout(() => setStatus('idle'), 3000);
      } else {
        const d = await res.json();
        setErrorMsg(d.error || '保存失败');
        setStatus('error');
      }
    } catch {
      setErrorMsg('网络错误');
      setStatus('error');
    }
  };

  return (
    <AppLayout>
      <div className="animate-fade-in p-8" data-testid="im-bind-page">
        <h2 className="text-2xl font-bold text-[var(--text-primary)] mb-2">飞书绑定</h2>
        <p className="text-[var(--text-secondary)] text-sm mb-6">
          在飞书开放平台创建机器人应用，获取 App ID 和 App Secret 后填入下方。
          绑定后飞书机器人的消息将关联到你的 DataAgent 账号。
        </p>

        {status === 'success' && (
          <div data-testid="im-bind-success" style={{ padding: '12px 16px', marginBottom: '16px',
            background: 'rgba(16,185,129,0.1)', borderRadius: '10px', color: '#10b981', fontSize: '13px' }}>
            绑定成功！飞书机器人已关联到你的账号。
          </div>
        )}

        <div className="glass" style={{ padding: '24px', maxWidth: '480px' }}>
          <div style={{ marginBottom: '12px' }}>
            <label style={{ display: 'block', fontSize: '12px', color: '#666', marginBottom: '4px' }}>App ID</label>
            <input data-testid="im-bind-app-id" value={appId}
              onChange={(e) => setAppId(e.target.value)}
              placeholder="cli_xxxxxxxx"
              style={{ width: '100%', padding: '8px 12px', background: 'rgba(255,255,255,0.06)',
                border: '1px solid rgba(255,255,255,0.1)', borderRadius: '8px', fontSize: '14px',
                color: 'var(--text-primary)', outline: 'none', boxSizing: 'border-box' }} />
          </div>
          <div style={{ marginBottom: '12px' }}>
            <label style={{ display: 'block', fontSize: '12px', color: '#666', marginBottom: '4px' }}>App Secret</label>
            <input data-testid="im-bind-app-secret" type="password" value={appSecret}
              onChange={(e) => setAppSecret(e.target.value)}
              placeholder="••••••••"
              style={{ width: '100%', padding: '8px 12px', background: 'rgba(255,255,255,0.06)',
                border: '1px solid rgba(255,255,255,0.1)', borderRadius: '8px', fontSize: '14px',
                color: 'var(--text-primary)', outline: 'none', boxSizing: 'border-box' }} />
          </div>
          {status === 'error' && (
            <p data-testid="im-bind-error" style={{ color: '#ef4444', fontSize: '12px', marginBottom: '8px' }}>{errorMsg}</p>
          )}
          <button data-testid="im-bind-submit" onClick={handleSave} disabled={status === 'saving'}
            style={{ width: '100%', padding: '10px', background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
              color: '#fff', border: 'none', borderRadius: '8px', fontSize: '14px', fontWeight: 600, cursor: 'pointer' }}>
            {status === 'saving' ? '保存中...' : '绑定'}
          </button>
        </div>
      </div>
    </AppLayout>
  );
}

export default function ImBindPage() {
  return (
    <Suspense fallback={<div className="min-h-screen" />}>
      <BindPage />
    </Suspense>
  );
}
