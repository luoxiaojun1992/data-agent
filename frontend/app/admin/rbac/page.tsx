'use client';

import React, { useState, useEffect } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '../../../lib/api';

interface RBACRole {
  id: string; name: string; display_name: string; description: string;
  parent_id: string; level: number; type: string; child_count: number; permission_count: number;
  created_at: string; updated_at: string;
}
interface RBACPermission {
  id: string; key: string; name: string; description: string; module: string; type: string;
}

const PAGE_SIZE = 10;

export default function RBACPage() {
  const { auth, apiFetch } = useAuth();
  const [tab, setTab] = useState<'roles' | 'permissions'>('roles');
  const [roles, setRoles] = useState<RBACRole[]>([]);
  const [perms, setPerms] = useState<RBACPermission[]>([]);
  const [roleTotal, setRoleTotal] = useState(0);
  const [permTotal, setPermTotal] = useState(0);
  const [rolePage, setRolePage] = useState(1);
  const [permPage, setPermPage] = useState(1);
  const [parentFilter, setParentFilter] = useState('');
  const [parentFilterName, setParentFilterName] = useState('');
  const [selectedRole, setSelectedRole] = useState<RBACRole | null>(null);
  const [showAddRole, setShowAddRole] = useState(false);
  const [showAddPerm, setShowAddPerm] = useState(false);
  const [showEditRole, setShowEditRole] = useState(false);
  const [toast, setToast] = useState('');

  const showToast = (msg: string) => { setToast(msg); setTimeout(() => setToast(''), 2000); };

  const fetchRoles = () => {
    const q = new URLSearchParams({ page: String(rolePage), page_size: String(PAGE_SIZE) });
    if (parentFilter) q.set('parent_id', parentFilter);
    apiFetch(`/admin/rbac/roles?${q}`).then(r => r.json()).then(data => {
      setRoles(data.roles || []); setRoleTotal(data.total || 0);
    }).catch(() => showToast('加载角色失败'));
  };

  const fetchPerms = () => {
    apiFetch(`/admin/rbac/permissions?page=${permPage}&page_size=${PAGE_SIZE}`).then(r => r.json()).then(data => {
      setPerms(data.permissions || []); setPermTotal(data.total || 0);
    }).catch(() => showToast('加载权限失败'));
  };

  useEffect(() => { if (auth.hydrated && tab === 'roles') fetchRoles(); }, [tab, rolePage, parentFilter, auth.hydrated]);
  useEffect(() => { if (auth.hydrated && tab === 'permissions') fetchPerms(); }, [tab, permPage, auth.hydrated]);

  const deleteRole = async (id: string) => {
    if (!confirm('确定删除？')) return;
    try { await apiFetch(`/admin/rbac/roles/${id}`, { method: 'DELETE' }); showToast('已删除'); fetchRoles(); }
    catch { showToast('删除失败'); }
  };
  const deletePerm = async (id: string) => {
    if (!confirm('确定删除？')) return;
    try { await apiFetch(`/admin/rbac/permissions/${id}`, { method: 'DELETE' }); showToast('已删除'); fetchPerms(); }
    catch { showToast('删除失败'); }
  };

  const clearFilter = () => { setParentFilter(''); setParentFilterName(''); setRolePage(1); };

  return (
    <AppLayout>
      <div style={{ maxWidth: 1200, margin: '0 auto', padding: 20 }}>
        <h1 style={{ fontSize: 24, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 20 }}>
          RBAC 管理{parentFilter && <span style={{ fontSize: 14, color: '#5c7cfa', marginLeft: 12 }}>— {parentFilterName} 的子角色</span>}
        </h1>

        <div style={{ display: 'flex', gap: 4, marginBottom: 20, borderBottom: '1px solid var(--border)' }}>
          <button onClick={() => setTab('roles')}
            style={{ padding: '8px 20px', background: 'transparent', border: 'none', fontSize: 15, cursor: 'pointer', fontWeight: 500,
              borderBottom: tab === 'roles' ? '2px solid #5c7cfa' : '2px solid transparent',
              color: tab === 'roles' ? '#5c7cfa' : 'var(--text-secondary)' }}>角色管理</button>
          <button onClick={() => setTab('permissions')}
            style={{ padding: '8px 20px', background: 'transparent', border: 'none', fontSize: 15, cursor: 'pointer', fontWeight: 500,
              borderBottom: tab === 'permissions' ? '2px solid #5c7cfa' : '2px solid transparent',
              color: tab === 'permissions' ? '#5c7cfa' : 'var(--text-secondary)' }}>权限列表</button>
        </div>

        {/* Roles Tab */}
        {tab === 'roles' && (<>
          <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
            {parentFilter && <button onClick={clearFilter} data-testid="rbac-clear-filter-btn"
              style={btnSec}>← 返回全部</button>}
            <button data-testid="rbac-add-role-btn" onClick={() => setShowAddRole(true)} style={btnPri}>+ 新建角色</button>
          </div>

          {roles.map(r => (
            <div key={r.id} data-testid={`rbac-role-${r.id}`} className="glass glass-hover"
              style={{ padding: '14px 16px', marginBottom: 8, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <div style={{ flex: 1 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                  <strong>{r.display_name}</strong>
                  <span style={{ padding: '2px 8px', borderRadius: 6, fontSize: 11, fontWeight: 500,
                    background: r.level === 0 ? '#ef4444' : r.level === 1 ? '#f59e0b' : '#34d399', color: '#fff' }}>L{r.level}</span>
                  {r.type === 'builtin' && <span style={{ padding: '2px 8px', borderRadius: 6, fontSize: 11, background: '#fbbf24', color: '#000' }}>内置</span>}
                </div>
                <div style={{ fontSize: 13, color: 'var(--text-secondary)' }}>{r.description || r.name}</div>
                <div style={{ display: 'flex', gap: 12, marginTop: 4, fontSize: 12, color: 'var(--text-secondary)' }}>
                  <span>子角色 {r.child_count}/10</span><span>权限 {r.permission_count}/10</span>
                </div>
              </div>
              <div style={{ display: 'flex', gap: 4, alignItems: 'center' }}>
                <a href={`/admin/rbac/roles/${r.id}/permissions`} style={{ ...btnSm, color: '#5c7cfa', textDecoration: 'none' }}>管理权限</a>
                {r.level < 2 && <button data-testid={`rbac-role-sub-${r.id}`}
                  onClick={() => { setParentFilter(r.id); setParentFilterName(r.display_name); setRolePage(1); }}
                  style={{ ...btnSm, color: '#a855f7' }}>子角色 ({r.child_count})</button>}
                {r.type === 'custom' && (<>
                  <button data-testid={`rbac-role-edit-${r.id}`} onClick={() => { setSelectedRole(r); setShowEditRole(true); }}
                    style={{ ...btnSm, color: '#f59e0b' }}>编辑</button>
                  <button data-testid={`rbac-role-delete-${r.id}`} onClick={() => deleteRole(r.id)}
                    style={{ ...btnSm, color: '#ef4444' }}>删除</button>
                </>)}
              </div>
            </div>
          ))}
          <Pagination page={rolePage} total={roleTotal} pageSize={PAGE_SIZE} onPage={setRolePage} />
        </>)}

        {/* Permissions Tab */}
        {tab === 'permissions' && (<>
          <div className="glass" style={{ padding: 0, overflowX: 'auto' }}>
            <table style={{ width: '100%', fontSize: 13, borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid rgba(255,255,255,0.1)' }}>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>名称</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>模块</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>Key</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>类型</th>
                  <th style={{ textAlign: 'left', padding: '10px 12px', color: 'var(--text-secondary)', fontWeight: 500 }}>操作</th>
                </tr>
              </thead>
              <tbody>
                {perms.map(p => (
                  <tr key={p.id} data-testid={`rbac-perm-${p.id}`} style={{ borderBottom: '1px solid rgba(255,255,255,0.06)' }}>
                    <td style={{ padding: '10px 12px', fontSize: 13 }}>{p.name}</td>
                    <td style={{ padding: '10px 12px', fontSize: 13 }}>{p.module}</td>
                    <td style={{ padding: '10px 12px', fontSize: 13 }}><code style={{ fontSize: 12 }}>{p.key}</code></td>
                    <td style={{ padding: '10px 12px', fontSize: 13 }}>
                      <span style={{ padding: '2px 8px', borderRadius: 6, fontSize: 11,
                        background: p.type === 'builtin' ? '#fbbf24' : '#34d399', color: '#000' }}>{p.type}</span></td>
                    <td style={{ padding: '10px 12px', fontSize: 13 }}>
                      {p.type === 'custom' && <button data-testid={`rbac-perm-delete-${p.id}`} onClick={() => deletePerm(p.id)}
                        style={{ ...btnSm, color: '#ef4444' }}>删除</button>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <button data-testid="rbac-add-perm-btn" onClick={() => setShowAddPerm(true)} style={btnPri}>+ 新建权限</button>
          <Pagination page={permPage} total={permTotal} pageSize={PAGE_SIZE} onPage={setPermPage} />
        </>)}

        {toast && <div style={{ position: 'fixed', bottom: 20, right: 20, padding: '10px 20px', borderRadius: 8,
          background: toast.includes('失败') ? '#ef4444' : '#34d399', color: '#fff', zIndex: 9999 }}>{toast}</div>}

        {showAddPerm && <AddPermModal apiFetch={apiFetch} onClose={() => setShowAddPerm(false)} onSuccess={() => { setShowAddPerm(false); fetchPerms(); }} showToast={showToast} />}
        {showAddRole && <AddRoleModal apiFetch={apiFetch} roles={roles} onClose={() => setShowAddRole(false)}
          onSuccess={() => { setShowAddRole(false); fetchRoles(); }} showToast={showToast} />}
        {showEditRole && selectedRole && <EditRoleModal apiFetch={apiFetch} role={selectedRole} roles={roles}
          onClose={() => { setShowEditRole(false); setSelectedRole(null); }}
          onSuccess={() => { setShowEditRole(false); setSelectedRole(null); fetchRoles(); }} showToast={showToast} />}
      </div>
    </AppLayout>
  );
}

function Pagination({ page, total, pageSize, onPage }: { page: number; total: number; pageSize: number; onPage: (p: number) => void }) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const pp = page; /* use p to avoid shadow */
  return (
    <div style={{ display: 'flex', gap: 4, justifyContent: 'center', marginTop: 16 }}>
      <button disabled={pp <= 1} onClick={() => onPage(pp - 1)}
        style={{ padding: '4px 10px', border: '1px solid var(--border)', borderRadius: 6, cursor: 'pointer', fontSize: 13, background: 'transparent', color: 'var(--text-primary)' }}>‹</button>
      {Array.from({ length: totalPages }, (_, i) => i + 1).map(p => (
        <button key={p} onClick={() => onPage(p)}
          style={{ padding: '4px 10px', border: '1px solid var(--border)', borderRadius: 6, cursor: 'pointer', fontSize: 13,
            background: p === pp ? '#5c7cfa' : 'transparent', color: p === pp ? '#fff' : 'var(--text-primary)' }}>{p}</button>
      ))}
      <button disabled={pp >= totalPages} onClick={() => onPage(pp + 1)}
        style={{ padding: '4px 10px', border: '1px solid var(--border)', borderRadius: 6, cursor: 'pointer', fontSize: 13, background: 'transparent', color: 'var(--text-primary)' }}>›</button>
    </div>
  );
}

const btnPri: React.CSSProperties = { padding: '8px 16px', background: '#5c7cfa', color: '#fff', border: 'none', borderRadius: 8, cursor: 'pointer', fontSize: 14 };
const btnSec: React.CSSProperties = { padding: '8px 16px', background: 'transparent', color: 'var(--text-secondary)', border: '1px solid var(--border)', borderRadius: 8, cursor: 'pointer', fontSize: 14 };
const btnSm: React.CSSProperties = { padding: '4px 10px', background: 'transparent', border: '1px solid var(--border)', borderRadius: 6, cursor: 'pointer', fontSize: 12 };
const mOverlay: React.CSSProperties = { position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 9999 };
const mContent: React.CSSProperties = { background: 'var(--card-bg)', padding: 24, borderRadius: 12, minWidth: 400 };
const inLabel: React.CSSProperties = { display: 'block', fontSize: 13, marginBottom: 8, color: 'var(--text-secondary)' };
const inStyle: React.CSSProperties = { display: 'block', width: '100%', marginTop: 4, padding: 8, borderRadius: 6, border: '1px solid var(--border)', background: 'var(--input-bg)', color: 'var(--text-primary)', fontSize: 14 };

function AddRoleModal({ apiFetch, roles, onClose, onSuccess, showToast }: any) {
  const [name, setName] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [description, setDescription] = useState('');
  const [parentID, setParentID] = useState('');
  const create = async () => {
    try {
      await apiFetch('/admin/rbac/roles', { method: 'POST', body: JSON.stringify({ name, display_name: displayName, description, parent_id: parentID }) });
      showToast('角色已创建'); onSuccess();
    } catch (e: any) { showToast(e?.message || '创建失败'); }
  };
  const lvl = (r: RBACRole) => `L${r.level}`;
  return (
    <div style={mOverlay} onClick={onClose}><div style={mContent} onClick={e => e.stopPropagation()}>
      <h3 style={{ marginBottom: 12 }}>新建角色</h3>
      <label style={inLabel}>名称 <input style={inStyle} value={name} onChange={e => setName(e.target.value)} placeholder="my_custom_role" /></label>
      <label style={inLabel}>显示名 <input style={inStyle} value={displayName} onChange={e => setDisplayName(e.target.value)} placeholder="我的自定义角色" /></label>
      <label style={inLabel}>描述 <input style={inStyle} value={description} onChange={e => setDescription(e.target.value)} /></label>
      <label style={inLabel}>父角色 <select style={inStyle} value={parentID} onChange={e => setParentID(e.target.value)}>
        <option value="">无（根角色）</option>
        {roles.filter((r: RBACRole) => r.level < 2 && r.child_count < 10).map((r: RBACRole) => <option key={r.id} value={r.id}>{r.display_name} ({lvl(r)})</option>)}
      </select></label>
      <div style={{ display: 'flex', gap: 8, marginTop: 16, justifyContent: 'flex-end' }}>
        <button onClick={onClose} style={btnSec}>取消</button>
        <button data-testid="rbac-add-role-submit" onClick={create} style={btnPri}>创建</button>
      </div>
    </div></div>
  );
}
function EditRoleModal({ apiFetch, role, roles, onClose, onSuccess, showToast }: any) {
  const [displayName, setDisplayName] = useState(role.display_name);
  const [description, setDescription] = useState(role.description || '');
  const [parentID, setParentID] = useState(role.parent_id || '');
  const update = async () => {
    try {
      await apiFetch(`/admin/rbac/roles/${role.id}`, { method: 'PUT', body: JSON.stringify({ display_name: displayName, description, parent_id: parentID }) });
      showToast('已更新'); onSuccess();
    } catch (e: any) { showToast(e?.message || '更新失败'); }
  };
  return (
    <div style={mOverlay} onClick={onClose}><div style={mContent} onClick={e => e.stopPropagation()}>
      <h3 style={{ marginBottom: 12 }}>编辑 — {role.name}</h3>
      <p style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 12 }}>层级 L{role.level}（不可更改） | {role.type}</p>
      <label style={inLabel}>显示名 <input style={inStyle} value={displayName} onChange={e => setDisplayName(e.target.value)} /></label>
      <label style={inLabel}>描述 <input style={inStyle} value={description} onChange={e => setDescription(e.target.value)} /></label>
      <label style={inLabel}>父角色 <select style={inStyle} value={parentID} onChange={e => setParentID(e.target.value)}>
        <option value="">无</option>
        {roles.filter((r: RBACRole) => r.level === role.level - 1 && r.id !== role.id && r.child_count < 10).map((r: RBACRole) => <option key={r.id} value={r.id}>{r.display_name} (L{r.level})</option>)}
      </select></label>
      <div style={{ display: 'flex', gap: 8, marginTop: 16, justifyContent: 'flex-end' }}>
        <button onClick={onClose} style={btnSec}>取消</button>
        <button data-testid="rbac-edit-role-submit" onClick={update} style={btnPri}>保存</button>
      </div>
    </div></div>
  );
}


function AddPermModal({ apiFetch, onClose, onSuccess, showToast }: any) {
  const [key, setKey] = useState('');
  const [name, setName] = useState('');
  const [module, setModule] = useState('custom');
  const [description, setDescription] = useState('');
  const create = async () => {
    if (!key || !name || !module) { showToast('Key、名称、模块必填'); return; }
    try {
      await apiFetch('/admin/rbac/permissions', { method: 'POST', body: JSON.stringify({ key, name, module, description }) });
      showToast('权限已创建'); onSuccess();
    } catch (e: any) { showToast(e?.message || '创建失败'); }
  };
  const mo: React.CSSProperties = { position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 9999 };
  const mc: React.CSSProperties = { background: 'var(--card-bg)', padding: 24, borderRadius: 12, minWidth: 450 };
  return (
    <div style={mo} onClick={onClose}>
      <div style={mc} onClick={e => e.stopPropagation()}>
        <h3 style={{ marginBottom: 12 }}>新建权限</h3>
        <label style={inLabel}>Key (必填, e.g. mymodule:action) <input style={inStyle} value={key} onChange={e => setKey(e.target.value)} /></label>
        <label style={inLabel}>名称 (必填) <input style={inStyle} value={name} onChange={e => setName(e.target.value)} /></label>
        <label style={inLabel}>模块 (必填) <input style={inStyle} value={module} onChange={e => setModule(e.target.value)} /></label>
        <label style={inLabel}>描述 <input style={inStyle} value={description} onChange={e => setDescription(e.target.value)} /></label>
        <div style={{ marginTop: 12, textAlign: 'right' }}>
          <button onClick={onClose} style={btnSec}>取消</button>{' '}
          <button onClick={create} style={btnPri} data-testid="rbac-add-perm-submit">创建</button>
        </div>
      </div>
    </div>
  );
}
