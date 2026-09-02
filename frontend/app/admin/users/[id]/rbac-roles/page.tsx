'use client';

import React, { useState, useEffect } from 'react';
import { useParams } from 'next/navigation';
import AppLayout from '../../../../providers';
import { useAuth } from '../../../../../lib/api';
import { useDebouncedSearch, SearchableOption } from '../../../../components/SearchableSelect';
import Pagination from '../../../../components/Pagination';
import { primaryButtonStyle, modalOverlayStyle } from '../../../../components/ui';

interface RBACRole { id: string; name: string; display_name: string; level: number; type: string; }

const PAGE_SIZE = 10;

export default function UserRBACRolesPage() {
  const { auth, apiFetch } = useAuth();
  const { id } = useParams<{ id: string }>();
  const [roles, setRoles] = useState<RBACRole[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [showAdd, setShowAdd] = useState(false);
  const [toast, setToast] = useState('');

  const showToast = (msg: string) => { setToast(msg); setTimeout(() => setToast(''), 2000); };

  const fetchRoles = () => {
    apiFetch(`/admin/users/${id}/rbac-roles?page=${page}&page_size=${PAGE_SIZE}`).then(r => r.json()).then(data => {
      setRoles(data.roles || []); setTotal(data.total || 0);
    }).catch(() => showToast('加载失败'));
  };

  useEffect(() => { if (auth.hydrated) fetchRoles(); }, [id, page, auth.hydrated]);

  const add = (roleID: string) => {
    apiFetch(`/admin/users/${id}/rbac-roles`, { method: 'POST', body: JSON.stringify({ role_id: roleID }) })
      .then(() => { showToast('已添加'); fetchRoles(); }).catch(() => showToast('添加失败'));
  };
  const remove = (roleID: string) => {
    apiFetch(`/admin/users/${id}/rbac-roles/${roleID}`, { method: 'DELETE' })
      .then(() => { showToast('已移除'); fetchRoles(); }).catch(() => showToast('移除失败'));
  };

  const btnSec: React.CSSProperties = { padding: '8px 16px', background: 'transparent', color: 'var(--text-secondary)', border: '1px solid var(--border)', borderRadius: 8, cursor: 'pointer', fontSize: 14 };
  const btnSm: React.CSSProperties = { padding: '4px 10px', background: 'transparent', border: '1px solid var(--border)', borderRadius: 6, cursor: 'pointer', fontSize: 12 };

  return (
    <AppLayout>
      <div style={{ maxWidth: 700, margin: '0 auto', padding: 20 }}>
        <a href="/admin/users" style={{ color: '#5c7cfa', fontSize: 13 }}>← 返回用户管理</a>
        <h2 style={{ fontSize: 20, fontWeight: 600, margin: '8px 0' }}>RBAC 角色管理</h2>

        <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
          <button data-testid="rbac-user-add-role-btn" onClick={() => setShowAdd(true)}
            style={primaryButtonStyle}>+ 添加角色（{roles.length}/10）</button>
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
        <Pagination page={page} total={total} pageSize={PAGE_SIZE} onChange={setPage} />

        {showAdd && (
          <AddRoleModal apiFetch={apiFetch} userId={id} maxReached={roles.length >= 10}
            onAdd={add} onClose={() => setShowAdd(false)} />
        )}

        {toast && <div style={{ position: 'fixed', bottom: 20, right: 20, padding: '10px 20px', borderRadius: 8,
          background: toast.includes('失败') ? '#ef4444' : '#34d399', color: '#fff', zIndex: 9999 }}>{toast}</div>}
      </div>
    </AppLayout>
  );
}

// AddRoleModal loads available roles straight from the backend (SPEC-074):
// /admin/rbac/roles?limit=20&exclude_user_id=<id> (default) and appends
// &q=<kw> for debounced search. The client only renders the returned top-N;
// no local filter/sort/slice.
function AddRoleModal({ apiFetch, userId, maxReached, onAdd, onClose }: {
  apiFetch: (path: string, options?: RequestInit) => Promise<Response>;
  userId: string;
  maxReached: boolean;
  onAdd: (roleId: string) => void;
  onClose: () => void;
}) {
  const fetchAvail = async (q: string, limit: number): Promise<SearchableOption[]> => {
    const res = await apiFetch(`/admin/rbac/roles?limit=${limit}&exclude_user_id=${userId}${q ? `&q=${encodeURIComponent(q)}` : ''}`);
    if (!res.ok) throw new Error('加载失败');
    const data = await res.json();
    return (data.roles || []) as SearchableOption[];
  };
  const { items, loading, error, query, onSearch } = useDebouncedSearch(fetchAvail, 20);

  const mo: React.CSSProperties = { ...modalOverlayStyle, zIndex: 9999 };
  const mc: React.CSSProperties = { background: 'var(--bg-secondary)', border: '1px solid var(--border-glass)', padding: 24, borderRadius: 12, minWidth: 400 };
  const inp: React.CSSProperties = { width: '100%', padding: 8, borderRadius: 6, border: '1px solid var(--border)', background: 'var(--input-bg)', color: 'var(--text-primary)', fontSize: 14, boxSizing: 'border-box' };
  const btnSm: React.CSSProperties = { padding: '4px 10px', background: 'transparent', border: '1px solid var(--border)', borderRadius: 6, cursor: 'pointer', fontSize: 12 };
  const btnSec: React.CSSProperties = { padding: '8px 16px', background: 'transparent', color: 'var(--text-secondary)', border: '1px solid var(--border)', borderRadius: 8, cursor: 'pointer', fontSize: 14 };

  return (
    <div style={mo} onClick={onClose}><div style={mc} onClick={e => e.stopPropagation()}>
      <h3 style={{ marginBottom: 12 }}>添加角色</h3>
      {maxReached && <p style={{ color: '#ef4444', fontSize: 13, marginBottom: 8 }}>已达到上限 10 个</p>}
      <input style={inp} placeholder="搜索角色..." value={query} onChange={e => onSearch(e.target.value)} />
      <div style={{ maxHeight: 300, overflowY: 'auto', marginTop: 8 }}>
        {loading && <p style={{ color: 'var(--text-secondary)', fontSize: 13 }}>加载中...</p>}
        {!loading && error && <p style={{ color: '#ef4444', fontSize: 13 }}>{error}</p>}
        {!loading && !error && items.length === 0 && <p style={{ color: 'var(--text-secondary)', fontSize: 13 }}>无结果</p>}
        {!loading && items.map((r) => (
          <div key={r.id} data-testid={`rbac-avail-role-${r.id}`}
            style={{ display: 'flex', justifyContent: 'space-between', padding: '8px 0', borderBottom: '1px solid rgba(255,255,255,0.06)' }}>
            <div>
              <span style={{ fontSize: 14 }}>{r.display_name}</span>
              <span style={{ padding: '2px 8px', borderRadius: 6, fontSize: 11, marginLeft: 8,
                background: r.level === 0 ? '#ef4444' : r.level === 1 ? '#f59e0b' : '#34d399', color: '#fff' }}>L{r.level}</span>
            </div>
            <button data-testid={`rbac-user-add-role-${r.id}`} onClick={() => onAdd(r.id)} style={{ ...btnSm, color: '#5c7cfa' }} disabled={maxReached}>添加</button>
          </div>
        ))}
      </div>
      <div style={{ marginTop: 12, textAlign: 'right' }}><button onClick={onClose} style={btnSec}>关闭</button></div>
    </div></div>
  );
}
