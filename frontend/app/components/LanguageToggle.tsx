'use client';

import React, { useTransition } from 'react';
import { useLocale } from 'next-intl';
import { useRouter } from 'next/navigation';
import { LOCALE_COOKIE, LOCALE_STORAGE_KEY } from '../../lib/i18n/config';

// SPEC-103 语言切换：写 cookie（SSR 生效）+ localStorage 镜像，然后 router.refresh()
// 触发服务端按新语言重渲染。按钮文案固定显示「目标语言」，不随翻译走。
export default function LanguageToggle() {
  const locale = useLocale();
  const router = useRouter();
  const [isPending, startTransition] = useTransition();

  const toggle = () => {
    const next = locale === 'zh' ? 'en' : 'zh';
    document.cookie = `${LOCALE_COOKIE}=${next}; path=/; max-age=31536000; SameSite=Lax`;
    try {
      localStorage.setItem(LOCALE_STORAGE_KEY, next);
    } catch {
      /* ignore */
    }
    startTransition(() => {
      router.refresh();
    });
  };

  return (
    <button
      type="button"
      onClick={toggle}
      disabled={isPending}
      data-testid="language-toggle"
      aria-label={locale === 'zh' ? 'Switch to English' : '切换到中文'}
      title={locale === 'zh' ? 'Switch to English' : '切换到中文'}
      className="p-2 rounded-lg hover:bg-[var(--glass-bg)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors text-xs font-medium"
    >
      {locale === 'zh' ? 'EN' : '中文'}
    </button>
  );
}
