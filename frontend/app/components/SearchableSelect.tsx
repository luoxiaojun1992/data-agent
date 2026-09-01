'use client';

import React, { useState, useEffect, useRef, useCallback } from 'react';

// SearchableOption is the unified dropdown item shape. Backends return items
// with `id` plus a display field (`name` for models/permissions,
// `display_name` for roles). Extra fields are passed through untouched.
export interface SearchableOption {
  id: string;
  name?: string;
  display_name?: string;
  [key: string]: any;
}

// optionLabel resolves the human-readable label for an item using labelKey
// (default "name"), falling back to display_name / name / id.
export function optionLabel(item: SearchableOption, labelKey: string): string {
  const v = item[labelKey];
  if (typeof v === 'string' && v !== '') return v;
  return item.display_name || item.name || item.id;
}

// useDebouncedSearch drives the backend search data source shared by both the
// single-select combobox (below) and the list-style "add row" modals. All
// filtering happens in the backend (q + limit); the client only renders the
// returned top-N results (SPEC-074).
export function useDebouncedSearch(
  fetchFn: (q: string, limit: number) => Promise<SearchableOption[]>,
  limit = 20,
) {
  const [items, setItems] = useState<SearchableOption[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const fetchRef = useRef(fetchFn);
  fetchRef.current = fetchFn;
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const run = useCallback((q: string) => {
    if (timer.current) clearTimeout(timer.current);
    // First load is immediate; subsequent keystrokes are debounced 250ms.
    const delay = q === '' ? 0 : 250;
    timer.current = setTimeout(async () => {
      setLoading(true);
      try {
        const res = await fetchRef.current(q, limit);
        setItems(res || []);
        setError(null);
      } catch (e: any) {
        setError(e?.message || '加载失败');
        setItems([]);
      } finally {
        setLoading(false);
      }
    }, delay);
  }, [limit]);

  useEffect(() => {
    run('');
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [run]);

  const onSearch = useCallback((q: string) => {
    setQuery(q);
    run(q);
  }, [run]);

  const reload = useCallback(() => run(query), [run, query]);

  return { items, loading, error, query, onSearch, reload };
}

interface SearchableSelectProps {
  fetch: (q: string, limit: number) => Promise<SearchableOption[]>;
  value: string;
  onChange: (id: string) => void;
  labelKey?: string; // default "name"
  placeholder?: string;
  disabled?: boolean;
  dataTestid?: string;
  // Single-select "empty" option (e.g. "无（根角色）" for parent role).
  allowEmpty?: boolean;
  emptyLabel?: string;
  // Optional per-item label override (e.g. append " (默认)" for default models).
  renderLabel?: (item: SearchableOption) => React.ReactNode;
}

// SearchableSelect is a searchable single-select combobox used for model and
// parent-role dropdowns. It loads top-N on open and debounce-searches the
// backend on input (SPEC-074). Filtering/sorting/truncation stay in the DB.
export default function SearchableSelect({
  fetch,
  value,
  onChange,
  labelKey = 'name',
  placeholder,
  disabled,
  dataTestid,
  allowEmpty,
  emptyLabel = '无',
  renderLabel,
}: SearchableSelectProps) {
  const [open, setOpen] = useState(false);
  const { items, loading, error, query, onSearch } = useDebouncedSearch(fetch, 20);
  const [selectedCache, setSelectedCache] = useState<SearchableOption | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  // Close on outside click.
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  // Keep a cache of the currently selected item so the label stays correct
  // even when a search temporarily replaces the list.
  useEffect(() => {
    if (!value) {
      setSelectedCache(null);
      return;
    }
    const found = items.find(i => i.id === value);
    if (found) setSelectedCache(found);
  }, [value, items]);

  const label = selectedCache
    ? optionLabel(selectedCache, labelKey)
    : (value || placeholder || '选择…');

  return (
    <div ref={ref} style={{ position: 'relative' }} data-testid={dataTestid}>
      <button
        type="button"
        onClick={() => !disabled && setOpen(o => !o)}
        disabled={disabled}
        className="w-full px-3 py-1.5 text-xs rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none disabled:opacity-50 flex items-center justify-between gap-2"
      >
        <span className="truncate">{label}</span>
        <span className="text-[var(--text-secondary)]">▾</span>
      </button>

      {open && (
        <div className="absolute z-50 mt-1 w-full rounded-lg border border-[var(--border-glass)] bg-[#1a1a2e] shadow-xl overflow-hidden">
          <input
            autoFocus
            value={query}
            onChange={e => onSearch(e.target.value)}
            placeholder="搜索…"
            className="w-full px-3 py-2 text-xs bg-transparent border-b border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none placeholder:text-[var(--text-secondary)]"
          />
          <div className="max-h-56 overflow-y-auto">
            {allowEmpty && (
              <button
                type="button"
                onClick={() => { onChange(''); setOpen(false); }}
                className="w-full text-left px-3 py-2 text-xs text-[var(--text-secondary)] hover:bg-[var(--glass-hover)]"
              >
                {emptyLabel}
              </button>
            )}
            {loading && <div className="px-3 py-2 text-xs text-[var(--text-secondary)]">加载中…</div>}
            {!loading && error && <div className="px-3 py-2 text-xs text-[#ef4444]">{error}</div>}
            {!loading && !error && items.length === 0 && (
              <div className="px-3 py-2 text-xs text-[var(--text-secondary)]">无结果</div>
            )}
            {!loading && items.map(item => (
              <button
                key={item.id}
                type="button"
                onClick={() => { onChange(item.id); setOpen(false); }}
                className="w-full text-left px-3 py-2 text-xs text-[var(--text-primary)] hover:bg-[var(--glass-hover)]"
              >
                {renderLabel ? renderLabel(item) : optionLabel(item, labelKey)}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
