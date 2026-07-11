import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'DataAgent — 企业数据分析平台',
  description: '智能数据分析 Agent，Chat + Agent 双模式',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="antialiased">{children}</body>
    </html>
  );
}
// trigger clean CI run
