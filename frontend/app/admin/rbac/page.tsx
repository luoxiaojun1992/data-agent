'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '../../../lib/api';

interface RBACRole {
  id: string;
  name: string;
  display_name: string;
  description: string;
  parent_id: string;
  level: number;
  type: string;
  child_count: number;
  permission_count: number;
  created_at: string;
  updated_at: string;
}

interface RBACPermission {
  id: string;
  key: string;
  name: string;
  description: string;
  module: string;
  type: string;
}

const PAGE_SIZE = 20;

export default function RBACPage() {
  const { auth, apiFetch } = useAuth();
  const [tab, setTab] = useState<'roles' | 'permissions'>('roles');
  const [roles, setRoles] = useState<RBACRole[]>([]);
  const [perms, setPerms] = useState<RBACPermission[]>([]);
  const [roleTotal, setRoleTotal] = useState(0);
  const [permTotal, setPermTotal] = useState(0);
  const [rolePage, setRolePage] = useState(1);
  const [permPage, setPermPage] = useState(1);
  const [showAddRole, setShowAddRole] = useState(false);
  const [showEditRole, setShowEditRole] = useState(false);
  const [selectedRole, setSelectedRole] = useState<RBACRole | null>(null);
  const [parentFilter, setParentFilter] = useState<string>('');
  const [parentFilterName, setParentFilterName] = useState<string>('');
  const [toast, setToast] = useState<{ msg: string; type: 'success' | 'error' } | null>(null);

  const showToast = useCallback((msg: string, type: 'success' | 'error') => {
    setToast({ msg, type });
    setTimeout(() => setToast(null), 2000);
  }, []);

  const fetchRoles = useCallback(async () => {
    if (!auth.hydrated) return;
    try {
      const qs = new URLSearchParams({ page: String(rolePage), page_size: String(PAGE_SIZE) }); if (parentFilter) qs.set('parent_id', parentFilter);
      const data = await (await apiFetch(`/rbac/roles?${qs}`)).json()
      setRoles(data.roles || []);
      setRoleTotal(data.total || 0);
    } catch { showToast('加载角色失败', 'error'); }
  }, [apiFetch, [apiFetch, rolePage, showToast], auth.hydrated]);

  const fetchPerms = useCallback(async () => {
    if (!auth.hydrated) return;
    try {
      const data = await (await apiFetch(`/rbac/permissions?page=${permPage}&page_size=${PAGE_SIZE}`)).json()
      setPerms(data.permissions || []);
      setPermTotal(data.total || 0);
    } catch { showToast('加载权限失败', 'error'); }
  }, [apiFetch, [apiFetch, permPage, showToast], auth.hydrated]);

  useEffect(() => {
    if (!auth.hydrated) return;
    if (tab === "roles") fetchRoles();
    else fetchPerms();
  }, [tab, fetchRoles, fetchPerms]);

  const deleteRole = async (id: string) => {
    if (!confirm('确定要删除该角色吗？')) return;
    try {
      await apiFetch(`/rbac/roles/${id}`, { method: 'DELETE' });
      showToast('已删除', 'success');
      fetchRoles();
    } catch { showToast('删除失败', 'error'); }
  };

  const deletePermission = async (id: string) => {
    if (!confirm('确定要删除该权限吗？')) return;
    try {
      await apiFetch(`/rbac/permissions/${id}`, { method: 'DELETE' });
      showToast('已删除', 'success');
      fetchPerms();
    } catch { showToast('删除失败', 'error'); }
  };

  return (
    <AppLayout>
      <div style={{ maxWidth: '1200px', margin: '0 auto', padding: '20px' }}>
        <h1 style={{ fontSize: '24px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '20px' }}>
          RBAC 管理{parentFilter && <span style={{ fontSize: '14px', color: '#5c7cfa', marginLeft: '12px' }}>— {parentFilterName} 的子角色</span>}
        </h1>

        {/* Tabs */}
        <div style={{ display: 'flex', gap: '4px', marginBottom: '20px', borderBottom: '1px solid var(--border)' }}>
          <button onClick={() => setTab('roles')} style={{
            ...tabBtnStyle, borderBottom: tab === 'roles' ? '2px solid #5c7cfa' : '2px solid transparent',
            color: tab === 'roles' ? '#5c7cfa' : 'var(--text-secondary)',
          }}>角色管理</button>
          <button onClick={() => setTab('permissions')} style={{
            ...tabBtnStyle, borderBottom: tab === 'permissions' ? '2px solid #5c7cfa' : '2px solid transparent',
            color: tab === 'permissions' ? '#5c7cfa' : 'var(--text-secondary)',
          }}>权限列表</button>
        </div>

        {/* Role Tab */}
        {tab === 'roles' && (
          <>
            {parentFilter && <button data-testid="rbac-clear-filter-btn" onClick={() => { setParentFilter(''); setParentFilterName(''); setRolePage(1); }}
              style={{ ...btnSecondaryStyle, marginRight: '8px' }}>← 返回全部</button>}
            <button data-testid="rbac-add-role-btn" onClick={() => setShowAddRole(true)}
              style={btnPrimaryStyle}>+ 新建角色</button>

            <div style={{ marginTop: '16px' }}>
              {roles.map((r) => (
                <div key={r.id} data-testid={`rbac-role-${r.id}`}
                  className="glass glass-hover" style={cardStyle}>
                  <div style={{ flex: 1 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '4px' }}>
                      <strong>{r.display_name}</strong>
                      <span style={badgeStyle(r.level)}>L{r.level}</span>
                      {r.type === 'builtin' && <span style={{ ...badgeStyle(-1), background: '#fbbf24', color: '#000' }}>内置</span>}
                    </div>
                    <div style={{ fontSize: '13px', color: 'var(--text-secondary)' }}>
                      {r.description || `name: ${r.name}`}
                    </div>
                    <div style={{ display: 'flex', gap: '12px', marginTop: '4px' }}>
                      <span style={countBadgeStyle}>{`子角色 ${r.child_count}/10`}</span>
                      <span style={countBadgeStyle}>{`权限 ${r.permission_count}/10`}</span>
                    </div>
                  </div>
                  <div style={{ display: 'flex', gap: '4px', alignItems: 'center' }}>
                    <a href={`/admin/rbac/roles/${r.id}/permissions`} style={{ ...btnSmallStyle, color: '#5c7cfa' }}>管理权限</a>
                    {r.level < 2 && (
                      <button data-testid={`rbac-role-sub-${r.id}`} onClick={() => { setParentFilter(r.id); setParentFilterName(r.display_name); setRolePage(1); }}
                        style={{ ...btnSmallStyle, color: '#a855f7' }}>子角色 ({r.child_count})</button>
                    )}
                    {r.type === 'custom' && (
                      <>
                        <button data-testid={`rbac-role-edit-${r.id}`} onClick={() => { setSelectedRole(r); setShowEditRole(true); }}
                          style={{ ...btnSmallStyle, color: '#f59e0b' }}>编辑</button>
                        <button data-testid={`rbac-role-delete-${r.id}`} onClick={() => deleteRole(r.id)}
                          style={{ ...btnSmallStyle, color: '#ef4444' }}>删除</button>
                      </>
                    )}
                  </div>
                </div>
              ))}
            </div>

            <Pagination page={rolePage} total={roleTotal} pageSize={PAGE_SIZE} onPage={setRolePage} />
          </>
        )}

        {/* Permission Tab */}
        {tab === 'permissions' && (
          <table className="glass" style={tableStyle}>
            <thead>
              <tr>
                <th style={thStyle}>Key</th>
                <th style={thStyle}>名称</th>
                <th style={thStyle}>模块</th>
                <th style={thStyle}>类型</th>
                <th style={thStyle}>操作</th>
              </tr>
            </thead>
            <tbody>
              {perms.map((p) => (
                <tr key={p.id} data-testid={`rbac-perm-${p.id}`} style={trStyle}>
                  <td style={tdStyle}><code>{p.key}</code></td>
                  <td style={tdStyle}>{p.name}</td>
                  <td style={tdStyle}>{p.module}</td>
                  <td style={tdStyle}>
                    <span style={{ ...badgeStyle(-1), background: p.type === 'builtin' ? '#fbbf24' : '#34d399', color: '#000' }}>{p.type}</span>
                  </td>
                  <td style={tdStyle}>
                    {p.type === 'custom' && (
                      <button data-testid={`rbac-perm-delete-${p.id}`} onClick={() => deletePermission(p.id)}
                        style={{ ...btnSmallStyle, color: '#ef4444' }}>删除</button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        {tab === 'permissions' && <Pagination page={permPage} total={permTotal} pageSize={PAGE_SIZE} onPage={setPermPage} />}

        {/* Toast */}
        {toast && (
          <div style={{ position: 'fixed', bottom: '20px', right: '20px', padding: '10px 20px', borderRadius: '8px',
            background: toast.type === 'success' ? '#34d399' : '#ef4444', color: '#fff', zIndex: 9999 }}>
            {toast.msg}
          </div>
        )}
      </div>

      {/* Add Role Modal */}
      {showAddRole && <AddRoleModal apiFetch={apiFetch} roles={roles} onClose={() => setShowAddRole(false)} onSuccess={() => { fetchRoles(); setShowAddRole(false); }} showToast={showToast} />}
      {showEditRole && selectedRole && <EditRoleModal apiFetch={apiFetch} role={selectedRole} roles={roles} onClose={() => { setShowEditRole(false); setSelectedRole(null); }} onSuccess={() => { fetchRoles(); setShowEditRole(false); setSelectedRole(null); }} showToast={showToast} />}
    </AppLayout>
  );
}

// ── Modal Components ─────────────────────────────────────────────────



function AddRoleModal({ apiFetch, roles, onClose, onSuccess, showToast }: any) {
  const [name, setName] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [description, setDescription] = useState('');
  const [parentID, setParentID] = useState('');

  const handleCreate = async () => {
    try {
      await apiFetch('/rbac/roles', {
        method: 'POST',
        body: JSON.stringify({ name, display_name: displayName, description, parent_id: parentID }),
      });
      showToast('角色已创建', 'success');
      onSuccess();
    } catch (e: any) {
      showToast(e?.message || '创建失败', 'error');
    }
  };

  return (
    <div style={modalOverlayStyle} onClick={onClose}>
      <div style={modalContentStyle} onClick={(e) => e.stopPropagation()}>
        <h3 style={{ marginBottom: '12px' }}>新建角色</h3>
        <label style={labelStyle}>名称 (name) <input style={inputStyle} value={name} onChange={(e) => setName(e.target.value)} placeholder="my_custom_role" /></label>
        <label style={labelStyle}>显示名 <input style={inputStyle} value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder="我的自定义角色" /></label>
        <label style={labelStyle}>描述 <input style={inputStyle} value={description} onChange={(e) => setDescription(e.target.value)} /></label>
        <label style={labelStyle}>父角色
          <select style={inputStyle} value={parentID} onChange={(e) => setParentID(e.target.value)}>
            <option value="">无（根角色）</option>
            {roles.filter((r: RBACRole) => r.level < 2 && r.child_count < 10).map((r: RBACRole) => (
              <option key={r.id} value={r.id}>{r.display_name} (L{r.level})</option>
            ))}
          </select>
        </label>
        <div style={{ display: 'flex', gap: '8px', marginTop: '16px', justifyContent: 'flex-end' }}>
          <button onClick={onClose} style={btnSecondaryStyle}>取消</button>
          <button data-testid="rbac-add-role-submit" onClick={handleCreate} style={btnPrimaryStyle}>创建</button>
        </div>
      </div>
    </div>
  );
}

function EditRoleModal({ apiFetch, role, roles, onClose, onSuccess, showToast }: any) {
  const [displayName, setDisplayName] = useState(role.display_name);
  const [description, setDescription] = useState(role.description || '');
  const [parentID, setParentID] = useState(role.parent_id || '');

  const handleUpdate = async () => {
    try {
      await apiFetch(`/rbac/roles/${role.id}`, {
        method: 'PUT',
        body: JSON.stringify({ display_name: displayName, description, parent_id: parentID }),
      });
      showToast('已更新', 'success');
      onSuccess();
    } catch (e: any) {
      showToast(e?.message || '更新失败', 'error');
    }
  };

  return (
    <div style={modalOverlayStyle} onClick={onClose}>
      <div style={modalContentStyle} onClick={(e) => e.stopPropagation()}>
        <h3 style={{ marginBottom: '12px' }}>编辑角色 — {role.name}</h3>
        <p style={{ fontSize: '12px', color: 'var(--text-secondary)', marginBottom: '12px' }}>
          层级 L{role.level}（不可更改） | 类型: {role.type}
        </p>
        <label style={labelStyle}>显示名 <input style={inputStyle} value={displayName} onChange={(e) => setDisplayName(e.target.value)} /></label>
        <label style={labelStyle}>描述 <input style={inputStyle} value={description} onChange={(e) => setDescription(e.target.value)} /></label>
        <label style={labelStyle}>父角色
          <select style={inputStyle} value={parentID} onChange={(e) => setParentID(e.target.value)}>
            <option value="">无</option>
            {roles.filter((r: RBACRole) => r.level === role.level - 1 && r.id !== role.id && r.child_count < 10).map((r: RBACRole) => (
              <option key={r.id} value={r.id}>{r.display_name} (L{r.level})</option>
            ))}
          </select>
        </label>
        <div style={{ display: 'flex', gap: '8px', marginTop: '16px', justifyContent: 'flex-end' }}>
          <button onClick={onClose} style={btnSecondaryStyle}>取消</button>
          <button data-testid="rbac-edit-role-submit" onClick={handleUpdate} style={btnPrimaryStyle}>保存</button>
        </div>
      </div>
    </div>
  );
}

// ── Pagination ───────────────────────────────────────────────────────

function Pagination({ page, total, pageSize, onPage }: { page: number; total: number; pageSize: number; onPage: (p: number) => void }) {
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

// ── Styles ───────────────────────────────────────────────────────────

const tabBtnStyle: React.CSSProperties = { padding: '8px 20px', background: 'transparent', border: 'none', fontSize: '15px', cursor: 'pointer', fontWeight: 500 };
const btnPrimaryStyle: React.CSSProperties = { padding: '8px 16px', background: '#5c7cfa', color: '#fff', border: 'none', borderRadius: '8px', cursor: 'pointer', fontSize: '14px' };
const btnSecondaryStyle: React.CSSProperties = { padding: '8px 16px', background: 'transparent', color: 'var(--text-secondary)', border: '1px solid var(--border)', borderRadius: '8px', cursor: 'pointer', fontSize: '14px' };
const btnSmallStyle: React.CSSProperties = { padding: '4px 10px', background: 'transparent', border: '1px solid var(--border)', borderRadius: '6px', cursor: 'pointer', fontSize: '12px' };
const cardStyle: React.CSSProperties = { display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 16px', marginBottom: '8px', border: '1px solid var(--border)', borderRadius: '10px', background: 'var(--card-bg)' };
const badgeStyle = (level: number): React.CSSProperties => ({ padding: '2px 8px', borderRadius: '6px', fontSize: '11px', fontWeight: 500, background: level === 0 ? '#ef4444' : level === 1 ? '#f59e0b' : '#34d399', color: '#fff' });
const countBadgeStyle: React.CSSProperties = { fontSize: '12px', color: 'var(--text-secondary)', background: 'rgba(92, 124, 250, 0.1)', padding: '2px 8px', borderRadius: '4px' };
const tableStyle: React.CSSProperties = { width: '100%', borderCollapse: 'collapse' };
const thStyle: React.CSSProperties = { padding: '10px 12px', textAlign: 'left', borderBottom: '1px solid var(--border)', fontSize: '13px', fontWeight: 600, color: 'var(--text-secondary)' };
const tdStyle: React.CSSProperties = { padding: '10px 12px', borderBottom: '1px solid var(--border)', fontSize: '14px' };
const trStyle: React.CSSProperties = { borderBottom: '1px solid var(--border-eo)' };
const modalOverlayStyle: React.CSSProperties = { position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 9999 };
const modalContentStyle: React.CSSProperties = { background: 'var(--card-bg)', padding: '24px', borderRadius: '12px', minWidth: '400px', maxWidth: '500px' };
const labelStyle: React.CSSProperties = { display: 'block', fontSize: '13px', marginBottom: '8px', color: 'var(--text-secondary)' };
const inputStyle: React.CSSProperties = { display: 'block', width: '100%', marginTop: '4px', padding: '8px', borderRadius: '6px', border: '1px solid var(--border)', background: 'var(--input-bg)', color: 'var(--text-primary)', fontSize: '14px' };
const pageBtnStyle: React.CSSProperties = { padding: '4px 10px', border: '1px solid var(--border)', borderRadius: '6px', cursor: 'pointer', fontSize: '13px', background: 'transparent', color: 'var(--text-primary)' };
