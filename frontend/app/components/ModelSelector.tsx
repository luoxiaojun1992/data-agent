'use client';

import React, { useCallback, useRef } from 'react';
import SearchableSelect, { SearchableOption } from './SearchableSelect';

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1';

interface ModelSelectorProps {
  value: string;
  onChange: (modelId: string) => void;
  disabled?: boolean;
  token?: string | null;
}

/**
 * ModelSelector renders a searchable dropdown of available LLM models
 * (Type==llm only). SPEC-062: used by chat/agent/imbind pages to bind a model
 * to a session. SPEC-074: loads top-N (20) from the backend and debounce-
 * searches via /models/list?q=&limit= — filtering/sorting stay in the DB.
 * The default model (is_default_for non-empty) is surfaced first by the
 * backend and shown with a "(默认)" suffix.
 */
export default function ModelSelector({ value, onChange, disabled, token }: ModelSelectorProps) {
  // Track the latest bound value so the first-load auto-select (below) sees
  // the current value without re-running on every render.
  const valueRef = useRef(value);
  valueRef.current = value;

  const doFetch = useCallback(async (q: string, limit: number): Promise<SearchableOption[]> => {
    const headers: Record<string, string> = {};
    if (token) headers['Authorization'] = `Bearer ${token}`;
    const url = `${API_BASE}/models/list?limit=${limit}${q ? `&q=${encodeURIComponent(q)}` : ''}`;
    const res = await fetch(url, { headers });
    if (!res.ok) throw new Error('加载模型失败');
    const data = await res.json();
    const models: SearchableOption[] = data.models || [];
    // Auto-select the default model on the initial (empty-query) load when
    // nothing is bound yet — mirrors the native <select> behaviour and the
    // "new session" reset. Search results must NOT auto-select.
    if (q === '' && models.length > 0 && !valueRef.current) {
      const def = models.find(m => Array.isArray(m.is_default_for) && m.is_default_for.length > 0);
      onChange(def ? def.id : models[0].id);
    }
    return models;
  }, [token, onChange]);

  if (disabled) {
    return (
      <span
        className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg border border-[var(--border-glass)] text-[var(--text-secondary)]"
        data-testid="model-selector-locked"
        title="会话已绑定模型，不可更换"
      >
        🔒 {value || '默认模型'}
      </span>
    );
  }

  return (
    <SearchableSelect
      fetch={doFetch}
      value={value}
      onChange={onChange}
      placeholder="选择模型…"
      dataTestid="model-selector"
      renderLabel={(item) => (
        <span>
          {item.name}
          {Array.isArray(item.is_default_for) && item.is_default_for.length > 0 && (
            <span style={{ color: 'var(--text-secondary)' }}> (默认)</span>
          )}
        </span>
      )}
    />
  );
}
