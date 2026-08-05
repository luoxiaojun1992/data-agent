'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useParams } from 'next/navigation';
import AppLayout from '../../../../../providers';
import { useAuth } from '../../../../../../lib/api';

interface RBACPermission {
  id: string;
  key: string;
  name: string;
  description: string;
  module: string;
  type: string;
}

const PAGE_SIZE = 20;

export default function RolePermissionsPage() {
  const { auth, apiFetch } = useAuth();
  const { id } = useParams<{ id: string }>();
  const [perms, setPerms] = useState<RBACPermission[]>([]);
  const [allPerms, setAllPerms] = useState<RBACPermission[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');
  const [showAdd, setShowAdd] = useState(false);
  const [roleName, setRoleName] = useState('');
  const [level, setLevel] = useState(0);
  const [toast, setToast] = useState<{ msg: string; type: string } | null>(null);

  const showToast = useCallback((msg: string, type: string) => {
    setToast({ msg, type });
    setTimeout(() => setToast(null), 2000);
  }, []);

  const fetchRole = useCallback(async () => {
    if (!auth.hydrated) return;
    try {
      const data = await (await apiFetch(`/rbac/roles/${id}`)).json()
      setRoleName(data.role?.display_name || '');
      setLevel(data.role?.level || 0);
    } catch {}
  }, [apiFetch, id]);

  const fetchPerms = useCallback(async () => {
    if (!auth.hydrated) return;
    try {
      const data = await (await apiFetch(`/rbac/roles/${id}/permissions?page=${page}&page_size=${PAGE_SIZE}`)).json()
      setPerms(data.permissions || []);
      setTotal(data.total || 0);
    } catch { showToast('加载权限失败', 'error'); }
  }, [apiFetch, id, page, showToast]);

  const fetchAllPerms = useCallback(async () => {
    if (!auth.hydrated) return;
    try {
      const data = await (await apiFetch('/rbac/permissions?page=1&page_size=200')).json()
      setAllPerms(data.permissions || []);
    } catch {}
  }, [apiFetch]);

  useEffect(() => { fetchRole(); fetchPerms(); }, [fetchRole, fetchPerms]);

  const availablePerms = allPerms.filter((p) =>
    !perms.find((pp) => pp.id === p.id) &&
    (search === '' || p.key.includes(search) || p.name.includes(search) || p.module.includes(search))
  );

  const addPermission = async (permID: string) => {
    try {
      await apiFetch(`/rbac/roles/${id}/permissions`, {
        method: 'POST', body: JSON.stringify({ permission_id: permID }),
      });
      showToast('已添加', 'success');
      fetchPerms();
    } catch { showToast('添加失败', 'error'); }
  };

  const removePermission = async (permID: string) => {
    try {
      await apiFetch(`/rbac/roles/${id}/permissions/${permID}`, { method: 'DELETE' });
      showToast('已移除', 'success');
      fetchPerms();
    } catch { showToast('移除失败', 'error'); }
  };

  return (
    <AppLayout>
      <div style={{ maxWidth: '900px', margin: '0 auto', padding: '20px' }}>
        <div style={{ marginBottom: '20px' }}>
          <a href="/admin/rbac" style={{ color: '#5c7cfa', fontSize: '13px' }}>← 返回 RBAC 管理</a>
          <h2 style={{ fontSize: '20px', fontWeight: 600, margin: '8px 0' }}>
            {roleName} <span style={{
              ...badgeStyle(level), fontSize: '13px', padding: '3px 10px', marginLeft: '8px',
            }}>L{level}</span>
          </h2>
          <p style={{ fontSize: '13px', color: 'var(--text-secondary)' }}>已关联 {total} 个权限</p>
        </div>

        <div style={{ display: 'flex', gap: '8px', marginBottom: '16px' }}>
          <button data-testid="rbac-role-add-perm-btn" onClick={() => { setShowAdd(true); fetchAllPerms(); }} style={btnPrimaryStyle}>
            + 添加权限
          </button>
        </div>

        <table style={tableStyle}>
          <thead>
            <tr>
              <th style={thStyle}>Key</th><th style={thStyle}>名称</th><th style={thStyle}>模块</th><th style={thStyle}>操作</th>
            </tr>
          </thead>
          <tbody>
            {perms.map((p) => (
              <tr key={p.id} data-testid={`rbac-role-perm-${p.id}`} style={trStyle}>
                <td style={tdStyle}><code style={{ fontSize: '12px' }}>{p.key}</code></td>
                <td style={tdStyle}>{p.name}</td>
                <td style={tdStyle}>{p.module}</td>
                <td style={tdStyle}>
                  <button data-testid={`rbac-role-perm-remove-${p.id}`} onClick={() => removePermission(p.id)}
                    style={{ ...btnSmallStyle, color: '#ef4444' }}>移除</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>

        {total > PAGE_SIZE && <Pagination page={page} total={total} pageSize={PAGE_SIZE} onPage={setPage} />}
      </div>

      {/* Add Permission Modal */}
      {showAdd && (
        <div style={modalOverlayStyle} onClick={() => setShowAdd(false)}>
          <div style={modalContentStyle} onClick={(e) => e.stopPropagation()}>
            <h3 style={{ marginBottom: '12px' }}>添加权限</h3>
            <input style={inputStyle} placeholder="搜索权限 key / 名称 / 模块..." value={search}
              onChange={(e) => setSearch(e.target.value)} />
            <div style={{ maxHeight: '300px', overflowY: 'auto', marginTop: '8px' }}>
              {availablePerms.map((p) => (
                <div key={p.id} data-testid={`rbac-avail-perm-${p.id}`}
                  style={{ display: 'flex', justifyContent: 'space-between', padding: '8px 0', borderBottom: '1px solid var(--border-eo)' }}>
                  <div>
                    <code style={{ fontSize: '12px' }}>{p.key}</code>
                    <span style={{ fontSize: '13px', marginLeft: '8px' }}>{p.name}</span>
                  </div>
                  <button onClick={() => addPermission(p.id)} style={{ ...btnSmallStyle, color: '#5c7cfa' }}>添加</button>
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
        <div style={{ position: 'fixed', bottom: '20px', right: '20px', padding: '10px 20px',
          borderRadius: '8px', background: toast.type === 'success' ? '#34d399' : '#ef4444', color: '#fff', zIndex: 9999 }}>
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

const badgeStyle = (level: number): React.CSSProperties => ({ padding: '2px 8px', borderRadius: '6px', fontSize: '11px', fontWeight: 500, background: level === 0 ? '#ef4444' : level === 1 ? '#f59e0b' : '#34d399', color: '#fff' });
const btnPrimaryStyle: React.CSSProperties = { padding: '8px 16px', background: '#5c7cfa', color: '#fff', border: 'none', borderRadius: '8px', cursor: 'pointer', fontSize: '14px' };
const btnSecondaryStyle: React.CSSProperties = { padding: '8px 16px', background: 'transparent', color: 'var(--text-secondary)', border: '1px solid var(--border)', borderRadius: '8px', cursor: 'pointer', fontSize: '14px' };
const btnSmallStyle: React.CSSProperties = { padding: '4px 10px', background: 'transparent', border: '1px solid var(--border)', borderRadius: '6px', cursor: 'pointer', fontSize: '12px' };
const tableStyle: React.CSSProperties = { width: '100%', borderCollapse: 'collapse' as const };
const thStyle: React.CSSProperties = { padding: '10px 12px', textAlign: 'left', borderBottom: '1px solid var(--border)', fontSize: '13px', fontWeight: 600, color: 'var(--text-secondary)' };
const tdStyle: React.CSSProperties = { padding: '10px 12px', borderBottom: '1px solid var(--border)', fontSize: '14px' };
const trStyle: React.CSSProperties = { borderBottom: '1px solid var(--border-eo)' };
const modalOverlayStyle: React.CSSProperties = { position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 9999 };
const modalContentStyle: React.CSSProperties = { background: 'var(--card-bg)', padding: '24px', borderRadius: '12px', minWidth: '450px' };
const inputStyle: React.CSSProperties = { width: '100%', padding: '8px', borderRadius: '6px', border: '1px solid var(--border)', background: 'var(--input-bg)', color: 'var(--text-primary)', fontSize: '14px', boxSizing: 'border-box' };
const pageBtnStyle: React.CSSProperties = { padding: '4px 10px', border: '1px solid var(--border)', borderRadius: '6px', cursor: 'pointer', fontSize: '13px', background: 'transparent', color: 'var(--text-primary)' };
