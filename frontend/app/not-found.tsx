import Link from 'next/link';

export default function NotFound() {
  return (
    <div className="flex items-center justify-center min-h-screen bg-[var(--bg-primary)]" data-testid="page-404">
      <div className="text-center glass p-12 rounded-2xl max-w-md">
        <div className="text-6xl mb-4">🔮</div>
        <h1 className="text-2xl font-bold text-[var(--text-primary)] mb-2" data-testid="page-404-title">
          页面未找到
        </h1>
        <p className="text-[var(--text-secondary)] mb-6">
          您访问的页面不存在或已被移除
        </p>
        <Link
          href="/"
          className="inline-block px-6 py-2.5 bg-[var(--accent)] text-white rounded-lg hover:opacity-90 transition-opacity"
          data-testid="page-404-home-link"
        >
          返回首页
        </Link>
      </div>
    </div>
  );
}
