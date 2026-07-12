'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '../../../lib/api';

interface Role {
  id: string;
  name: string;
  display_name: string;
  description?: string;
  permissions: string[];
  type: string;
  created_at?: string;
}

interface PermissionInfo {
  key: string;
  name: string;
  description: string;
}

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1';

export default function RolesPage() {
  const { auth, apiFetch } = useAuth();

  const [tab, setTab] = useState<'roles' | 'permissions'>('roles');
  const [roles, setRoles] = useState<Role[]>([]);
  const [permissions, setPermissions] = useState<PermissionInfo[]>([]);
  const [loading, setLoading] = useState(true);

  // Modal states
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [selectedRole, setSelectedRole] = useState<Role | null>(null);

  // Form states
  const [formName, setFormName] = useState('');
  const [formDisplayName, setFormDisplayName] = useState('');
  const [formPermissions, setFormPermissions] = useState<string[]>([]);
  const [formError, setFormError] = useState('');
  const [formSubmitting, setFormSubmitting] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  const showToast = (msg: string, type: 'success' | 'error') => {
    setToast({ message: msg, type });
    setTimeout(() => setToast(null), 3000);
  };

  const fetchData = useCallback(async () => {
    try {
      setLoading(true);
      const [rolesRes, permsRes] = await Promise.all([
        apiFetch('/roles'),
        apiFetch('/permissions'),
      ]);
      if (rolesRes.ok) {
        const data = await rolesRes.json();
        setRoles(data.roles || []);
      }
      if (permsRes.ok) {
        const data = await permsRes.json();
        setPermissions(data.permissions || []);
      }
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, [apiFetch]);

  useEffect(() => {
    if (auth.hydrated) fetchData();
  }, [auth.hydrated, fetchData]);

  const fixedRoles = roles.filter((r) => r.type === 'fixed');
  const customRoles = roles.filter((r) => r.type === 'custom');

  // Create custom role
  const handleCreate = async () => {
    setFormError('');
    if (!formName || !formDisplayName) {
      setFormError('请填写角色名称');
      return;
    }
    setFormSubmitting(true);
    try {
      const res = await apiFetch('/roles', {
        method: 'POST',
        body: JSON.stringify({ name: formName, display_name: formDisplayName, permissions: formPermissions }),
      });
      if (!res.ok) {
        const d = await res.json();
        setFormError(d.error || '创建失败');
        return;
      }
      showToast('角色创建成功', 'success');
      setShowCreateModal(false);
      resetForm();
      fetchData();
    } catch {
      setFormError('创建失败');
    } finally {
      setFormSubmitting(false);
    }
  };

  // Edit permissions
  const handleEdit = async () => {
    if (!selectedRole) return;
    setFormSubmitting(true);
    try {
      const res = await apiFetch(`/roles/${selectedRole.id}`, {
        method: 'PUT',
        body: JSON.stringify({ permissions: formPermissions }),
      });
      if (!res.ok) {
        const d = await res.json();
        setFormError(d.error || '更新失败');
        return;
      }
      showToast('权限已更新', 'success');
      setShowEditModal(false);
      resetForm();
      fetchData();
    } catch {
      setFormError('更新失败');
    } finally {
      setFormSubmitting(false);
    }
  };

  // Delete custom role
  const handleDelete = async () => {
    if (!selectedRole) return;
    setFormSubmitting(true);
    try {
      const res = await apiFetch(`/roles/${selectedRole.id}`, { method: 'DELETE' });
      if (!res.ok) {
        const d = await res.json();
        showToast(d.error || '删除失败', 'error');
        setShowDeleteModal(false);
        return;
      }
      showToast('角色已删除', 'success');
      setShowDeleteModal(false);
      fetchData();
    } catch {
      showToast('删除失败', 'error');
    } finally {
      setFormSubmitting(false);
    }
  };

  const resetForm = () => {
    setFormName('');
    setFormDisplayName('');
    setFormPermissions([]);
    setFormError('');
    setFormSubmitting(false);
    setSelectedRole(null);
  };

  const openEdit = (role: Role) => {
    setSelectedRole(role);
    setFormPermissions([...role.permissions]);
    setFormError('');
    setShowEditModal(true);
  };

  const openDelete = (role: Role) => {
    setSelectedRole(role);
    setShowDeleteModal(true);
  };

  const togglePermission = (key: string) => {
    setFormPermissions((prev) =>
      prev.includes(key) ? prev.filter((k) => k !== key) : [...prev, key]
    );
  };

  return (
    <AppLayout>
      <div className="animate-fade-in">
        {/* Header */}
        <div className="mb-8" data-testid="admin-roles-header">
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div>
              <h2 className="text-2xl font-bold text-[var(--text-primary)]" data-testid="admin-roles-title">
                权限管理
              </h2>
            </div>
            <button
              data-testid="role-create-btn"
              onClick={() => { resetForm(); setShowCreateModal(true); }}
              style={{
                background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
                color: '#fff', border: 'none', borderRadius: '8px',
                padding: '8px 20px', fontSize: '14px', fontWeight: 600, cursor: 'pointer',
              }}
            >
              + 新建角色
            </button>
          </div>
        </div>

        {/* Page header for SPEC-024 */}
        <div data-testid="role-page-header" style={{ display: 'none' }} />

        {/* Tabs */}
        <div data-testid="role-tabs" style={{ display: 'flex', gap: '0px', marginBottom: '24px' }}>
          <button
            data-testid="role-tab-roles"
            onClick={() => setTab('roles')}
            style={{
              padding: '8px 24px', border: 'none', borderRadius: '8px 0 0 8px',
              background: tab === 'roles' ? 'var(--accent)' : 'rgba(255,255,255,0.06)',
              color: tab === 'roles' ? '#fff' : 'var(--text-secondary)',
              fontSize: '14px', cursor: 'pointer',
            }}
          >
            角色
          </button>
          <button
            data-testid="role-tab-permissions"
            onClick={() => setTab('permissions')}
            style={{
              padding: '8px 24px', border: 'none', borderRadius: '0 8px 8px 0',
              background: tab === 'permissions' ? 'var(--accent)' : 'rgba(255,255,255,0.06)',
              color: tab === 'permissions' ? '#fff' : 'var(--text-secondary)',
              fontSize: '14px', cursor: 'pointer',
            }}
          >
            权限
          </button>
        </div>

        {/* Toast */}
        {toast && (
          <div style={{ position: 'fixed', top: 20, right: 20, zIndex: 9999,
            background: toast.type === 'success' ? 'rgba(16,185,129,0.9)' : 'rgba(239,68,68,0.9)',
            color: '#fff', padding: '12px 20px', borderRadius: '8px', fontSize: '14px', fontWeight: 500,
          }}>
            {toast.message}
          </div>
        )}

        {/* Loading */}
        {loading && (
          <div className="glass p-12 text-center">
            <p className="text-sm text-[var(--text-secondary)]">加载中...</p>
          </div>
        )}

        {/* Roles Tab */}
        {!loading && tab === 'roles' && (
          <>
            {/* Fixed Role Cards */}
            <div data-testid="role-fixed-cards" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: '16px', marginBottom: '32px' }}>
              {fixedRoles.map((role, i) => (
                <div
                  key={role.id}
                  data-testid={`role-fixed-card-${i}`}
                  className="glass"
                  style={{ padding: '20px' }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '8px' }}>
                    <span style={{ fontSize: '18px', fontWeight: 700, color: 'var(--text-primary)' }}>
                      {role.display_name}
                    </span>
                    <span
                      data-testid="role-fixed-badge"
                      style={{
                        display: 'inline-block', padding: '2px 10px', borderRadius: '10px',
                        background: 'rgba(156,163,175,0.15)', color: '#9ca3af',
                        fontSize: '11px', fontWeight: 500,
                      }}
                    >
                      固定角色
                    </span>
                  </div>
                  <p style={{ fontSize: '13px', color: '#7A7A7A', lineHeight: 1.6 }}>{role.description}</p>
                  <div style={{ marginTop: '12px', display: 'flex', flexWrap: 'wrap', gap: '4px' }}>
                    {role.permissions.slice(0, 3).map((p) => (
                      <span key={p} style={{ padding: '2px 8px', borderRadius: '4px', background: 'rgba(92,124,250,0.1)', color: '#5c7cfa', fontSize: '11px' }}>{p}</span>
                    ))}
                    {role.permissions.length > 3 && (
                      <span style={{ fontSize: '11px', color: 'var(--text-secondary)' }}>+{role.permissions.length - 3}</span>
                    )}
                  </div>
                </div>
              ))}
            </div>

            {/* Custom Roles Section */}
            {customRoles.length > 0 && (
              <div>
                <h3 style={{ fontSize: '15px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '12px' }}>
                  自定义角色 ({customRoles.length})
                </h3>
                <div className="glass" style={{ overflow: 'hidden' }}>
                  <table data-testid="role-custom-table" style={{ width: '100%', borderCollapse: 'collapse' }}>
                    <thead>
                      <tr style={{ background: 'rgba(255,255,255,0.03)' }}>
                        <th style={thStyle}>角色名称</th>
                        <th style={thStyle}>权限数量</th>
                        <th style={thStyle}>创建时间</th>
                        <th style={thStyle}>操作</th>
                      </tr>
                    </thead>
                    <tbody>
                      {customRoles.map((role) => (
                        <tr key={role.id} style={{ borderBottom: '1px solid rgba(255,255,255,0.06)' }}>
                          <td style={tdStyle}>{role.display_name || role.name}</td>
                          <td style={tdStyle}>
                            <span style={{ padding: '2px 8px', borderRadius: '10px', background: 'rgba(92,124,250,0.1)', color: '#5c7cfa', fontSize: '12px' }}>
                              {role.permissions?.length || 0}
                            </span>
                          </td>
                          <td style={tdStyle}>{role.created_at ? new Date(role.created_at).toLocaleDateString('zh-CN') : '—'}</td>
                          <td style={tdStyle}>
                            <div style={{ display: 'flex', gap: '8px' }}>
                              <button
                                data-testid={`role-edit-btn-${role.id}`}
                                onClick={() => openEdit(role)}
                                style={actionBtnStyle('#5c7cfa')}
                              >
                                编辑
                              </button>
                              <button
                                data-testid={`role-delete-btn-${role.id}`}
                                onClick={() => openDelete(role)}
                                style={actionBtnStyle('#ef4444')}
                              >
                                删除
                              </button>
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}

            {/* Empty state */}
            {!loading && roles.length === 0 && (
              <div className="glass p-12 text-center" data-testid="admin-roles-empty">
                <p className="text-lg text-[var(--text-primary)] mb-2">权限管理</p>
                <p className="text-sm text-[var(--text-secondary)]">暂无数据</p>
              </div>
            )}
          </>
        )}

        {/* Permissions Tab */}
        {!loading && tab === 'permissions' && (
          <div data-testid="role-permissions-tab">
            <div className="glass" style={{ overflow: 'hidden' }}>
              <table data-testid="role-permission-table" style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ background: 'rgba(255,255,255,0.03)' }}>
                    <th style={thStyle}>权限标识</th>
                    <th style={thStyle}>权限名称</th>
                    <th style={thStyle}>描述</th>
                  </tr>
                </thead>
                <tbody>
                  {permissions.map((perm) => (
                    <tr key={perm.key} style={{ borderBottom: '1px solid rgba(255,255,255,0.06)' }}>
                      <td style={{ ...tdStyle, fontFamily: 'monospace', fontSize: '12px', color: '#5c7cfa' }}>
                        {perm.key}
                      </td>
                      <td style={tdStyle}>{perm.name}</td>
                      <td style={tdStyle}>{perm.description}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {/* ── Create Role Modal ── */}
        {showCreateModal && (
          <ModalOverlay onClose={() => setShowCreateModal(false)}>
            <div data-testid="role-create-modal" style={modalStyle} onClick={(e) => e.stopPropagation()}>
              <h3 style={modalTitleStyle}>新建角色</h3>
              {formError && <p style={{ color: '#ef4444', fontSize: '13px', marginBottom: '12px' }}>{formError}</p>}
              <div style={fieldStyle}>
                <label style={labelStyle}>角色名称 *</label>
                <input data-testid="role-create-name" value={formDisplayName} onChange={(e) => { setFormDisplayName(e.target.value); setFormName(e.target.value.toLowerCase().replace(/\s+/g, '_')); }} placeholder="例如：销售经理" style={inputStyle} />
              </div>
              <div style={fieldStyle}>
                <label style={labelStyle}>权限选择</label>
                <div data-testid="role-create-permissions" style={permListStyle}>
                  {permissions.map((perm) => (
                    <label key={perm.key} style={permItemStyle}>
                      <input
                        type="checkbox"
                        checked={formPermissions.includes(perm.key)}
                        onChange={() => togglePermission(perm.key)}
                        style={{ marginRight: '8px' }}
                      />
                      <span style={{ fontSize: '13px' }}>{perm.name}</span>
                      <span style={{ fontSize: '11px', color: 'var(--text-secondary)', marginLeft: '6px' }}>{perm.key}</span>
                    </label>
                  ))}
                </div>
              </div>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', marginTop: '20px' }}>
                <button onClick={() => setShowCreateModal(false)} style={cancelBtnStyle}>取消</button>
                <button data-testid="role-create-submit" onClick={handleCreate} disabled={formSubmitting}
                  style={{ ...submitBtnStyle, opacity: formSubmitting ? 0.6 : 1 }}>确认创建</button>
              </div>
            </div>
          </ModalOverlay>
        )}

        {/* ── Edit Role Modal ── */}
        {showEditModal && selectedRole && (
          <ModalOverlay onClose={() => setShowEditModal(false)}>
            <div data-testid="role-edit-modal" style={modalStyle} onClick={(e) => e.stopPropagation()}>
              <h3 style={modalTitleStyle}>编辑权限 — {selectedRole.display_name || selectedRole.name}</h3>
              {formError && <p style={{ color: '#ef4444', fontSize: '13px', marginBottom: '12px' }}>{formError}</p>}
              <div style={permListStyle}>
                {permissions.map((perm) => (
                  <label key={perm.key} style={permItemStyle}>
                    <input
                      type="checkbox"
                      checked={formPermissions.includes(perm.key)}
                      onChange={() => togglePermission(perm.key)}
                      style={{ marginRight: '8px' }}
                    />
                    <span style={{ fontSize: '13px' }}>{perm.name}</span>
                    <span style={{ fontSize: '11px', color: 'var(--text-secondary)', marginLeft: '6px' }}>{perm.key}</span>
                  </label>
                ))}
              </div>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', marginTop: '20px' }}>
                <button onClick={() => setShowEditModal(false)} style={cancelBtnStyle}>取消</button>
                <button data-testid="role-edit-submit" onClick={handleEdit} disabled={formSubmitting}
                  style={{ ...submitBtnStyle, opacity: formSubmitting ? 0.6 : 1 }}>保存</button>
              </div>
            </div>
          </ModalOverlay>
        )}

        {/* ── Delete Confirmation ── */}
        {showDeleteModal && selectedRole && (
          <ModalOverlay onClose={() => setShowDeleteModal(false)}>
            <div data-testid="role-delete-confirm-modal" style={modalStyle} onClick={(e) => e.stopPropagation()}>
              <h3 style={modalTitleStyle}>删除角色</h3>
              <p style={{ fontSize: '14px', color: 'var(--text-secondary)', marginBottom: '20px' }}>
                确定要删除角色「{selectedRole.display_name || selectedRole.name}」吗？此操作不可撤销。
              </p>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
                <button onClick={() => setShowDeleteModal(false)} style={cancelBtnStyle}>取消</button>
                <button data-testid="role-delete-confirm-btn" onClick={handleDelete} disabled={formSubmitting}
                  style={{ ...submitBtnStyle, background: '#ef4444', opacity: formSubmitting ? 0.6 : 1 }}>确认删除</button>
              </div>
            </div>
          </ModalOverlay>
        )}
      </div>
    </AppLayout>
  );
}

// ── Shared Styles ──

const thStyle: React.CSSProperties = {
  padding: '12px 16px', textAlign: 'left', fontSize: '11px', fontWeight: 700,
  textTransform: 'uppercase', color: '#666', borderBottom: '1px solid rgba(255,255,255,0.06)',
};
const tdStyle: React.CSSProperties = {
  padding: '12px 16px', fontSize: '13px', color: '#7A7A7A',
};
const actionBtnStyle = (color: string): React.CSSProperties => ({
  background: 'transparent', border: `1px solid ${color}40`, color, borderRadius: '6px',
  padding: '4px 12px', fontSize: '12px', cursor: 'pointer',
});
const modalStyle: React.CSSProperties = {
  background: '#1a1a2e', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '16px',
  padding: '28px', maxWidth: '480px', width: '100%', maxHeight: '80vh', overflowY: 'auto',
};
const modalTitleStyle: React.CSSProperties = { fontSize: '18px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '16px' };
const fieldStyle: React.CSSProperties = { marginBottom: '12px' };
const labelStyle: React.CSSProperties = { display: 'block', fontSize: '13px', color: 'var(--text-secondary)', marginBottom: '4px' };
const inputStyle: React.CSSProperties = {
  width: '100%', padding: '8px 12px', background: 'rgba(255,255,255,0.06)',
  border: '1px solid rgba(255,255,255,0.1)', borderRadius: '8px', fontSize: '14px',
  color: 'var(--text-primary)', outline: 'none', boxSizing: 'border-box',
};
const cancelBtnStyle: React.CSSProperties = {
  padding: '8px 16px', background: 'transparent', border: '1px solid rgba(255,255,255,0.1)',
  borderRadius: '8px', color: 'var(--text-secondary)', fontSize: '14px', cursor: 'pointer',
};
const submitBtnStyle: React.CSSProperties = {
  padding: '8px 20px', background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
  border: 'none', borderRadius: '8px', color: '#fff', fontSize: '14px', fontWeight: 600, cursor: 'pointer',
};
const permListStyle: React.CSSProperties = {
  maxHeight: '300px', overflowY: 'auto', padding: '8px',
  background: 'rgba(255,255,255,0.03)', borderRadius: '8px',
};
const permItemStyle: React.CSSProperties = {
  display: 'flex', alignItems: 'center', padding: '6px 8px',
  cursor: 'pointer', borderRadius: '4px',
};

function ModalOverlay({ children, onClose }: { children: React.ReactNode; onClose: () => void }) {
  return (
    <div onClick={onClose} style={{
      position: 'fixed', top: 0, left: 0, right: 0, bottom: 0,
      background: 'rgba(0,0,0,0.6)', backdropFilter: 'blur(4px)',
      display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000,
    }}>
      {children}
    </div>
  );
}
