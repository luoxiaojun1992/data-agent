'use client';

import React, { useState, useEffect } from 'react';
import { useParams } from 'next/navigation';
import AppLayout from '../../../../../providers';
import { useAuth } from '../../../../../../lib/api';
import { useDebouncedSearch, SearchableOption } from '../../../../../components/SearchableSelect';
import { primaryButtonStyle, modalOverlayStyle, modalPanelStyle, modalInputStyle } from '../../../../../components/ui';

interface RBACPermission { id: string; key: string; name: string; module: string; type: string; }

const PAGE_SIZE = 10;

export default function RolePermissionsPage() {
  const { auth, apiFetch } = useAuth();
  const { id } = useParams<{ id: string }>();
  const [perms, setPerms] = useState<RBACPermission[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [roleName, setRoleName] = useState('');
  const [level, setLevel] = useState(0);
  const [showAdd, setShowAdd] = useState(false);
  const [toast, setToast] = useState('');

  const showToast = (msg: string) => { setToast(msg); setTimeout(() => setToast(''), 2000); };

  const fetchPerms = () => {
    apiFetch(`/admin/rbac/roles/${id}/permissions?page=${page}&page_size=${PAGE_SIZE}`).then(r => r.json()).then(data => {
      setPerms(data.permissions || []); setTotal(data.total || 0);
    }).catch(() => showToast('加载失败'));
  };

  useEffect(() => {
    if (!auth.hydrated) return;
    apiFetch(`/admin/rbac/roles/${id}`).then(r => r.json()).then(data => {
      setRoleName(data.role?.display_name || ''); setLevel(data.role?.level || 0);
    });
    fetchPerms();
  }, [id, page, auth.hydrated]);

  const add = (pid: string) => {
    apiFetch(`/admin/rbac/roles/${id}/permissions`, { method: 'POST', body: JSON.stringify({ permission_id: pid }) })
      .then(() => { showToast('已添加'); fetchPerms(); }).catch(() => showToast('添加失败'));
  };
  const remove = (pid: string) => {
    apiFetch(`/admin/rbac/roles/${id}/permissions/${pid}`, { method: 'DELETE' })
      .then(() => { showToast('已移除'); fetchPerms(); }).catch(() => showToast('移除失败'));
  };

  const badge = (l: number) => ({
    padding: '2px 8px', borderRadius: 6, fontSize: 11, fontWeight: 500,
    background: l === 0 ? '#ef4444' : l === 1 ? '#f59e0b' : '#34d399', color: '#fff', marginLeft: 8,
  });
  const btnSm: React.CSSProperties = { padding: '4px 10px', background: 'transparent', border: '1px solid var(--border)', borderRadius: 6, cursor: 'pointer', fontSize: 12 };

  const pagination = () => {
    const tp = Math.max(1, Math.ceil(total / PAGE_SIZE));
    return (<div style={{ display: 'flex', gap: 4, justifyContent: 'center', marginTop: 16 }}>
      <button disabled={page <= 1} onClick={() => setPage(page - 1)} style={pgBtn}>‹</button>
      {Array.from({ length: tp }, (_, i) => i + 1).map(p => (
        <button key={p} onClick={() => setPage(p)} style={{ ...pgBtn, background: p === page ? '#5c7cfa' : 'transparent', color: p === page ? '#fff' : 'var(--text-primary)' }}>{p}</button>
      ))}
      <button disabled={page >= tp} onClick={() => setPage(page + 1)} style={pgBtn}>›</button>
    </div>);
  };
  const pgBtn: React.CSSProperties = { padding: '4px 10px', border: '1px solid var(--border)', borderRadius: 6, cursor: 'pointer', fontSize: 13, background: 'transparent', color: 'var(--text-primary)' };

  return (
    <AppLayout>
      <div style={{ maxWidth: 900, margin: '0 auto', padding: 20 }}>
        <a href="/admin/rbac" style={{ color: '#5c7cfa', fontSize: 13 }}>← 返回 RBAC 管理</a>
        <h2 style={{ fontSize: 20, fontWeight: 600, margin: '8px 0' }}>{roleName}<span style={badge(level)}>L{level}</span></h2>
        <p style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 16 }}>已关联 {total} 个权限</p>
        <button data-testid="rbac-role-add-perm-btn" onClick={() => setShowAdd(true)} style={{ ...primaryButtonStyle, marginBottom: 16 }}>+ 添加权限</button>

        <div className="glass" style={{ padding: 0, overflowX: 'auto' }}>
          <table style={{ width: '100%', fontSize: 13, borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid rgba(255,255,255,0.1)' }}>
                <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>Key</th>
                <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>名称</th>
                <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>模块</th>
                <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>操作</th>
              </tr>
            </thead>
            <tbody>
              {perms.map(p => (
                <tr key={p.id} data-testid={`rbac-role-perm-${p.id}`} style={{ borderBottom: '1px solid rgba(255,255,255,0.06)' }}>
                  <td style={{ padding: '10px 12px', fontSize: 13 }}><code style={{ fontSize: 12 }}>{p.key}</code></td>
                  <td style={{ padding: '10px 12px', fontSize: 13 }}>{p.name}</td>
                  <td style={{ padding: '10px 12px', fontSize: 13 }}>{p.module}</td>
                  <td style={{ padding: '10px 12px', fontSize: 13 }}>
                    {p.type !== "builtin" && <button data-testid={`rbac-role-perm-remove-${p.id}`} onClick={() => remove(p.id)} style={{ ...btnSm, color: '#ef4444' }}>移除</button>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {pagination()}

        {showAdd && (
          <AddPermModal apiFetch={apiFetch} roleId={id} onAdd={add} onClose={() => setShowAdd(false)} />
        )}

        {toast && <div style={{ position: 'fixed', bottom: 20, right: 20, padding: '10px 20px', borderRadius: 8,
          background: toast.includes('失败') ? '#ef4444' : '#34d399', color: '#fff', zIndex: 9999 }}>{toast}</div>}
      </div>
    </AppLayout>
  );
}

// AddPermModal loads available permissions straight from the backend (SPEC-074):
// /admin/rbac/permissions?limit=20&exclude_role_id=<id> (default) and appends
// &q=<kw> for debounced search. The client only renders the returned top-N;
// no local filter/sort/slice.
function AddPermModal({ apiFetch, roleId, onAdd, onClose }: {
  apiFetch: (path: string, options?: RequestInit) => Promise<Response>;
  roleId: string;
  onAdd: (permId: string) => void;
  onClose: () => void;
}) {
  const fetchAvail = async (q: string, limit: number): Promise<SearchableOption[]> => {
    const res = await apiFetch(`/admin/rbac/permissions?limit=${limit}&exclude_role_id=${roleId}${q ? `&q=${encodeURIComponent(q)}` : ''}`);
    if (!res.ok) throw new Error('加载失败');
    const data = await res.json();
    return (data.permissions || []) as SearchableOption[];
  };
  const { items, loading, error, query, onSearch } = useDebouncedSearch(fetchAvail, 20);

  const mo: React.CSSProperties = { ...modalOverlayStyle, zIndex: 9999 };
  const mc: React.CSSProperties = { ...modalPanelStyle, minWidth: 450 };
  const inp: React.CSSProperties = { ...modalInputStyle };
  const btnSm: React.CSSProperties = { padding: '4px 10px', background: 'transparent', border: '1px solid var(--border)', borderRadius: 6, cursor: 'pointer', fontSize: 12 };
  const btnSec: React.CSSProperties = { padding: '8px 16px', background: 'transparent', color: 'var(--text-secondary)', border: '1px solid var(--border)', borderRadius: 8, cursor: 'pointer', fontSize: 14 };

  return (
    <div style={mo} onClick={onClose}><div style={mc} onClick={e => e.stopPropagation()}>
      <h3 style={{ marginBottom: 12 }}>添加权限</h3>
      <input style={inp} placeholder="搜索..." value={query} onChange={e => onSearch(e.target.value)} />
      <div style={{ maxHeight: 300, overflowY: 'auto', marginTop: 8 }}>
        {loading && <p style={{ color: 'var(--text-secondary)', fontSize: 13 }}>加载中...</p>}
        {!loading && error && <p style={{ color: '#ef4444', fontSize: 13 }}>{error}</p>}
        {!loading && !error && items.length === 0 && <p style={{ color: 'var(--text-secondary)', fontSize: 13 }}>无结果</p>}
        {!loading && items.map((p) => (
          <div key={p.id} style={{ display: 'flex', justifyContent: 'space-between', padding: '8px 0', borderBottom: '1px solid rgba(255,255,255,0.06)' }}>
            <div><code style={{ fontSize: 12 }}>{p.key}</code><span style={{ fontSize: 13, marginLeft: 8 }}>{p.name}</span></div>
            <button onClick={() => onAdd(p.id)} style={{ ...btnSm, color: '#5c7cfa' }}>添加</button>
          </div>
        ))}
      </div>
      <div style={{ marginTop: 12, textAlign: 'right' }}><button onClick={onClose} style={btnSec}>关闭</button></div>
    </div></div>
  );
}
