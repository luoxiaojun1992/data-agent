import React from 'react';

// SPEC-078：前端列表页 UI 规范统一的共享样式常量。

// 顶部主按钮：渐变 #5c7cfa→#7c3aed（以 admin/users「添加用户」为准）
export const primaryButtonStyle: React.CSSProperties = {
  background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
  color: '#fff',
  border: 'none',
  borderRadius: '8px',
  padding: '8px 20px',
  fontSize: '14px',
  fontWeight: 600,
  cursor: 'pointer',
};

// 弹窗遮罩层：玻璃模糊
export const modalOverlayStyle: React.CSSProperties = {
  position: 'fixed',
  inset: 0,
  zIndex: 1000,
  background: 'rgba(0,0,0,0.6)',
  backdropFilter: 'blur(4px)',
  WebkitBackdropFilter: 'blur(4px)',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
};

// 弹窗面板：玻璃透明（引用当前主题既有 CSS 变量，避免硬编码色值）
export const modalPanelStyle: React.CSSProperties = {
  background: 'var(--bg-secondary)',
  border: '1px solid var(--border-glass)',
  borderRadius: '16px',
  padding: '28px',
  maxWidth: '480px',
  width: '100%',
  boxShadow: '0 8px 32px rgba(0,0,0,0.5)',
};
