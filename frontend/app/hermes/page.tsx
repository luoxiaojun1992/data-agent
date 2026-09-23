'use client';

import React from 'react';
import { useTranslations } from 'next-intl';
import AppLayout from '../providers';

export default function HermesPage() {
  const t = useTranslations('hermes');
  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-8" data-testid="hermes-page-header">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]" data-testid="hermes-page-title">
            {t('title')}
          </h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">
            {t('desc')}
          </p>
        </div>

        <div className="glass p-12 text-center" data-testid="hermes-under-construction">
          <span className="text-5xl block mb-4">🚧</span>
          <p className="text-lg text-[var(--text-primary)] mb-2" data-testid="hermes-uc-title">
            {t('underConstruction')}
          </p>
          <p className="text-sm text-[var(--text-secondary)]">
            {t('comingSoon')}
          </p>
        </div>
      </div>
    </AppLayout>
  );
}
