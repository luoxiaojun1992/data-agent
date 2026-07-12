'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '../../../lib/api';

export default function SysConfigPage() {
  const { auth, apiFetch } = useAuth();

  const [sessionRecovery, setSessionRecovery] = useState(24);
  const [auditRetention, setAuditRetention] = useState(90);
  const [notifTTL, setNotifTTL] = useState(90);
  const [emailWhitelist, setEmailWhitelist] = useState<string[]>([]);
  const [newEmail, setNewEmail] = useState('');
  const [reportRetry, setReportRetry] = useState(3);
  const [error, setError] = useState('');
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  const showToast = (msg: string, type: 'success' | 'error') => {
    setToast({ message: msg, type });
    setTimeout(() => setToast(null), 3000);
  };

  const fetchConfig = useCallback(async () => {
    try {
      const res = await apiFetch('/sysconfig');
      if (res.ok) {
        const data = await res.json();
        if (data.session_recovery_hours !== undefined) setSessionRecovery(Number(data.session_recovery_hours));
        if (data.audit_retention_days !== undefined) setAuditRetention(Number(data.audit_retention_days));
        if (data.notification_ttl_days !== undefined) setNotifTTL(Number(data.notification_ttl_days));
        if (data.report_retry_count !== undefined) setReportRetry(Number(data.report_retry_count));
        if (Array.isArray(data.email_whitelist)) setEmailWhitelist(data.email_whitelist);
        setError('');
      } else {
        setError('加载配置失败');
      }
    } catch {
      setError('加载配置失败');
    }
  }, [apiFetch]);

  useEffect(() => {
    if (auth.hydrated) fetchConfig();
  }, [auth.hydrated, fetchConfig]);

  const save = async (key: string, value: number | string[]) => {
    try {
      const res = await apiFetch('/sysconfig', {
        method: 'PUT',
        body: JSON.stringify({ [key]: value }),
      });
      if (!res.ok) {
        const d = await res.json();
        showToast(d.error || '保存失败', 'error');
        if (d.error) setError(d.error);
        return;
      }
      showToast('配置已更新', 'success');
      setError('');
      fetchConfig();
    } catch {
      showToast('保存失败', 'error');
    }
  };

  return (
    <AppLayout>
      <div className="animate-fade-in" data-testid="sysconfig-page-header">
        <div className="mb-8">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">系统配置</h2>
        </div>

        {/* Toast */}
        {toast && (
          <div data-testid="sysconfig-save-success-toast" style={{ position: 'fixed', top: 20, right: 20, zIndex: 9999,
            background: toast.type === 'success' ? 'rgba(16,185,129,0.9)' : 'rgba(239,68,68,0.9)',
            color: '#fff', padding: '12px 20px', borderRadius: '8px', fontSize: '14px',
          }}>
            {toast.message}
          </div>
        )}

        {/* Error banner */}
        {error && (
          <div style={{ padding: '12px', marginBottom: '16px', background: 'rgba(239,68,68,0.1)', borderRadius: '8px', color: '#ef4444', fontSize: '13px' }}>
            {error}
          </div>
        )}

        {/* Session Recovery Buffer */}
        <div className="glass" data-testid="sysconfig-session-recovery" style={cardStyle}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div>
              <h3 style={titleStyle}>Session 恢复缓冲期</h3>
              <p style={descStyle}>默认 24 小时，可配置 1~168 小时</p>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
              <input data-testid="sysconfig-session-recovery-input" type="number" min={1} max={168}
                value={sessionRecovery} onChange={(e) => setSessionRecovery(Number(e.target.value))} style={inputStyle} />
              <span style={{ fontSize: '13px', color: '#7A7A7A' }}>小时</span>
              <button data-testid="sysconfig-session-recovery-save"
                onClick={() => save('session_recovery_hours', sessionRecovery)} style={btnStyle}>保存</button>
            </div>
          </div>
          <p data-testid="sysconfig-session-recovery-error" style={{ color: '#ef4444', fontSize: '12px', marginTop: '8px', display: 'none' }} />
        </div>

        {/* Audit Retention */}
        <div className="glass" data-testid="sysconfig-audit-retention" style={cardStyle}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div>
              <h3 style={titleStyle}>审计日志保留天数</h3>
              <p style={descStyle}>默认 90 天</p>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
              <input type="number" min={1} value={auditRetention}
                onChange={(e) => setAuditRetention(Number(e.target.value))} style={inputStyle} />
              <span style={{ fontSize: '13px', color: '#7A7A7A' }}>天</span>
              <button onClick={() => save('audit_retention_days', auditRetention)} style={btnStyle}>保存</button>
            </div>
          </div>
        </div>

        {/* Notification TTL */}
        <div className="glass" data-testid="sysconfig-notif-ttl" style={cardStyle}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div>
              <h3 style={titleStyle}>通知 TTL 天数</h3>
              <p style={descStyle}>默认 90 天</p>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
              <input type="number" min={1} value={notifTTL}
                onChange={(e) => setNotifTTL(Number(e.target.value))} style={inputStyle} />
              <span style={{ fontSize: '13px', color: '#7A7A7A' }}>天</span>
              <button onClick={() => save('notification_ttl_days', notifTTL)} style={btnStyle}>保存</button>
            </div>
          </div>
        </div>

        {/* Email Whitelist */}
        <div className="glass" data-testid="sysconfig-email-whitelist" style={cardStyle}>
          <div>
            <h3 style={titleStyle}>邮件域名白名单</h3>
            <p style={{ ...descStyle, marginBottom: '12px' }}>添加允许注册的邮件域名</p>
          </div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px', marginBottom: '12px' }}>
            {emailWhitelist.map((domain, i) => (
              <div key={i} style={{ display: 'flex', alignItems: 'center', gap: '6px',
                background: 'rgba(92,124,250,0.1)', borderRadius: '8px', padding: '4px 12px' }}>
                <span style={{ fontSize: '13px', color: '#5c7cfa' }}>{domain}</span>
                <button onClick={() => {
                  const next = emailWhitelist.filter((_, j) => j !== i);
                  setEmailWhitelist(next);
                  save('email_whitelist', next);
                }} style={{ background: 'none', border: 'none', color: '#ef4444', cursor: 'pointer', fontSize: '14px' }}>
                  ×
                </button>
              </div>
            ))}
          </div>
          <div style={{ display: 'flex', gap: '8px' }}>
            <input data-testid="sysconfig-email-input" placeholder="example.com"
              value={newEmail} onChange={(e) => setNewEmail(e.target.value)} style={inputStyle} />
            <button data-testid="sysconfig-email-add" onClick={() => {
              if (newEmail && !emailWhitelist.includes(newEmail)) {
                const next = [...emailWhitelist, newEmail];
                setEmailWhitelist(next);
                setNewEmail('');
                save('email_whitelist', next);
              }
            }} style={btnStyle}>添加</button>
          </div>
        </div>

        {/* Report Retry */}
        <div className="glass" data-testid="sysconfig-report-retry" style={cardStyle}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div>
              <h3 style={titleStyle}>报告格式校验重试次数</h3>
              <p style={descStyle}>默认 3 次</p>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
              <input type="number" min={1} value={reportRetry}
                onChange={(e) => setReportRetry(Number(e.target.value))} style={inputStyle} />
              <span style={{ fontSize: '13px', color: '#7A7A7A' }}>次</span>
              <button onClick={() => save('report_retry_count', reportRetry)} style={btnStyle}>保存</button>
            </div>
          </div>
        </div>
      </div>
    </AppLayout>
  );
}

const cardStyle: React.CSSProperties = { padding: '20px', marginBottom: '16px' };
const titleStyle: React.CSSProperties = { fontSize: '15px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '4px' };
const descStyle: React.CSSProperties = { fontSize: '13px', color: '#7A7A7A' };
const inputStyle: React.CSSProperties = {
  width: '100px', padding: '8px 12px', background: 'rgba(255,255,255,0.06)',
  border: '1px solid rgba(255,255,255,0.1)', borderRadius: '8px', fontSize: '14px',
  color: 'var(--text-primary)', outline: 'none', boxSizing: 'border-box',
};
const btnStyle: React.CSSProperties = {
  padding: '8px 16px', background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
  color: '#fff', border: 'none', borderRadius: '8px', fontSize: '13px', fontWeight: 600, cursor: 'pointer',
};
