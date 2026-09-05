'use client';

import React, { useState, useEffect, useCallback, useRef } from 'react';
import { useAuth, NOTIFICATION_PERMS } from '@/lib/api';

interface Notif {
  id: string;
  title: string;
  content: string;
  type: string;
  created_at: string;
  read: boolean;
}

type SendMode = 'direct' | 'broadcast';

export default function NotificationBell() {
  const { auth, apiFetch, canAccess } = useAuth();
  const [unread, setUnread] = useState(0);
  const [notifs, setNotifs] = useState<Notif[]>([]);
  const [open, setOpen] = useState(false);
  const [sendMode, setSendMode] = useState<SendMode | null>(null);
  const [sendForm, setSendForm] = useState({ title: '', content: '', type: 'info', target: '' });
  const ref = useRef<HTMLDivElement>(null);

  // SPEC-084 §6.2: 定向发送 is user-level, 广播 is admin-level. The bell
  // only renders each send entry when the user holds the matching permission.
  const canSend = canAccess(NOTIFICATION_PERMS.send);
  const canBroadcast = canAccess(NOTIFICATION_PERMS.broadcast);

  const fetchCount = useCallback(async () => {
    if (!auth.token) return;
    try {
      const res = await apiFetch('/notifications/unread-count');
      if (res.ok) {
        const d = await res.json();
        setUnread(d.count || 0);
      }
    } catch { /* */ }
  }, [apiFetch, auth.token]);

  const fetchList = useCallback(async () => {
    try {
      const res = await apiFetch('/notifications?limit=20');
      if (res.ok) {
        const data = await res.json();
        setNotifs(data.map((n: any) => ({ ...n, read: (n.read_by || []).length > 0 })));
      }
    } catch { /* */ }
  }, [apiFetch]);

  useEffect(() => {
    fetchCount();
    const t = setInterval(fetchCount, 30000);
    return () => clearInterval(t);
  }, [fetchCount]);

  useEffect(() => {
    if (open) fetchList();
  }, [open, fetchList]);

  // Close on outside click
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    if (open) document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  const handleMarkRead = async (id: string) => {
    await apiFetch(`/notifications/${id}/read`, { method: 'PUT' });
    setNotifs((p) => p.map((n) => (n.id === id ? { ...n, read: true } : n)));
    setUnread((c) => Math.max(0, c - 1));
  };

  const handleMarkAllRead = async () => {
    await apiFetch('/notifications/read-all', { method: 'PUT' });
    setNotifs((p) => p.map((n) => ({ ...n, read: true })));
    setUnread(0);
  };

  const openSend = (mode: SendMode) => {
    setSendForm({ title: '', content: '', type: 'info', target: '' });
    setSendMode(mode);
  };

  const handleSend = async () => {
    try {
      const res = sendMode === 'broadcast'
        ? await apiFetch('/notifications/broadcast', {
            method: 'POST',
            body: JSON.stringify({ title: sendForm.title, content: sendForm.content, type: sendForm.type }),
          })
        : await apiFetch('/notifications', {
            method: 'POST',
            body: JSON.stringify({
              title: sendForm.title, content: sendForm.content, type: sendForm.type,
              target_ids: sendForm.target ? [sendForm.target] : [],
            }),
          });
      if (res.ok) {
        setSendMode(null);
        setSendForm({ title: '', content: '', type: 'info', target: '' });
        fetchCount();
      }
    } catch { /* */ }
  };

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <button data-testid="notif-bell-icon" onClick={() => setOpen(!open)}
        style={{ background: 'transparent', border: 'none', cursor: 'pointer', fontSize: '20px', position: 'relative', padding: '4px' }}>
        🔔
        {unread > 0 && (
          <span data-testid="notif-unread-badge" style={{ position: 'absolute', top: -2, right: -2,
            width: '18px', height: '18px', borderRadius: '50%', background: '#ef4444', color: '#fff',
            fontSize: '10px', fontWeight: 700, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <span data-testid="notif-unread-count">{unread > 99 ? '99+' : unread}</span>
          </span>
        )}
      </button>

      {open && (
        <div data-testid="notif-dropdown" style={{ position: 'absolute', right: 0, top: '100%', zIndex: 1000,
          width: '360px', maxHeight: '400px', overflowY: 'auto', background: '#1a1a2e',
          border: '1px solid rgba(255,255,255,0.1)', borderRadius: '12px', boxShadow: '0 8px 32px rgba(0,0,0,0.5)' }}>
          <div style={{ padding: '12px 16px', display: 'flex', justifyContent: 'space-between', alignItems: 'center',
            borderBottom: '1px solid rgba(255,255,255,0.06)' }}>
            <span style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text-primary)' }}>通知</span>
            <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
              {unread > 0 && (
                <button data-testid="notif-mark-all-read" onClick={handleMarkAllRead}
                  style={{ fontSize: '12px', color: '#5c7cfa', background: 'none', border: 'none', cursor: 'pointer' }}>
                  全部已读
                </button>
              )}
              {canSend && (
                <button data-testid="notif-send-direct-btn" onClick={() => openSend('direct')}
                  style={{ fontSize: '12px', color: '#34D399', background: 'none', border: 'none', cursor: 'pointer' }}>
                  定向发送
                </button>
              )}
              {canBroadcast && (
                <button data-testid="notif-broadcast-btn" onClick={() => openSend('broadcast')}
                  style={{ fontSize: '12px', color: '#F59E0B', background: 'none', border: 'none', cursor: 'pointer' }}>
                  广播
                </button>
              )}
            </div>
          </div>
          {notifs.length === 0 ? (
            <div style={{ padding: '40px', textAlign: 'center', color: '#666', fontSize: '13px' }}>暂无通知</div>
          ) : (
            notifs.map((n) => (
              <div key={n.id} data-testid={`notif-item-${n.id}`}
                data-teststatus={n.read ? 'read' : 'unread'}
                onClick={() => !n.read && handleMarkRead(n.id)}
                style={{ padding: '12px 16px', cursor: n.read ? 'default' : 'pointer',
                  background: n.read ? 'transparent' : 'rgba(59,130,246,0.05)',
                  borderBottom: '1px solid rgba(255,255,255,0.04)' }}>
                <p style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '4px' }}>
                  {n.read ? '' : '● '}{n.title}
                </p>
                <p style={{ fontSize: '12px', color: '#7A7A7A', marginBottom: '4px' }}>{n.content?.slice(0, 60)}</p>
                <p style={{ fontSize: '11px', color: '#555' }}>
                  {n.created_at ? new Date(n.created_at).toLocaleString('zh-CN') : ''}
                </p>
              </div>
            ))
          )}
        </div>
      )}

      {/* Send Modal (direct 定向 / broadcast 广播) */}
      {sendMode && (
        <div data-testid={sendMode === 'broadcast' ? 'notif-broadcast-modal' : 'notif-send-modal'}
          style={{ position: 'fixed', inset: 0, zIndex: 2000,
          background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}
          onClick={(e) => { if (e.target === e.currentTarget) setSendMode(null); }}>
          <div className="glass" style={{ padding: '24px', maxWidth: '440px', width: '90%' }}>
            <h3 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '12px' }}>
              {sendMode === 'broadcast' ? '广播通知' : '定向发送站内信'}
            </h3>
            {sendMode === 'direct' && (
              <input data-testid="notif-send-recipient" placeholder="接收人 ID" value={sendForm.target}
                onChange={(e) => setSendForm({ ...sendForm, target: e.target.value })}
                style={inputStyle} />
            )}
            <input data-testid="notif-send-subject" placeholder="标题" value={sendForm.title}
              onChange={(e) => setSendForm({ ...sendForm, title: e.target.value })}
              style={{ ...inputStyle, marginTop: '8px' }} />
            <textarea data-testid="notif-send-body" placeholder="内容" value={sendForm.content}
              onChange={(e) => setSendForm({ ...sendForm, content: e.target.value })}
              style={{ ...inputStyle, marginTop: '8px', height: '80px', resize: 'vertical' }} />
            <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end', marginTop: '12px' }}>
              <button onClick={() => setSendMode(null)} style={secondaryBtn}>取消</button>
              <button data-testid="notif-send-submit" onClick={handleSend} style={primaryBtn}>发送</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

const inputStyle: React.CSSProperties = { width: '100%', padding: '8px 12px', background: 'rgba(255,255,255,0.06)', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '8px', fontSize: '14px', color: 'var(--text-primary)', outline: 'none', boxSizing: 'border-box' };
const primaryBtn: React.CSSProperties = { padding: '8px 20px', background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)', color: '#fff', border: 'none', borderRadius: '8px', fontSize: '14px', cursor: 'pointer' };
const secondaryBtn: React.CSSProperties = { padding: '8px 20px', background: 'rgba(255,255,255,0.06)', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '8px', fontSize: '14px', color: '#7A7A7A', cursor: 'pointer' };
