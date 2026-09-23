import { getRequestConfig } from 'next-intl/server';
import { cookies } from 'next/headers';
import { DEFAULT_LOCALE, LOCALE_COOKIE } from './config';

// SPEC-103 D4：语言状态存 cookie（SSR 可读，避免首屏闪烁 + hydration mismatch）。
// 默认 zh；切换时客户端写 NEXT_LOCALE cookie + data-agent-locale localStorage 镜像。
export default getRequestConfig(async () => {
  const cookieStore = cookies();
  const locale = cookieStore.get(LOCALE_COOKIE)?.value === 'en' ? 'en' : DEFAULT_LOCALE;

  return {
    locale,
    messages: (await import(`./messages/${locale}.json`)).default,
  };
});
