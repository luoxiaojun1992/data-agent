'use client';

import React, { useState } from 'react';
import { useAuth } from '../../lib/api';
import {
  modalOverlayStyle,
  modalPanelStyle,
  modalInputStyle,
  modalLabelStyle,
  primaryButtonStyle,
  modalCancelBtnStyle,
} from './ui';

/**
 * ChangePasswordModal is the glass-styled password change dialog (SPEC-083).
 * It posts to the migrated /auth/change-password endpoint (no RBAC — any
 * logged-in user may change their own password) and, on success, notifies the
 * caller so the app can log the user out and return to the login page.
 */
export default function ChangePasswordModal({
  onClose,
  onSuccess,
}: {
  onClose: () => void;
  onSuccess: () => void;
}) {
  const { apiFetch } = useAuth();
  const [oldPwd, setOldPwd] = useState('');
  const [newPwd, setNewPwd] = useState('');
  const [confirmPwd, setConfirmPwd] = useState('');
  const [error, setError] = useState('');
  const [confirmError, setConfirmError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async () => {
    setError('');
    setConfirmError('');

    if (newPwd !== confirmPwd) {
      setConfirmError('两次输入的密码不一致');
      return;
    }

    setSubmitting(true);
    try {
      const res = await apiFetch('/auth/change-password', {
        method: 'POST',
        body: JSON.stringify({ old_password: oldPwd, new_password: newPwd }),
      });
      if (res.ok) {
        onSuccess();
      } else {
        const d = await res.json().catch(() => ({}));
        setError(d.error || '修改失败');
      }
    } catch {
      setError('修改失败');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div style={modalOverlayStyle} data-testid="pwd-modal" onClick={onClose}>
      <div style={modalPanelStyle} onClick={(e) => e.stopPropagation()}>
        <h3 className="text-lg font-semibold text-[var(--text-primary)] mb-5">修改密码</h3>

        <div className="mb-4">
          <label style={modalLabelStyle} htmlFor="pwd-modal-old-input">旧密码</label>
          <input
            id="pwd-modal-old-input"
            style={modalInputStyle}
            data-testid="pwd-modal-old-input"
            type="password"
            value={oldPwd}
            onChange={(e) => setOldPwd(e.target.value)}
          />
        </div>

        <div className="mb-4">
          <label style={modalLabelStyle} htmlFor="pwd-modal-new-input">新密码</label>
          <input
            id="pwd-modal-new-input"
            style={modalInputStyle}
            data-testid="pwd-modal-new-input"
            type="password"
            value={newPwd}
            onChange={(e) => setNewPwd(e.target.value)}
          />
        </div>

        <div className="mb-4">
          <label style={modalLabelStyle} htmlFor="pwd-modal-confirm-input">确认新密码</label>
          <input
            id="pwd-modal-confirm-input"
            style={modalInputStyle}
            data-testid="pwd-modal-confirm-input"
            type="password"
            value={confirmPwd}
            onChange={(e) => setConfirmPwd(e.target.value)}
          />
          {confirmError && (
            <p data-testid="pwd-modal-confirm-error" className="text-xs text-red-400 mt-1">{confirmError}</p>
          )}
        </div>

        {error && (
          <p data-testid="pwd-modal-error" className="text-xs text-red-400 mb-3">{error}</p>
        )}

        <div className="flex justify-end gap-3 mt-2">
          <button
            type="button"
            style={modalCancelBtnStyle}
            data-testid="pwd-modal-cancel-btn"
            onClick={onClose}
            disabled={submitting}
          >
            取消
          </button>
          <button
            type="button"
            style={primaryButtonStyle}
            data-testid="pwd-modal-submit-btn"
            onClick={handleSubmit}
            disabled={submitting}
          >
            {submitting ? '提交中…' : '确认修改'}
          </button>
        </div>
      </div>
    </div>
  );
}
