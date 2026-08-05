'use client';

import React, { useState, useEffect } from 'react';
import { useParams } from 'next/navigation';
import AppLayout from '../../../../providers';
import { useAuth } from '../../../../../lib/api';

interface RBACRole { id: string; name: string; display_name: string; level: number; type: string; }

const PAGE_SIZE = 10;

export default function UserRBACRolesPage() {
  const { auth, apiFetch } = useAuth();
  const { id } = useParams<{ id: string }>();
  const [roles, setRoles] = useState<RBACRole[]>([]);
  const [allRoles, setAllRoles] = useState<RBACRole[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');
  const [showAdd, setShowAdd] = useState(false);
  const [toast, setToast] = useState('');

  const showToast = (msg: string) => { setToast(msg); setTimeout(() => setToast(''), 2000); };

  const fetchRoles = () => {
    apiFetch(`/admin/users/${id}/rbac-roles?page=${page}&page_size=${PAGE_SIZE}`).then(r => r.json()).then(data => {
      setRoles(data.roles || []); setTotal(data.total || 0);
    }).catch(() => showToast('加载失败'));
  };
  const fetchAll = () => {
    apiFetch('/rbac/roles?page=1&page_size=200').then(r => r.json()).then(data => setAllRoles(data.permissions || data.roles || []));
  };

  useEffect(() => { if (auth.hydrated) fetchRoles(); }, [id, page, auth.hydrated]);

  const available = allRoles.filter(r => !roles.find(rr => rr.id === r.id) &&
    (search === '' || r.name.includes(search) || r.display_name.includes(search)));

  const add = (roleID: string) => {
    apiFetch(`/admin/users/${id}/rbac-roles`, { method: 'POST', body: JSON.stringify({ role_id: roleID }) })
      .then(() => { showToast('已添加'); fetchRoles(); }).catch(() => showToast('添加失败'));
  };
  const remove = (roleID: string) => {
    apiFetch(`/admin/users/${id}/rbac-roles/${roleID}`, { method: 'DELETE' })
      .then(() => { showToast('已移除'); fetchRoles(); }).catch(() => showToast('移除失败'));
  };

  const btnPri: React.CSSProperties = { padding: '8px 16px', background: '#5c7cfa', color: '#fff', border: 'none', borderRadius: 8, cursor: 'pointer', fontSize: 14 };
  const btnSec: React.CSSProperties = { padding: '8px 16px', background: 'transparent', color: 'var(--text-secondary)', border: '1px solid var(--border)', borderRadius: 8, cursor: 'pointer', fontSize: 14 };
  const btnSm: React.CSSProperties = { padding: '4px 10px', background: 'transparent', border: '1px solid var(--border)', borderRadius: 6, cursor: 'pointer', fontSize: 12 };
  const mo: React.CSSProperties = { position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 9999 };
  const mc: React.CSSProperties = { background: 'var(--card-bg)', padding: 24, borderRadius: 12, minWidth: 400 };
  const inp: React.CSSProperties = { width: '100%', padding: 8, borderRadius: 6, border: '1px solid var(--border)', background: 'var(--input-bg)', color: 'var(--text-primary)', fontSize: 14, boxSizing: 'border-box' };

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const pagination = (<div style={{ display: 'flex', gap: 4, justifyContent: 'center', marginTop: 16 }}>
    <button disabled={page <= 1} onClick={() => setPage(page - 1)} style={{ padding: '4px 10px', border: '1px solid var(--border)', borderRadius: 6, cursor: 'pointer', fontSize: 13, background: 'transparent', color: 'var(--text-primary)' }}>‹</button>
    {Array.from({ length: totalPages }, (_, i) => i + 1).map(p => (
      <button key={p} onClick={() => setPage(p)} style={{ padding: '4px 10px', border: '1px solid var(--border)', borderRadius: 6, cursor: 'pointer', fontSize: 13, background: p === page ? '#5c7cfa' : 'transparent', color: p === page ? '#fff' : 'var(--text-primary)' }}>{p}</button>
    ))}
    <button disabled={page >= totalPages} onClick={() => setPage(page + 1)} style={{ padding: '4px 10px', border: '1px solid var(--border)', borderRadius: 6, cursor: 'pointer', fontSize: 13, background: 'transparent', color: 'var(--text-primary)' }}>›</button>
  </div>);

  return (
    <AppLayout>
      <div style={{ maxWidth: 700, margin: '0 auto', padding: 20 }}>
        <a href="/admin/users" style={{ color: '#5c7cfa', fontSize: 13 }}>← 返回用户管理</a>
        <h2 style={{ fontSize: 20, fontWeight: 600, margin: '8px 0' }}>RBAC 角色管理</h2>

        <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
          <button data-testid="rbac-user-add-role-btn" onClick={() => { fetchAll(); setShowAdd(true); }}
            style={btnPri}>+ 添加角色（{roles.length}/10）</button>
        </div>

        {roles.length === 0 ? (
          <p style={{ color: 'var(--text-secondary)', fontSize: 14 }}>暂无关联角色</p>
        ) : (
          <div>
            {roles.map((r) => (
              <div key={r.id} data-testid={`rbac-user-role-${r.id}`} className="glass glass-hover"
                style={{ padding: '12px 16px', marginBottom: 8, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <div style={{ flex: 1 }}>
                  <strong>{r.display_name}</strong>
                  <span style={{ padding: '2px 8px', borderRadius: 6, fontSize: 11, marginLeft: 8,
                    background: r.level === 0 ? '#ef4444' : r.level === 1 ? '#f59e0b' : '#34d399', color: '#fff' }}>L{r.level}</span>
                  <span style={{ fontSize: 12, color: 'var(--text-secondary)', marginLeft: 8 }}>{r.name}</span>
                </div>
                {r.type !== 'builtin' && <button data-testid={`rbac-user-role-remove-${r.id}`} onClick={() => remove(r.id)} style={{ ...btnSm, color: '#ef4444' }}>移除</button>}
              </div>
            ))}
          </div>
        )}
        {pagination}

        {showAdd && (
          <div style={mo} onClick={() => setShowAdd(false)}><div style={mc} onClick={e => e.stopPropagation()}>
            <h3 style={{ marginBottom: 12 }}>添加角色</h3>
            {roles.length >= 10 && <p style={{ color: '#ef4444', fontSize: 13, marginBottom: 8 }}>已达到上限 10 个</p>}
            <input style={inp} placeholder="搜索角色..." value={search} onChange={e => setSearch(e.target.value)} />
            <div style={{ maxHeight: 300, overflowY: 'auto', marginTop: 8 }}>
              {available.map((r) => (
                <div key={r.id} data-testid={`rbac-avail-role-${r.id}`}
                  style={{ display: 'flex', justifyContent: 'space-between', padding: '8px 0', borderBottom: '1px solid rgba(255,255,255,0.06)' }}>
                  <div>
                    <span style={{ fontSize: 14 }}>{r.display_name}</span>
                    <span style={{ padding: '2px 8px', borderRadius: 6, fontSize: 11, marginLeft: 8,
                      background: r.level === 0 ? '#ef4444' : r.level === 1 ? '#f59e0b' : '#34d399', color: '#fff' }}>L{r.level}</span>
                  </div>
                  <button data-testid={`rbac-user-add-role-${r.id}`} onClick={() => add(r.id)} style={{ ...btnSm, color: '#5c7cfa' }} disabled={roles.length >= 10}>添加</button>
                </div>
              ))}
            </div>
            <div style={{ marginTop: 12, textAlign: 'right' }}><button onClick={() => setShowAdd(false)} style={btnSec}>关闭</button></div>
          </div></div>
        )}

        {toast && <div style={{ position: 'fixed', bottom: 20, right: 20, padding: '10px 20px', borderRadius: 8,
          background: toast.includes('失败') ? '#ef4444' : '#34d399', color: '#fff', zIndex: 9999 }}>{toast}</div>}
      </div>
    </AppLayout>
  );
}
