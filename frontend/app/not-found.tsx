import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

export default async function NotFound() {
  const t = await getTranslations('notFound');

  return (
    <div className="flex items-center justify-center min-h-screen bg-[var(--bg-primary)]" data-testid="page-404">
      <div className="text-center glass p-12 rounded-2xl max-w-md">
        <div className="text-6xl mb-4">🔮</div>
        <h1 className="text-2xl font-bold text-[var(--text-primary)] mb-2" data-testid="page-404-title">
          {t('title')}
        </h1>
        <p className="text-[var(--text-secondary)] mb-6">
          {t('desc')}
        </p>
        <Link
          href="/"
          className="inline-block px-6 py-2.5 bg-[var(--accent)] text-white rounded-lg hover:opacity-90 transition-opacity"
          data-testid="page-404-home-link"
        >
          {t('home')}
        </Link>
      </div>
    </div>
  );
}
