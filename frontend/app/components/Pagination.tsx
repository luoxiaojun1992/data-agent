'use client';

import React from 'react';

interface PaginationProps {
  current: number;
  totalPages: number;
  onChange: (page: number) => void;
  className?: string;
}

// Smart pagination: 上一页 [1] ... [4] [5] [6] ... [20] 下一页
// Shows: first, current±1, last with ellipsis for gaps.
export default function Pagination({ current, totalPages, onChange, className = '' }: PaginationProps) {
  if (totalPages <= 1) return null;

  const getPages = (): (number | '...')[] => {
    const pages: (number | '...')[] = [];
    const last = totalPages;

    // Always show: 1, current-1, current, current+1, last
    const set = new Set<number>([1, last, current, current - 1, current + 1]);
    // also neighbors: ±2
    set.add(current - 2);
    set.add(current + 2);

    const sorted = Array.from(set).filter(p => p >= 1 && p <= last).sort((a, b) => a - b);

    let prev = 0;
    for (const p of sorted) {
      if (prev && p - prev > 1) pages.push('...');
      pages.push(p);
      prev = p;
    }
    return pages;
  };

  const btn = 'min-w-[36px] h-9 px-3 rounded text-sm flex items-center justify-center transition-colors';
  const active = 'bg-[#B1E2FF] text-black';
  const inactive = 'bg-white/5 text-[var(--text-secondary)] hover:bg-white/10';
  const disabled = 'bg-white/5 text-[var(--text-secondary)] opacity-40 cursor-not-allowed';

  return (
    <div className={`flex items-center justify-center gap-2 mt-4 ${className}`}>
      <button
        onClick={() => onChange(current - 1)}
        disabled={current <= 1}
        className={`${btn} ${current <= 1 ? disabled : inactive}`}
      >
        上一页
      </button>
      {getPages().map((p, i) =>
        p === '...' ? (
          <span key={`e${i}`} className="text-[var(--text-secondary)] px-1">…</span>
        ) : (
          <button
            key={p}
            onClick={() => onChange(p)}
            className={`${btn} ${p === current ? active : inactive}`}
          >
            {p}
          </button>
        )
      )}
      <button
        onClick={() => onChange(current + 1)}
        disabled={current >= totalPages}
        className={`${btn} ${current >= totalPages ? disabled : inactive}`}
      >
        下一页
      </button>
    </div>
  );
}