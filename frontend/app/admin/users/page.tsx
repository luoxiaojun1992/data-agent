'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useTranslations } from 'next-intl';
import AppLayout from '../../providers';
import { useAuth } from '../../../lib/api';
import Pagination from '../../components/Pagination';
import { primaryButtonStyle, modalOverlayStyle, modalPanelStyle, modalInputStyle } from '../../components/ui';

interface User {
  id: string;
  username: string;
  role: string;
  status: string;
  created_at?: string;
}

export default function UsersPage() {
  const t = useTranslations('adminUsers');
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
      setFormError(t('requiredFields'));
      return;
    }
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    if (!emailRegex.test(formEmail)) {
      setFormError(t('invalidEmail'));
      return;
    }
    setFormSubmitting(true);
    try {
      const res = await apiFetch('/users', {
        method: 'POST',
        body: JSON.stringify({
          username: formEmail,
          password: formPassword,
          role: formRole,
          status: 'enabled',
        }),
      });
      const data = await res.json();
      if (!res.ok) {
        setFormError(data.error || t('createFailed'));
        return;
      }
      showToast(t('userCreated'), 'success');
      setShowAddModal(false);
      resetForm();
      fetchUsers();
    } catch {
      setFormError(t('createFailed'));
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
        setFormError(data.error || t('updateFailed'));
        return;
      }
      showToast(t('roleUpdated'), 'success');
      setShowEditModal(false);
      resetForm();
      fetchUsers();
    } catch {
      setFormError(t('updateFailed'));
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
        showToast(data.error || t('opFailed'), 'error');
        setShowToggleModal(false);
        return;
      }
      showToast(newStatus === 'enabled' ? t('userEnabled') : t('userDisabled'), 'success');
      setShowToggleModal(false);
      fetchUsers();
    } catch {
      showToast(t('opFailed'), 'error');
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
        showToast(data.error || t('deleteFailed'), 'error');
        setShowDeleteModal(false);
        return;
      }
      showToast(t('userDeleted'), 'success');
      setShowDeleteModal(false);
      fetchUsers();
    } catch {
      showToast(t('deleteFailed'), 'error');
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
      case 'system_admin': return t('roleSystemAdmin');
      case 'admin': return t('roleAdmin');
      case 'user': return t('roleUser');
      default: return role;
    }
  };

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
              <h2 className="text-2xl font-bold text-[var(--text-primary)]" data-testid="admin-users-title">{t('title')}</h2>
              <p className="text-sm text-[var(--text-secondary)] mt-1">{t('desc')}</p>
            </div>
            <button
              data-testid="user-add-btn"
              onClick={() => { resetForm(); setShowAddModal(true); }}
              style={{ ...primaryButtonStyle, display: 'flex', alignItems: 'center', gap: '6px' }}
            >
              <span>+</span> {t('addUser')}
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
                <tr style={{ background: 'var(--surface-3)' }}>
                  <th style={{ ...thStyle, width: '40px' }}>
                    <input type="checkbox" data-testid="user-select-all"
                      checked={selected.size === users.length && users.length > 0}
                      onChange={toggleSelectAll} />
                  </th>
                  <th data-testid="user-table-header-name" style={{ ...thStyle, cursor: 'pointer' }}
                    onClick={() => handleSort('username')}>
                    {t('colName')}<span data-testid="user-sort-name">{sortIndicator('username')}</span>
                  </th>
                  <th data-testid="user-table-header-email" style={thStyle}>{t('colEmail')}</th>
                  <th data-testid="user-table-header-role" style={thStyle}>{t('colRole')}</th>
                  <th data-testid="user-table-header-status" style={thStyle}>{t('colStatus')}</th>
                  <th data-testid="user-table-header-created" style={{ ...thStyle, cursor: 'pointer' }}
                    onClick={() => handleSort('created_at')}>
                    {t('colCreatedAt')}<span data-testid="user-sort-created">{sortIndicator('created_at')}</span>
                  </th>
                  <th data-testid="user-table-header-actions" style={thStyle}>{t('colActions')}</th>
                </tr>
              </thead>
              <tbody>
                {users.map((user) => (
                  <tr
                    key={user.id}
                    data-testid={`user-row-${user.id}`}
                    style={{
                      borderBottom: '1px solid var(--surface-6)',
                      transition: 'background 0.15s',
                    }}
                    onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--surface-3)'; }}
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
                        {user.status === 'enabled' ? t('statusEnabled') : t('statusDisabled')}
                      </span>
                    </td>
                    <td style={{ ...tdStyle, minWidth: '120px', padding: '4px 8px' }}>
                      <div style={{ display: 'flex', gap: '4px', justifyContent: 'flex-end' }}>
                        <a
                          href={`/admin/users/${user.id}/rbac-roles`}
                          data-testid={`user-rbac-btn-${user.id}`}
                          style={{ ...iconBtnStyle, color: '#a855f7', textDecoration: 'none' }}
                          title={t('rbacRoles')}
                        >
                          🛡️
                        </a>
                        {user.role !== 'system_admin' && (
                        <button
                          data-testid={`user-edit-btn-${user.id}`}
                          onClick={() => openEdit(user)}
                          style={{ ...iconBtnStyle, color: '#5c7cfa' }}
                          title={t('edit')}
                        >
                          ✏️
                        </button>
                        )}
                        {user.role !== 'system_admin' && (
                          <button
                            data-testid={`user-toggle-btn-${user.id}`}
                            onClick={() => openToggle(user)}
                            style={{ ...iconBtnStyle, color: user.status === 'enabled' ? '#f59e0b' : '#10b981' }}
                            title={user.status === 'enabled' ? t('disable') : t('enable')}
                          >
                            {user.status === 'enabled' ? '⏸' : '▶'}
                          </button>
                        )}
                        {user.role !== 'system_admin' && (
                          <button
                            data-testid={`user-delete-btn-${user.id}`}
                            onClick={() => openDelete(user)}
                            style={{ ...iconBtnStyle, color: '#ef4444' }}
                            title={t('delete')}
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
            <p className="text-lg text-[var(--text-primary)] mb-2">{t('title')}</p>
            <p className="text-sm text-[var(--text-secondary)]">{t('emptyHint')}</p>
          </div>
        )}

        {/* Loading */}
        {loading && (
          <div className="glass p-12 text-center">
            <p className="text-sm text-[var(--text-secondary)]">{t('loading')}</p>
          </div>
        )}

        {/* Pagination */}
        {total > 0 && (
          <div style={{ marginTop: '16px' }}>
            {selected.size > 0 && (
              <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: '8px' }}>
                <span data-testid="user-select-count" style={{ color: '#5c7cfa', fontSize: '13px' }}>
                  {t('selectedCount', { count: selected.size })}
                </span>
              </div>
            )}
            <Pagination
              page={page}
              total={total}
              pageSize={pageSize}
              onChange={setPage}
              onPageSizeChange={(s) => { setPageSize(s); setPage(1); }}
              testIdPrefix="user"
            />
          </div>
        )}

        {/* ── Add User Modal ── */}
        {showAddModal && (
          <ModalOverlay onClose={() => setShowAddModal(false)}>
            <div data-testid="user-add-modal" style={modalStyle} onClick={(e) => e.stopPropagation()}>
              <h3 style={modalTitleStyle}>{t('addUserTitle')}</h3>
              {formError && (
                <p data-testid="user-add-email-error" style={{ color: '#ef4444', fontSize: '13px', marginBottom: '12px' }}>
                  {formError}
                </p>
              )}
              <div style={fieldStyle}>
                <label style={labelStyle}>{t('nameLabel')}</label>
                <input
                  data-testid="user-add-name"
                  value={formName}
                  onChange={(e) => setFormName(e.target.value)}
                  placeholder={t('namePlaceholder')}
                  style={inputStyle}
                />
              </div>
              <div style={fieldStyle}>
                <label style={labelStyle}>{t('emailLabel')}</label>
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
                <label style={labelStyle}>{t('passwordLabel')}</label>
                <input
                  data-testid="user-add-password"
                  value={formPassword}
                  onChange={(e) => setFormPassword(e.target.value)}
                  placeholder={t('passwordPlaceholder')}
                  type="password"
                  style={inputStyle}
                />
              </div>
              <div style={fieldStyle}>
                <label style={labelStyle}>{t('roleLabel')}</label>
                <select
                  data-testid="user-add-role"
                  value={formRole}
                  onChange={(e) => setFormRole(e.target.value)}
                  style={inputStyle}
                >
                  <option value="user">{t('roleUser')}</option>
                  <option value="admin">{t('roleAdmin')}</option>
                </select>
              </div>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', marginTop: '20px' }}>
                <button
                  onClick={() => setShowAddModal(false)}
                  style={cancelBtnStyle}
                >
                  {t('cancel')}
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
                  {t('confirmAdd')}
                </button>
              </div>
            </div>
          </ModalOverlay>
        )}

        {/* ── Edit User Modal ── */}
        {showEditModal && selectedUser && (
          <ModalOverlay onClose={() => setShowEditModal(false)}>
            <div data-testid="user-edit-modal" style={modalStyle} onClick={(e) => e.stopPropagation()}>
              <h3 style={modalTitleStyle}>{t('editUserRoleTitle')}</h3>
              {formError && (
                <p style={{ color: '#ef4444', fontSize: '13px', marginBottom: '12px' }}>{formError}</p>
              )}
              <p style={{ fontSize: '13px', color: 'var(--text-secondary)', marginBottom: '16px' }}>
                {selectedUser.username}
              </p>
              <div style={fieldStyle}>
                <label style={labelStyle}>{t('roleLabel')}</label>
                <select
                  data-testid="user-edit-role"
                  value={formRole}
                  onChange={(e) => setFormRole(e.target.value)}
                  style={inputStyle}
                >
                  <option value="user">{t('roleUser')}</option>
                  <option value="admin">{t('roleAdmin')}</option>
                </select>
              </div>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', marginTop: '20px' }}>
                <button onClick={() => setShowEditModal(false)} style={cancelBtnStyle}>{t('cancel')}</button>
                <button
                  data-testid="user-edit-submit"
                  onClick={handleEdit}
                  disabled={formSubmitting}
                  style={{ ...submitBtnStyle, opacity: formSubmitting ? 0.6 : 1 }}
                >
                  {t('save')}
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
                {selectedUser.status === 'enabled' ? t('disableUserTitle') : t('enableUserTitle')}
              </h3>
              <p style={{ fontSize: '14px', color: 'var(--text-secondary)', marginBottom: '20px' }}>
                {selectedUser.status === 'enabled'
                  ? t('disableConfirm', { username: selectedUser.username })
                  : t('enableConfirm', { username: selectedUser.username })}
                {selectedUser.status === 'enabled' && t('disableHint')}
              </p>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
                <button onClick={() => setShowToggleModal(false)} style={cancelBtnStyle}>{t('cancel')}</button>
                <button
                  onClick={handleToggle}
                  disabled={formSubmitting}
                  style={{ ...submitBtnStyle, opacity: formSubmitting ? 0.6 : 1 }}
                >
                  {t('confirm')}
                </button>
              </div>
            </div>
          </ModalOverlay>
        )}

        {/* ── Delete Confirmation Modal ── */}
        {showDeleteModal && selectedUser && (
          <ModalOverlay onClose={() => setShowDeleteModal(false)}>
            <div data-testid="user-delete-confirm-modal" style={modalStyle} onClick={(e) => e.stopPropagation()}>
              <h3 style={modalTitleStyle}>{t('deleteUserTitle')}</h3>
              <p style={{ fontSize: '14px', color: 'var(--text-secondary)', marginBottom: '20px' }}>
                {t('deleteConfirm', { username: selectedUser.username })}
              </p>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
                <button
                  onClick={() => setShowDeleteModal(false)}
                  style={cancelBtnStyle}
                  autoFocus
                >
                  {t('cancel')}
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
                  {t('confirmDelete')}
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
  borderBottom: '1px solid var(--surface-6)',
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
  ...modalPanelStyle,
  maxHeight: '85vh',
  overflowY: 'auto',
  maxWidth: '420px',
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
  ...modalInputStyle,
};

const cancelBtnStyle: React.CSSProperties = {
  padding: '8px 16px',
  background: 'transparent',
  border: '1px solid var(--surface-10)',
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
      style={modalOverlayStyle}
    >
      {children}
    </div>
  );
}
