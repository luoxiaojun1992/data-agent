'use client';

import React, { useState, useEffect } from 'react';

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1';

export interface ModelOption {
  id: string;
  name: string;
  capability?: string;
  is_default?: boolean;
}

interface ModelSelectorProps {
  value: string;
  onChange: (modelId: string) => void;
  disabled?: boolean;
  token?: string | null;
}

/**
 * ModelSelector renders a dropdown of available LLM models (Type==llm only).
 * SPEC-062: used by chat/agent/imbind pages to bind a model to a session.
 * When disabled (session already bound), shows the current model with a lock.
 */
export default function ModelSelector({ value, onChange, disabled, token }: ModelSelectorProps) {
  const [models, setModels] = useState<ModelOption[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (disabled) return; // no need to fetch when locked
    setLoading(true);
    const headers: Record<string, string> = {};
    if (token) headers['Authorization'] = `Bearer ${token}`;
    fetch(`${API_BASE}/models/list`, { headers })
      .then(res => res.ok ? res.json() : Promise.reject(new Error('load models failed')))
      .then(data => {
        setModels(data.models || []);
        setError(null);
        // Auto-select default when no value set.
        if (!value && (data.models || []).length > 0) {
          const def = data.models.find((m: ModelOption) => m.is_default);
          onChange(def ? def.id : data.models[0].id);
        }
      })
      .catch(err => { console.error('ModelSelector load:', err); setError(err.message); })
      .finally(() => setLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [disabled]);

  const selected = models.find(m => m.id === value);
  const displayName = selected ? selected.name : (value || '默认模型');

  if (disabled) {
    return (
      <span
        className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg border border-[var(--border-glass)] text-[var(--text-secondary)]"
        data-testid="model-selector-locked"
        title="会话已绑定模型，不可更换"
      >
        🔒 {displayName}
      </span>
    );
  }

  return (
    <select
      value={value}
      onChange={e => onChange(e.target.value)}
      disabled={loading}
      className="px-3 py-1.5 text-xs rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] focus:outline-none disabled:opacity-50"
      data-testid="model-selector"
    >
      {loading && <option value="">加载模型…</option>}
      {!loading && error && <option value="">模型加载失败</option>}
      {!loading && !error && models.length === 0 && <option value="">无可用模型</option>}
      {!loading && models.map(m => (
        <option key={m.id} value={m.id}>
          {m.name}{m.is_default ? ' (默认)' : ''}
        </option>
      ))}
    </select>
  );
}
