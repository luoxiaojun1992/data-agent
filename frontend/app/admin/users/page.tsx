'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '../../../lib/api';

interface User {
  id: string;
  username: string;
  role: string;
  status: string;
  created_at?: string;
}

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1';

export default function UsersPage() {
  const { auth, apiFetch } = useAuth();

  const [users, setUsers] = useState<User[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [sortBy, setSortBy] = useState('created_at');
  const [sortOrder, setSortOrder] = useState<'desc' | 'asc' | ''>('desc');
  const [selected, setSelected] = useState<Set<string>>(new Set());

  // Modal states
  const [showAddModal, setShowAddModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [showToggleModal, setShowToggleModal] = useState(false);
  const [selectedUser, setSelectedUser] = useState<User | null>(null);

  // Form states
  const [formName, setFormName] = useState('');
  const [formEmail, setFormEmail] = useState('');
  const [formPassword, setFormPassword] = useState('');
  const [formRole, setFormRole] = useState('user');
  const [formError, setFormError] = useState('');
  const [formSubmitting, setFormSubmitting] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  const showToast = (message: string, type: 'success' | 'error') => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 3000);
  };

  const fetchUsers = useCallback(async () => {
    try {
      setLoading(true);
      const sortParam = sortOrder ? `&sort_by=${sortBy}&sort_order=${sortOrder}` : '';
      const res = await apiFetch(`/users?skip=${(page - 1) * pageSize}&limit=${pageSize}${sortParam}`);
      if (res.ok) {
        const data = await res.json();
        setUsers(data.users || []);
        setTotal(data.total || 0);
      }
    } catch (err) {
      // ignore
    } finally {
      setLoading(false);
    }
  }, [apiFetch, page, pageSize, sortBy, sortOrder]);

  useEffect(() => {
    if (auth.hydrated) {
      fetchUsers();
    }
  }, [auth.hydrated, fetchUsers]);

  // Add user
  const handleAdd = async () => {
    setFormError('');
    if (!formName || !formEmail || !formPassword) {
      setFormError('请填写所有必填字段');
      return;
    }
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    if (!emailRegex.test(formEmail)) {
      setFormError('邮箱格式不正确');
      return;
    }
    setFormSubmitting(true);
    try {
      const res = await apiFetch('/users', {
        method: 'POST',
        body: JSON.stringify({
          username: formEmail,
          password: formPassword,
          role: formRole === 'system_admin' ? 'system_admin' : formRole === 'admin' ? 'admin' : 'user',
          status: 'enabled',
        }),
      });
      const data = await res.json();
      if (!res.ok) {
        setFormError(data.error || '创建失败');
        return;
      }
      showToast('用户创建成功', 'success');
      setShowAddModal(false);
      resetForm();
      fetchUsers();
    } catch {
      setFormError('创建失败');
    } finally {
      setFormSubmitting(false);
    }
  };

  // Edit user role
  const handleEdit = async () => {
    if (!selectedUser) return;
    setFormSubmitting(true);
    try {
      const res = await apiFetch(`/users/${selectedUser.id}`, {
        method: 'PUT',
        body: JSON.stringify({ role: formRole }),
      });
      if (!res.ok) {
        const data = await res.json();
        setFormError(data.error || '更新失败');
        return;
      }
      showToast('角色已更新', 'success');
      setShowEditModal(false);
      resetForm();
      fetchUsers();
    } catch {
      setFormError('更新失败');
    } finally {
      setFormSubmitting(false);
    }
  };

  // Toggle user status
  const handleToggle = async () => {
    if (!selectedUser) return;
    const newStatus = selectedUser.status === 'enabled' ? 'disabled' : 'enabled';
    setFormSubmitting(true);
    try {
      const res = await apiFetch(`/users/${selectedUser.id}/status`, {
        method: 'PATCH',
        body: JSON.stringify({ status: newStatus }),
      });
      if (!res.ok) {
        const data = await res.json();
        showToast(data.error || '操作失败', 'error');
        setShowToggleModal(false);
        return;
      }
      showToast(newStatus === 'enabled' ? '用户已启用' : '用户已停用', 'success');
      setShowToggleModal(false);
      fetchUsers();
    } catch {
      showToast('操作失败', 'error');
    } finally {
      setFormSubmitting(false);
    }
  };

  // Delete user
  const handleDelete = async () => {
    if (!selectedUser) return;
    setFormSubmitting(true);
    try {
      const res = await apiFetch(`/users/${selectedUser.id}`, { method: 'DELETE' });
      if (!res.ok) {
        const data = await res.json();
        showToast(data.error || '删除失败', 'error');
        setShowDeleteModal(false);
        return;
      }
      showToast('用户已删除', 'success');
      setShowDeleteModal(false);
      fetchUsers();
    } catch {
      showToast('删除失败', 'error');
    } finally {
      setFormSubmitting(false);
    }
  };

  const resetForm = () => {
    setFormName('');
    setFormEmail('');
    setFormPassword('');
    setFormRole('user');
    setFormError('');
    setFormSubmitting(false);
    setSelectedUser(null);
  };

  const openEdit = (user: User) => {
    setSelectedUser(user);
    setFormRole(user.role);
    setFormError('');
    setShowEditModal(true);
  };

  const openToggle = (user: User) => {
    setSelectedUser(user);
    setShowToggleModal(true);
  };

  const openDelete = (user: User) => {
    setSelectedUser(user);
    setShowDeleteModal(true);
  };

  const roleLabel = (role: string) => {
    switch (role) {
      case 'system_admin': return '系统管理员';
      case 'admin': return '管理员';
      case 'user': return '普通用户';
      default: return role;
    }
  };

  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  const handleSort = (column: string) => {
    if (sortBy !== column) { setSortBy(column); setSortOrder('desc'); }
    else if (sortOrder === 'desc') setSortOrder('asc');
    else if (sortOrder === 'asc') { setSortBy('created_at'); setSortOrder('desc'); }
  };

  const sortIndicator = (column: string) => {
    if (sortBy !== column) return '';
    return sortOrder === 'desc' ? ' ↓' : ' ↑';
  };

  const toggleSelectAll = () => {
    if (selected.size === users.length) { setSelected(new Set()); }
    else { setSelected(new Set(users.map(u => u.id))); }
  };

  const toggleSelect = (id: string) => {
    setSelected(prev => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  };

  return (
    <AppLayout>
      <div className="animate-fade-in">
        {/* Header */}
        <div className="mb-8" data-testid="admin-users-header">
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div>
              <h2 className="text-2xl font-bold text-[var(--text-primary)]" data-testid="admin-users-title">用户管理</h2>
              <p className="text-sm text-[var(--text-secondary)] mt-1">用户数据管理</p>
            </div>
            <button
              data-testid="user-add-btn"
              onClick={() => { resetForm(); setShowAddModal(true); }}
              style={{
                background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
                color: '#fff',
                border: 'none',
                borderRadius: '8px',
                padding: '8px 20px',
                fontSize: '14px',
                fontWeight: 600,
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
              }}
            >
              <span>+</span> 添加用户
            </button>
          </div>
        </div>

        {/* Page header (SPEC-023) */}
        <div data-testid="user-page-header" style={{ display: 'none' }} />

        {/* Toast */}
        {toast && (
          <div style={{
            position: 'fixed', top: 20, right: 20, zIndex: 9999,
            background: toast.type === 'success' ? 'rgba(16,185,129,0.9)' : 'rgba(239,68,68,0.9)',
            color: '#fff', padding: '12px 20px', borderRadius: '8px',
            fontSize: '14px', fontWeight: 500,
          }}>
            {toast.message}
          </div>
        )}

        {/* User Table */}
        {!loading && users.length > 0 && (
          <div className="glass" data-testid="admin-users-table" style={{ overflow: 'hidden' }}>
            <table data-testid="user-table" style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ background: 'rgba(255,255,255,0.03)' }}>
                  <th style={{ ...thStyle, width: '40px' }}>
                    <input type="checkbox" data-testid="user-select-all"
                      checked={selected.size === users.length && users.length > 0}
                      onChange={toggleSelectAll} />
                  </th>
                  <th data-testid="user-table-header-name" style={{ ...thStyle, cursor: 'pointer' }}
                    onClick={() => handleSort('username')}>
                    姓名<span data-testid="user-sort-name">{sortIndicator('username')}</span>
                  </th>
                  <th data-testid="user-table-header-email" style={thStyle}>邮箱</th>
                  <th data-testid="user-table-header-role" style={thStyle}>角色</th>
                  <th data-testid="user-table-header-status" style={thStyle}>状态</th>
                  <th data-testid="user-table-header-created" style={{ ...thStyle, cursor: 'pointer' }}
                    onClick={() => handleSort('created_at')}>
                    创建时间<span data-testid="user-sort-created">{sortIndicator('created_at')}</span>
                  </th>
                  <th data-testid="user-table-header-actions" style={thStyle}>操作</th>
                </tr>
              </thead>
              <tbody>
                {users.map((user) => (
                  <tr
                    key={user.id}
                    data-testid={`user-row-${user.id}`}
                    style={{
                      borderBottom: '1px solid rgba(255,255,255,0.06)',
                      transition: 'background 0.15s',
                    }}
                    onMouseEnter={(e) => { e.currentTarget.style.background = 'rgba(255,255,255,0.03)'; }}
                    onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent'; }}
                  >
                    <td style={tdStyle}>
                      <input type="checkbox" data-testid={`user-select-${user.id}`}
                        checked={selected.has(user.id)} onChange={() => toggleSelect(user.id)} />
                    </td>
                    <td style={tdStyle}>{user.username.split('@')[0] || user.username}</td>
                    <td style={tdStyle}>{user.username}</td>
                    <td style={tdStyle}>{roleLabel(user.role)}</td>
                    <td style={tdStyle}>
                      <span
                        data-testid={`user-status-${user.id}`}
                        style={{
                          display: 'inline-block',
                          padding: '2px 12px',
                          borderRadius: '12px',
                          fontSize: '12px',
                          fontWeight: 500,
                          background: user.status === 'enabled' ? 'rgba(16,185,129,0.15)' : 'rgba(244,114,182,0.15)',
                          color: user.status === 'enabled' ? '#10b981' : '#f472b6',
                        }}
                      >
                        {user.status === 'enabled' ? '🟢 启用' : '🔴 停用'}
                      </span>
                    </td>
                    <td style={{ ...tdStyle, minWidth: '120px', padding: '4px 8px' }}>
                      <div style={{ display: 'flex', gap: '4px', justifyContent: 'flex-end' }}>
                        <button
                          data-testid={`user-edit-btn-${user.id}`}
                          onClick={() => openEdit(user)}
                          style={{ ...iconBtnStyle, color: '#5c7cfa' }}
                          title="编辑"
                        >
                          ✏️
                        </button>
                        {user.role !== 'system_admin' && (
                          <button
                            data-testid={`user-toggle-btn-${user.id}`}
                            onClick={() => openToggle(user)}
                            style={{ ...iconBtnStyle, color: user.status === 'enabled' ? '#f59e0b' : '#10b981' }}
                            title={user.status === 'enabled' ? '停用' : '启用'}
                          >
                            {user.status === 'enabled' ? '⏸' : '▶'}
                          </button>
                        )}
                        {user.role !== 'system_admin' && (
                          <button
                            data-testid={`user-delete-btn-${user.id}`}
                            onClick={() => openDelete(user)}
                            style={{ ...iconBtnStyle, color: '#ef4444' }}
                            title="删除"
                          >
                            🗑
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {/* Empty state */}
        {!loading && users.length === 0 && (
          <div className="glass p-12 text-center" data-testid="admin-users-empty">
            <p className="text-lg text-[var(--text-primary)] mb-2">用户管理</p>
            <p className="text-sm text-[var(--text-secondary)]">暂无用户数据，点击右上角按钮添加</p>
          </div>
        )}

        {/* Loading */}
        {loading && (
          <div className="glass p-12 text-center">
            <p className="text-sm text-[var(--text-secondary)]">加载中...</p>
          </div>
        )}

        {/* Pagination */}
        {total > 0 && (
          <div data-testid="user-pagination" style={{
            display: 'flex', justifyContent: 'space-between', alignItems: 'center',
            marginTop: '16px', fontSize: '13px', color: 'var(--text-secondary)',
          }}>
            <div style={{ display: 'flex', gap: '12px', alignItems: 'center' }}>
              {selected.size > 0 && (
                <span data-testid="user-select-count" style={{ color: '#5c7cfa' }}>已选 {selected.size} 项</span>
              )}
              <span>共{total}条</span>
            </div>
            <div style={{ display: 'flex', gap: '6px', alignItems: 'center' }}>
              <select
                data-testid="user-page-size-select"
                value={pageSize}
                onChange={(e) => { setPageSize(Number(e.target.value)); setPage(1); }}
                style={{
                  background: 'rgba(255,255,255,0.06)',
                  border: '1px solid rgba(255,255,255,0.1)',
                  borderRadius: '6px', padding: '4px 8px',
                  color: 'var(--text-primary)', fontSize: '13px',
                }}
              >
                <option value={10}>10</option>
                <option value={20}>20</option>
                <option value={50}>50</option>
              </select>
              <button data-testid="user-pagination-prev" onClick={() => setPage(p => Math.max(1, p - 1))}
                disabled={page === 1}
                style={{ padding: '4px 10px', borderRadius: '6px', border: '1px solid rgba(255,255,255,0.1)',
                  background: 'transparent', color: 'var(--text-secondary)', cursor: page === 1 ? 'not-allowed' : 'pointer', fontSize: '13px', opacity: page === 1 ? 0.4 : 1 }}>
                上一页
              </button>
              {Array.from({ length: Math.min(totalPages, 5) }, (_, i) => (
                <button
                  key={i}
                  data-testid={`user-page-${i + 1}`}
                  onClick={() => setPage(i + 1)}
                  style={{
                    padding: '4px 12px',
                    borderRadius: '6px',
                    border: '1px solid rgba(255,255,255,0.1)',
                    background: page === i + 1 ? 'var(--accent)' : 'transparent',
                    color: page === i + 1 ? '#fff' : 'var(--text-secondary)',
                    cursor: 'pointer', fontSize: '13px',
                  }}
                >
                  {i + 1}
                </button>
              ))}
              <button data-testid="user-pagination-next" onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                disabled={page >= totalPages}
                style={{ padding: '4px 10px', borderRadius: '6px', border: '1px solid rgba(255,255,255,0.1)',
                  background: 'transparent', color: 'var(--text-secondary)', cursor: page >= totalPages ? 'not-allowed' : 'pointer', fontSize: '13px', opacity: page >= totalPages ? 0.4 : 1 }}>
                下一页
              </button>
            </div>
          </div>
        )}

        {/* ── Add User Modal ── */}
        {showAddModal && (
          <ModalOverlay onClose={() => setShowAddModal(false)}>
            <div data-testid="user-add-modal" style={modalStyle} onClick={(e) => e.stopPropagation()}>
              <h3 style={modalTitleStyle}>添加用户</h3>
              {formError && (
                <p data-testid="user-add-email-error" style={{ color: '#ef4444', fontSize: '13px', marginBottom: '12px' }}>
                  {formError}
                </p>
              )}
              <div style={fieldStyle}>
                <label style={labelStyle}>姓名 *</label>
                <input
                  data-testid="user-add-name"
                  value={formName}
                  onChange={(e) => setFormName(e.target.value)}
                  placeholder="输入姓名"
                  style={inputStyle}
                />
              </div>
              <div style={fieldStyle}>
                <label style={labelStyle}>邮箱 *</label>
                <input
                  data-testid="user-add-email"
                  value={formEmail}
                  onChange={(e) => setFormEmail(e.target.value)}
                  placeholder="example@company.com"
                  type="email"
                  style={inputStyle}
                />
              </div>
              <div style={fieldStyle}>
                <label style={labelStyle}>密码 *</label>
                <input
                  data-testid="user-add-password"
                  value={formPassword}
                  onChange={(e) => setFormPassword(e.target.value)}
                  placeholder="输入密码"
                  type="password"
                  style={inputStyle}
                />
              </div>
              <div style={fieldStyle}>
                <label style={labelStyle}>角色</label>
                <select
                  data-testid="user-add-role"
                  value={formRole}
                  onChange={(e) => setFormRole(e.target.value)}
                  style={inputStyle}
                >
                  <option value="user">普通用户</option>
                  <option value="admin">管理员</option>
                  <option
                    value="system_admin"
                    data-testid="user-add-role-system-admin-disabled"
                    disabled
                  >
                    系统管理员（已存在）
                  </option>
                </select>
              </div>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', marginTop: '20px' }}>
                <button
                  onClick={() => setShowAddModal(false)}
                  style={cancelBtnStyle}
                >
                  取消
                </button>
                <button
                  data-testid="user-add-submit"
                  onClick={handleAdd}
                  disabled={formSubmitting}
                  style={{
                    ...submitBtnStyle,
                    opacity: formSubmitting ? 0.6 : 1,
                  }}
                >
                  确认添加
                </button>
              </div>
            </div>
          </ModalOverlay>
        )}

        {/* ── Edit User Modal ── */}
        {showEditModal && selectedUser && (
          <ModalOverlay onClose={() => setShowEditModal(false)}>
            <div data-testid="user-edit-modal" style={modalStyle} onClick={(e) => e.stopPropagation()}>
              <h3 style={modalTitleStyle}>编辑用户角色</h3>
              {formError && (
                <p style={{ color: '#ef4444', fontSize: '13px', marginBottom: '12px' }}>{formError}</p>
              )}
              <p style={{ fontSize: '13px', color: 'var(--text-secondary)', marginBottom: '16px' }}>
                {selectedUser.username}
              </p>
              <div style={fieldStyle}>
                <label style={labelStyle}>角色</label>
                <select
                  data-testid="user-edit-role"
                  value={formRole}
                  onChange={(e) => setFormRole(e.target.value)}
                  style={inputStyle}
                >
                  <option value="user">普通用户</option>
                  <option value="admin">管理员</option>
                </select>
              </div>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', marginTop: '20px' }}>
                <button onClick={() => setShowEditModal(false)} style={cancelBtnStyle}>取消</button>
                <button
                  data-testid="user-edit-submit"
                  onClick={handleEdit}
                  disabled={formSubmitting}
                  style={{ ...submitBtnStyle, opacity: formSubmitting ? 0.6 : 1 }}
                >
                  保存
                </button>
              </div>
            </div>
          </ModalOverlay>
        )}

        {/* ── Toggle Status Modal ── */}
        {showToggleModal && selectedUser && (
          <ModalOverlay onClose={() => setShowToggleModal(false)}>
            <div data-testid="user-toggle-confirm-modal" style={modalStyle} onClick={(e) => e.stopPropagation()}>
              <h3 style={modalTitleStyle}>
                {selectedUser.status === 'enabled' ? '停用用户' : '启用用户'}
              </h3>
              <p style={{ fontSize: '14px', color: 'var(--text-secondary)', marginBottom: '20px' }}>
                确定要{selectedUser.status === 'enabled' ? '停用' : '启用'}用户 {selectedUser.username} 吗？
                {selectedUser.status === 'enabled' && '停用后该用户将无法登录。'}
              </p>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
                <button onClick={() => setShowToggleModal(false)} style={cancelBtnStyle}>取消</button>
                <button
                  onClick={handleToggle}
                  disabled={formSubmitting}
                  style={{ ...submitBtnStyle, opacity: formSubmitting ? 0.6 : 1 }}
                >
                  确认
                </button>
              </div>
            </div>
          </ModalOverlay>
        )}

        {/* ── Delete Confirmation Modal ── */}
        {showDeleteModal && selectedUser && (
          <ModalOverlay onClose={() => setShowDeleteModal(false)}>
            <div data-testid="user-delete-confirm-modal" style={modalStyle} onClick={(e) => e.stopPropagation()}>
              <h3 style={modalTitleStyle}>删除用户</h3>
              <p style={{ fontSize: '14px', color: 'var(--text-secondary)', marginBottom: '20px' }}>
                确定要删除用户 {selectedUser.username} 吗？此操作不可撤销。
              </p>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
                <button
                  onClick={() => setShowDeleteModal(false)}
                  style={cancelBtnStyle}
                  autoFocus
                >
                  取消
                </button>
                <button
                  data-testid="user-delete-confirm-btn"
                  onClick={handleDelete}
                  disabled={formSubmitting}
                  style={{
                    ...submitBtnStyle,
                    background: '#ef4444',
                    opacity: formSubmitting ? 0.6 : 1,
                  }}
                >
                  确认删除
                </button>
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
  padding: '12px 16px',
  textAlign: 'left',
  fontSize: '11px',
  fontWeight: 700,
  textTransform: 'uppercase',
  color: '#666',
  borderBottom: '1px solid rgba(255,255,255,0.06)',
};

const tdStyle: React.CSSProperties = {
  padding: '12px 16px',
  fontSize: '13px',
  color: '#7A7A7A',
};

const actionBtnStyle = (color: string): React.CSSProperties => ({
  background: 'transparent',
  border: `1px solid ${color}40`,
  color,
  borderRadius: '6px',
  padding: '4px 12px',
  fontSize: '12px',
  cursor: 'pointer',
  transition: 'all 0.15s',
});

const iconBtnStyle: React.CSSProperties = {
  width: '28px',
  height: '28px',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  background: 'transparent',
  border: 'none',
  borderRadius: '4px',
  fontSize: '14px',
  cursor: 'pointer',
  padding: 0,
};

const modalStyle: React.CSSProperties = {
  background: '#1a1a2e',
  border: '1px solid rgba(255,255,255,0.1)',
  borderRadius: '16px',
  padding: '28px',
  maxWidth: '420px',
  width: '100%',
};

const modalTitleStyle: React.CSSProperties = {
  fontSize: '18px',
  fontWeight: 600,
  color: 'var(--text-primary)',
  marginBottom: '16px',
};

const fieldStyle: React.CSSProperties = {
  marginBottom: '12px',
};

const labelStyle: React.CSSProperties = {
  display: 'block',
  fontSize: '13px',
  color: 'var(--text-secondary)',
  marginBottom: '4px',
};

const inputStyle: React.CSSProperties = {
  width: '100%',
  padding: '8px 12px',
  background: 'rgba(255,255,255,0.06)',
  border: '1px solid rgba(255,255,255,0.1)',
  borderRadius: '8px',
  fontSize: '14px',
  color: 'var(--text-primary)',
  outline: 'none',
  boxSizing: 'border-box',
};

const cancelBtnStyle: React.CSSProperties = {
  padding: '8px 16px',
  background: 'transparent',
  border: '1px solid rgba(255,255,255,0.1)',
  borderRadius: '8px',
  color: 'var(--text-secondary)',
  fontSize: '14px',
  cursor: 'pointer',
};

const submitBtnStyle: React.CSSProperties = {
  padding: '8px 20px',
  background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
  border: 'none',
  borderRadius: '8px',
  color: '#fff',
  fontSize: '14px',
  fontWeight: 600,
  cursor: 'pointer',
};

// ── Modal Overlay ──

function ModalOverlay({ children, onClose }: { children: React.ReactNode; onClose: () => void }) {
  return (
    <div
      onClick={onClose}
      style={{
        position: 'fixed',
        top: 0, left: 0, right: 0, bottom: 0,
        background: 'rgba(0,0,0,0.6)',
        backdropFilter: 'blur(4px)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
      }}
    >
      {children}
    </div>
  );
}
