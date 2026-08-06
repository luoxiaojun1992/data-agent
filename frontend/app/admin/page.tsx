'use client';

import React from 'react';
import AppLayout from '../providers';
import { useAuth, ADMIN_MENU_PERMS } from '../../lib/api';

const ENTRIES = [
  { title: '模型配置', desc: '管理 LLM 模型配置与参数', icon: '🤖', href: '/admin/models', perm: ADMIN_MENU_PERMS.models },
  { title: 'Skill 管理', desc: '管理 Agent 技能工具与配置', icon: '🔧', href: '/admin/skills', perm: ADMIN_MENU_PERMS.skills },
  { title: '用户管理', desc: '用户 CRUD 与角色分配', icon: '👥', href: '/admin/users', perm: ADMIN_MENU_PERMS.users },
  { title: 'RBAC 管理', desc: '角色与权限配置', icon: '🛡️', href: '/admin/rbac', perm: ADMIN_MENU_PERMS.rbac },
  { title: '邀请管理', desc: '生成和管理邀请注册链接', icon: '📨', href: '/admin/invites', perm: ADMIN_MENU_PERMS.invites },
  { title: '审计日志', desc: '查看系统操作审计记录', icon: '📋', href: '/admin/audit', perm: ADMIN_MENU_PERMS.audit },
  { title: '系统设置', desc: '全局配置参数管理', icon: '⚙', href: '/admin/settings', perm: ADMIN_MENU_PERMS.settings },
];

export default function AdminPage() {
  const { auth } = useAuth();

  const visible = ENTRIES.filter(e => auth.permissions.includes(e.perm));

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-8">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">管理后台</h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">系统配置与用户管理</p>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          {visible.map((item) => (
            <a key={item.href} href={item.href} className="glass p-6 glass-hover no-underline">
              <span className="text-3xl block mb-3">{item.icon}</span>
              <h3 className="text-base font-semibold text-[var(--text-primary)] mb-1">{item.title}</h3>
              <p className="text-sm text-[var(--text-secondary)]">{item.desc}</p>
            </a>
          ))}
        </div>
      </div>
    </AppLayout>
  );
}
