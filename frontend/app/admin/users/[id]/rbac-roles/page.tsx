'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useParams } from 'next/navigation';
import AppLayout from '../../../../providers';
import { useAuth } from '../../../../../lib/api';

interface RBACRole {
  id: string;
  name: string;
  display_name: string;
  level: number;
  type: string;
  child_count: number;
  permission_count: number;
}

const PAGE_SIZE = 20;

export default function UserRBACRolesPage() {
  const { auth, apiFetch } = useAuth();
  const { id } = useParams<{ id: string }>();
  const [roles, setRoles] = useState<RBACRole[]>([]);
  const [allRoles, setAllRoles] = useState<RBACRole[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');
  const [showAdd, setShowAdd] = useState(false);
  const [toast, setToast] = useState<{ msg: string; type: string } | null>(null);

  const showToast = useCallback((msg: string, type: string) => {
    setToast({ msg, type });
    setTimeout(() => setToast(null), 2000);
  }, []);

  const fetchRoles = useCallback(async () => {
    try {
      const data = await apiFetch(`/admin/users/${id}/rbac-roles?page=${page}&page_size=${PAGE_SIZE}`);
      setRoles(data.roles || []);
      setTotal(data.total || 0);
    } catch { showToast('加载角色失败', 'error'); }
  }, [apiFetch, id, page, showToast]);

  const fetchAllRoles = useCallback(async () => {
    try {
      const data = await apiFetch('/rbac/roles?page=1&page_size=200');
      setAllRoles(data.roles || []);
    } catch {}
  }, [apiFetch]);

  useEffect(() => { fetchRoles(); }, [fetchRoles]);

  const availableRoles = allRoles.filter((r) =>
    !roles.find((rr) => rr.id === r.id) &&
    (search === '' || r.name.includes(search) || r.display_name.includes(search))
  );

  const addRole = async (roleID: string) => {
    try {
      await apiFetch(`/admin/users/${id}/rbac-roles`, {
        method: 'POST', body: JSON.stringify({ role_id: roleID }),
      });
      showToast('已添加', 'success');
      fetchRoles();
    } catch { showToast('添加失败', 'error'); }
  };

  const removeRole = async (roleID: string) => {
    try {
      await apiFetch(`/admin/users/${id}/rbac-roles/${roleID}`, { method: 'DELETE' });
      showToast('已移除', 'success');
      fetchRoles();
    } catch { showToast('移除失败', 'error'); }
  };

  return (
    <AppLayout username={auth?.username || ''} role={auth?.role || ''} onLogout={() => {}}>
      <div style={{ maxWidth: '700px', margin: '0 auto', padding: '20px' }}>
        <div style={{ marginBottom: '20px' }}>
          <a href="/admin/users" style={{ color: '#5c7cfa', fontSize: '13px' }}>← 返回用户管理</a>
          <h2 style={{ fontSize: '20px', fontWeight: 600, margin: '8px 0' }}>RBAC 角色管理</h2>
        </div>

        <div style={{ display: 'flex', gap: '8px', marginBottom: '16px' }}>
          <button data-testid="rbac-user-add-role-btn" onClick={() => { setShowAdd(true); fetchAllRoles(); }}
            style={btnPrimaryStyle}>+ 添加角色（{roles.length}/10）</button>
        </div>

        {roles.length === 0 ? (
          <p style={{ color: 'var(--text-secondary)', fontSize: '14px' }}>暂无关联角色</p>
        ) : (
          <div>
            {roles.map((r) => (
              <div key={r.id} style={cardStyle}>
                <div style={{ flex: 1 }}>
                  <strong>{r.display_name}</strong>
                  <span style={{ ...badgeStyle(r.level), marginLeft: '8px' }}>L{r.level}</span>
                  <span style={{ fontSize: '12px', color: 'var(--text-secondary)', marginLeft: '8px' }}>{r.name}</span>
                </div>
                <button data-testid={`rbac-user-role-remove-${r.id}`} onClick={() => removeRole(r.id)}
                  style={{ ...btnSmallStyle, color: '#ef4444' }}>移除</button>
              </div>
            ))}
          </div>
        )}
        {total > PAGE_SIZE && <Pagination page={page} total={total} pageSize={PAGE_SIZE} onPage={setPage} />}
      </div>

      {showAdd && (
        <div style={modalOverlayStyle} onClick={() => setShowAdd(false)}>
          <div style={modalContentStyle} onClick={(e) => e.stopPropagation()}>
            <h3 style={{ marginBottom: '12px' }}>添加角色</h3>
            {roles.length >= 10 && <p style={{ color: '#ef4444', fontSize: '13px', marginBottom: '8px' }}>已达到上限 10 个</p>}
            <input style={inputStyle} placeholder="搜索角色..." value={search}
              onChange={(e) => setSearch(e.target.value)} />
            <div style={{ maxHeight: '300px', overflowY: 'auto', marginTop: '8px' }}>
              {availableRoles.map((r) => (
                <div key={r.id} style={{ display: 'flex', justifyContent: 'space-between', padding: '8px 0', borderBottom: '1px solid var(--border-eo)' }}>
                  <div>
                    <span style={{ fontSize: '14px' }}>{r.display_name}</span>
                    <span style={{ ...badgeStyle(r.level), marginLeft: '8px' }}>L{r.level}</span>
                  </div>
                  <button data-testid={`rbac-avail-role-${r.id}`}
                    onClick={() => addRole(r.id)} style={{ ...btnSmallStyle, color: '#5c7cfa' }}
                    disabled={roles.length >= 10}>添加</button>
                </div>
              ))}
            </div>
            <div style={{ marginTop: '12px', textAlign: 'right' }}>
              <button onClick={() => setShowAdd(false)} style={btnSecondaryStyle}>关闭</button>
            </div>
          </div>
        </div>
      )}

      {toast && (
        <div style={{ position: 'fixed', bottom: '20px', right: '20px', padding: '10px 20px', borderRadius: '8px',
          background: toast.type === 'success' ? '#34d399' : '#ef4444', color: '#fff', zIndex: 9999 }}>
          {toast.msg}
        </div>
      )}
    </AppLayout>
  );
}

function Pagination({ page, total, pageSize, onPage }: any) {
  const totalPages = Math.ceil(total / pageSize);
  if (totalPages <= 1) return null;
  return (
    <div style={{ display: 'flex', gap: '4px', justifyContent: 'center', marginTop: '16px' }}>
      <button disabled={page <= 1} onClick={() => onPage(page - 1)} style={pageBtnStyle}>‹</button>
      {Array.from({ length: totalPages }, (_, i) => i + 1).map((p) => (
        <button key={p} onClick={() => onPage(p)} style={{ ...pageBtnStyle, background: p === page ? '#5c7cfa' : 'transparent', color: p === page ? '#fff' : 'var(--text-primary)' }}>{p}</button>
      ))}
      <button disabled={page >= totalPages} onClick={() => onPage(page + 1)} style={pageBtnStyle}>›</button>
    </div>
  );
}

const badgeStyle = (level: number): React.CSSProperties => ({ padding: '2px 8px', borderRadius: '6px', fontSize: '11px', fontWeight: 500, background: level === 0 ? '#ef4444' : level === 1 ? '#f59e0b' : '#34d399', color: '#fff', display: 'inline-block' });
const btnPrimaryStyle: React.CSSProperties = { padding: '8px 16px', background: '#5c7cfa', color: '#fff', border: 'none', borderRadius: '8px', cursor: 'pointer', fontSize: '14px' };
const btnSecondaryStyle: React.CSSProperties = { padding: '8px 16px', background: 'transparent', color: 'var(--text-secondary)', border: '1px solid var(--border)', borderRadius: '8px', cursor: 'pointer', fontSize: '14px' };
const btnSmallStyle: React.CSSProperties = { padding: '4px 10px', background: 'transparent', border: '1px solid var(--border)', borderRadius: '6px', cursor: 'pointer', fontSize: '12px' };
const cardStyle: React.CSSProperties = { display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 16px', marginBottom: '8px', border: '1px solid var(--border)', borderRadius: '10px', background: 'var(--card-bg)' };
const modalOverlayStyle: React.CSSProperties = { position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 9999 };
const modalContentStyle: React.CSSProperties = { background: 'var(--card-bg)', padding: '24px', borderRadius: '12px', minWidth: '400px' };
const inputStyle: React.CSSProperties = { width: '100%', padding: '8px', borderRadius: '6px', border: '1px solid var(--border)', background: 'var(--input-bg)', color: 'var(--text-primary)', fontSize: '14px', boxSizing: 'border-box' };
const pageBtnStyle: React.CSSProperties = { padding: '4px 10px', border: '1px solid var(--border)', borderRadius: '6px', cursor: 'pointer', fontSize: '13px', background: 'transparent', color: 'var(--text-primary)' };
