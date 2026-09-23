'use client';

import React from 'react';
import { useTranslations } from 'next-intl';
import AppLayout from '../providers';
import { useAuth, ADMIN_MENU_PERMS } from '../../lib/api';

export default function AdminPage() {
  const t = useTranslations('admin');
  const { auth } = useAuth();

  const ENTRIES = [
    { title: t('modelsTitle'), desc: t('modelsDesc'), icon: '🤖', href: '/admin/models', perm: ADMIN_MENU_PERMS.models },
    { title: t('skillsTitle'), desc: t('skillsDesc'), icon: '🔧', href: '/admin/skills', perm: ADMIN_MENU_PERMS.skills },
    { title: t('apiTitle'), desc: t('apiDesc'), icon: '🔌', href: '/admin/api-collections', perm: ADMIN_MENU_PERMS.apiCollections },
    { title: t('usersTitle'), desc: t('usersDesc'), icon: '👥', href: '/admin/users', perm: ADMIN_MENU_PERMS.users },
    { title: t('rbacTitle'), desc: t('rbacDesc'), icon: '🛡️', href: '/admin/rbac', perm: ADMIN_MENU_PERMS.rbac },
    { title: t('invitesTitle'), desc: t('invitesDesc'), icon: '📨', href: '/admin/invites', perm: ADMIN_MENU_PERMS.invites },
    { title: t('auditTitle'), desc: t('auditDesc'), icon: '📋', href: '/admin/audit', perm: ADMIN_MENU_PERMS.audit },
    { title: t('settingsTitle'), desc: t('settingsDesc'), icon: '⚙', href: '/admin/settings', perm: ADMIN_MENU_PERMS.settings },
  ];

  const visible = ENTRIES.filter(e => auth.permissions.includes(e.perm));

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-8">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">{t('title')}</h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">{t('desc')}</p>
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
