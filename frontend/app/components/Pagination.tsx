'use client';

import React from 'react';

interface PaginationProps {
  // 语义 A（既有，保留）: 当前页 + 总页数
  current?: number;
  totalPages?: number;
  // 语义 B（新增，推荐）: 页码 + 总数 + 每页条数
  page?: number;
  total?: number;
  pageSize?: number;
  // 回调
  onChange: (page: number) => void;
  // 每页条数切换（统一内嵌进分页组件）
  onPageSizeChange?: (size: number) => void;
  pageSizeOptions?: number[]; // 默认 [10, 20, 50, 100]
  className?: string;
  // testid 前缀：生成 {prefix}-pagination / {prefix}-pagination-prev / {prefix}-pagination-next / {prefix}-page-{n} / {prefix}-page-size-select
  testIdPrefix?: string;
}

// Smart pagination: 上一页 [1] ... [4] [5] [6] ... [20] 下一页
// 统一分页组件（SPEC-078）：兼容语义 A（current/totalPages）与语义 B（page/total/pageSize），
// 内嵌每页条数下拉，可选显示「共 N 条」，支持 testid 前缀（对齐 SPEC-035）。
export default function Pagination({
  current, totalPages, page, total, pageSize,
  onChange, onPageSizeChange, pageSizeOptions = [10, 20, 50, 100],
  className = '', testIdPrefix,
}: PaginationProps) {
  const hasTotal = total !== undefined;
  const cur = page ?? current ?? 1;
  const tp = totalPages !== undefined
    ? totalPages
    : (hasTotal && pageSize ? Math.max(1, Math.ceil(total / pageSize)) : 1);

  // 分页显示规则（晓军拍板 2026-09-04）：只要有数据就显示分页，禁止因「只有 1 页/1 条」而隐藏。
  // - 语义 B（有 total）：total > 0 就显示（含 total=1，显示「共 1 条」）
  // - 语义 A（仅 totalPages）：totalPages >= 1 就显示
  // - 无数据（total=0 或 totalPages=0）才隐藏
  // - 提供每页条数切换时始终渲染（保证下拉可用）
  if (hasTotal) {
    if (total <= 0 && !onPageSizeChange) return null;
  } else if (tp < 1 && !onPageSizeChange) {
    return null;
  }

  const tid = (suffix: string) => (testIdPrefix ? `${testIdPrefix}-${suffix}` : undefined);

  const getPages = (): (number | '...')[] => {
    const pages: (number | '...')[] = [];
    const last = tp;

    // Always show: 1, current-1, current, current+1, last; also neighbors ±2
    const set = new Set<number>([1, last, cur, cur - 1, cur + 1, cur - 2, cur + 2]);
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
    <div data-testid={tid('pagination')} className={`flex items-center justify-center gap-2 mt-4 flex-wrap ${className}`}>
      {hasTotal && (
        <span className="text-[13px] text-[var(--text-secondary)] mr-1">共{total}条</span>
      )}
      {onPageSizeChange && (
        <select
          data-testid={tid('page-size-select')}
          value={pageSize ?? pageSizeOptions[0]}
          onChange={(e) => onPageSizeChange(Number(e.target.value))}
          className="h-9 px-2 rounded text-[13px] bg-white/5 border border-white/10 text-[var(--text-primary)] focus:outline-none"
        >
          {pageSizeOptions.map((s) => (
            <option key={s} value={s}>{s}</option>
          ))}
        </select>
      )}
      <button
        data-testid={tid('pagination-prev')}
        onClick={() => onChange(cur - 1)}
        disabled={cur <= 1}
        className={`${btn} ${cur <= 1 ? disabled : inactive}`}
      >
        上一页
      </button>
      {getPages().map((p, i) =>
        p === '...' ? (
          <span key={`e${i}`} className="text-[var(--text-secondary)] px-1">…</span>
        ) : (
          <button
            key={p}
            data-testid={tid(`page-${p}`)}
            onClick={() => onChange(p)}
            className={`${btn} ${p === cur ? active : inactive}`}
          >
            {p}
          </button>
        )
      )}
      <button
        data-testid={tid('pagination-next')}
        onClick={() => onChange(cur + 1)}
        disabled={cur >= tp}
        className={`${btn} ${cur >= tp ? disabled : inactive}`}
      >
        下一页
      </button>
    </div>
  );
}
