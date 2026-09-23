import type { Metadata } from 'next';
import { NextIntlClientProvider } from 'next-intl';
import { getLocale, getMessages } from 'next-intl/server';
import './globals.css';
import OnlineIndicator from './components/OnlineIndicator';

export const metadata: Metadata = {
  title: 'DataAgent — 企业数据分析平台',
  description: '智能数据分析 Agent，Chat + Agent 双模式',
  icons: {
    icon: '/favicon.svg',
  },
};

// 首帧前同步读 localStorage 设置 data-theme，避免主题闪烁（SPEC-076 §4.4）。
// 该脚本为静态内容，非用户输入，无 XSS 风险。
const themeInitScript = `(function(){try{var t=localStorage.getItem('data-agent-theme');if(t==='light'||t==='dark'){document.documentElement.setAttribute('data-theme',t);}}catch(e){}})();`;

// 语言由 cookie（NEXT_LOCALE）驱动，SSR 已按正确语言渲染首屏（SPEC-103 D4）。
// 此处 inline script 仅在水合前再按 cookie 对齐 <html lang>，作为双保险。
const langInitScript = `(function(){try{var l=document.cookie.match(/(?:^|; )NEXT_LOCALE=([^;]+)/);var v=l?decodeURIComponent(l[1]):'zh';document.documentElement.setAttribute('lang',v==='en'?'en':'zh-CN');}catch(e){}})();`;

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const locale = await getLocale();
  const messages = await getMessages();

  return (
    <html lang={locale === 'en' ? 'en' : 'zh-CN'} suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeInitScript }} />
        <script dangerouslySetInnerHTML={{ __html: langInitScript }} />
      </head>
      <body className="antialiased">
        <NextIntlClientProvider locale={locale} messages={messages}>
          <OnlineIndicator />
          {children}
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
