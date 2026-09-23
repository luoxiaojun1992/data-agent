'use client';

import React, { useState, useEffect } from 'react';
import { useTranslations } from 'next-intl';

export default function ScrollToTop() {
  const t = useTranslations('scrollTop');
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const onScroll = () => {
      setVisible(window.scrollY > 300);
    };
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => window.removeEventListener('scroll', onScroll);
  }, []);

  if (!visible) return null;

  return (
    <button
      onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
      aria-label={t('top')}
      title={t('top')}
      data-testid="scroll-to-top"
      style={{
        position: 'fixed',
        bottom: 32,
        right: 32,
        zIndex: 999,
        width: 44,
        height: 44,
        borderRadius: '50%',
        background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
        color: '#fff',
        border: 'none',
        cursor: 'pointer',
        fontSize: 20,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        boxShadow: '0 4px 14px rgba(92, 124, 250, 0.4)',
        transition: 'opacity 0.3s, transform 0.3s',
      }}
      onMouseEnter={e => (e.currentTarget.style.transform = 'translateY(-2px)')}
      onMouseLeave={e => (e.currentTarget.style.transform = 'translateY(0)')}
    >
      ↑
    </button>
  );
}
