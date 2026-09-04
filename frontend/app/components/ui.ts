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

// 弹窗遮罩层：玻璃模糊（SPEC-085 基准：对齐「新建分析任务」弹窗 bg-black/50）
export const modalOverlayStyle: React.CSSProperties = {
  position: 'fixed',
  inset: 0,
  zIndex: 1000,
  background: 'rgba(0,0,0,0.5)',
  backdropFilter: 'blur(4px)',
  WebkitBackdropFilter: 'blur(4px)',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
};

// 弹窗面板：玻璃透明（SPEC-085 基准：对齐 .glass 玻璃等效 var(--glass-bg) + blur(20px)）
export const modalPanelStyle: React.CSSProperties = {
  background: 'var(--glass-bg)',
  backdropFilter: 'blur(20px)',
  WebkitBackdropFilter: 'blur(20px)',
  border: '1px solid var(--border-glass)',
  borderRadius: '16px',
  padding: '24px',
  maxWidth: '512px',
  width: '100%',
  boxShadow: '0 8px 32px rgba(0,0,0,0.5)',
};

// 弹窗面板 input 样式（SPEC-085：边框 rgba(255,255,255,0.15) 可见，替代 var(--border)/0.1）
export const modalInputStyle: React.CSSProperties = {
  width: '100%',
  padding: '8px 12px',
  fontSize: '14px',
  background: 'rgba(255,255,255,0.06)',
  border: '1px solid rgba(255,255,255,0.15)',
  borderRadius: '8px',
  color: 'var(--text-primary)',
  outline: 'none',
  boxSizing: 'border-box',
};

// 弹窗面板 label 样式
export const modalLabelStyle: React.CSSProperties = {
  display: 'block',
  fontSize: '13px',
  color: 'var(--text-secondary)',
  marginBottom: '4px',
};

// 弹窗面板 select 样式（继承 modalInputStyle）
export const modalSelectStyle: React.CSSProperties = { ...modalInputStyle };

// 弹窗取消按钮
export const modalCancelBtnStyle: React.CSSProperties = {
  padding: '8px 16px',
  background: 'transparent',
  border: '1px solid rgba(255,255,255,0.15)',
  borderRadius: '8px',
  color: 'var(--text-secondary)',
  fontSize: '14px',
  cursor: 'pointer',
};
