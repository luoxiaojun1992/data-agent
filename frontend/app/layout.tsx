import type { Metadata } from 'next';
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

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeInitScript }} />
      </head>
      <body className="antialiased">
        <OnlineIndicator />
        {children}
      </body>
    </html>
  );
}
