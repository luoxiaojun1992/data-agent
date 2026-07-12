'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '../../../lib/api';

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1';

export default function ModelsPage() {
  const { auth, apiFetch } = useAuth();

  const [apiUrl, setApiUrl] = useState('https://api.openai.com/v1');
  const [apiKey, setApiKey] = useState('');
  const [apiKeyExists, setApiKeyExists] = useState(false);
  const [showApiKey, setShowApiKey] = useState(false);
  const [modelName, setModelName] = useState('gpt-4o');
  const [contextLen, setContextLen] = useState(128000);
  const [maxOutput, setMaxOutput] = useState(16000);
  const [temperature, setTemperature] = useState('0.7');
  const [topP, setTopP] = useState('0.95');
  const [hermesUrl, setHermesUrl] = useState('http://hermes:8081');
  const [hermesApiKey, setHermesApiKey] = useState('');
  const [showHermesKey, setShowHermesKey] = useState(false);
  const [saving, setSaving] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  const showToast = (msg: string, type: 'success' | 'error') => {
    setToast({ message: msg, type });
    setTimeout(() => setToast(null), 3000);
  };

  const fetchConfig = useCallback(async () => {
    try {
      const res = await apiFetch('/model-config');
      if (res.ok) {
        const data = await res.json();
        if (data.api_url) setApiUrl(data.api_url);
        if (data.model_name) setModelName(data.model_name);
        if (data.context_len) setContextLen(Number(data.context_len));
        if (data.max_output) setMaxOutput(Number(data.max_output));
        if (data.temperature) setTemperature(String(data.temperature));
        if (data.top_p) setTopP(String(data.top_p));
        if (data.hermes_url) setHermesUrl(data.hermes_url);
        setApiKeyExists(data.api_key_exists);
        if (data.api_key_exists) setApiKey('••••••••••');
      }
    } catch {
      // defaults
    }
  }, [apiFetch]);

  useEffect(() => {
    if (auth.hydrated) fetchConfig();
  }, [auth.hydrated, fetchConfig]);

  const handleSave = async () => {
    setSaving(true);
    try {
      const body: Record<string, string> = {
        api_url: apiUrl,
        model_name: modelName,
        context_len: String(contextLen),
        max_output: String(maxOutput),
        temperature,
        top_p: topP,
        hermes_url: hermesUrl,
      };
      // Only include API key if it was changed (not masked)
      if (apiKey && apiKey !== '••••••••••') {
        body.api_key = apiKey;
      }
      if (hermesApiKey) {
        body.hermes_api_key = hermesApiKey;
      }
      const res = await apiFetch('/model-config', {
        method: 'PUT',
        body: JSON.stringify(body),
      });
      if (!res.ok) {
        const d = await res.json();
        showToast(d.error || '保存失败', 'error');
        return;
      }
      showToast('配置已保存', 'success');
      fetchConfig();
    } catch {
      showToast('保存失败', 'error');
    } finally {
      setSaving(false);
    }
  };

  const toggleEye = async () => {
    if (showApiKey) {
      setShowApiKey(false);
      setApiKey('••••••••••');
      return;
    }
    // Fetch decrypted key from vault
    try {
      const res = await apiFetch('/model-config');
      if (res.ok) {
        const data = await res.json();
        if (data.api_key_exists) {
          try {
            const decryptRes = await apiFetch('/vault/decrypt', {
              method: 'POST',
              body: JSON.stringify({ value: data.api_key || '' }),
            });
            if (decryptRes.ok) {
              const d = await decryptRes.json();
              setApiKey(d.plaintext);
              setShowApiKey(true);
              // Auto-hide after 5 seconds
              setTimeout(() => {
                setShowApiKey(false);
                setApiKey('••••••••••');
              }, 5000);
              return;
            }
          } catch {
            // fall through
          }
        }
      }
    } catch {
      // ignore
    }
    // Local toggle (no backend yet)
    setShowApiKey(!showApiKey);
  };

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-8" data-testid="admin-models-header">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]" data-testid="admin-models-title">模型配置</h2>
        </div>
        <div data-testid="model-page-header" style={{ display: 'none' }} />

        {/* Toast */}
        {toast && (
          <div style={{ position: 'fixed', top: 20, right: 20, zIndex: 9999,
            background: toast.type === 'success' ? 'rgba(16,185,129,0.9)' : 'rgba(239,68,68,0.9)',
            color: '#fff', padding: '12px 20px', borderRadius: '8px', fontSize: '14px', fontWeight: 500,
          }}>
            {toast.message}
          </div>
        )}

        {/* LLM Model Config Card */}
        <div className="glass" data-testid="model-llm-card" style={{ padding: '24px', marginBottom: '20px' }}>
          <h3 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '20px' }}>
            默认 LLM 模型配置
          </h3>

          {/* API URL */}
          <ConfigRow label="OpenAI 兼容 API URL" tid="model-api-url">
            <input data-testid="model-api-url-input" value={apiUrl} onChange={(e) => setApiUrl(e.target.value)}
              style={inputStyle} />
            <span data-testid="model-api-url-label" style={{ display: 'none' }} />
          </ConfigRow>

          {/* API Key */}
          <ConfigRow label="API Key" tid="model-api-key">
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <input
                data-testid="model-api-key-input"
                type={showApiKey ? 'text' : 'password'}
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
                placeholder={apiKeyExists ? '••••••••••' : '输入 API Key'}
                style={{ ...inputStyle, flex: 1 }}
              />
              <button
                data-testid="model-api-key-eye-toggle"
                onClick={toggleEye}
                style={{
                  background: 'transparent', border: '1px solid rgba(255,255,255,0.1)',
                  borderRadius: '6px', padding: '6px 10px', cursor: 'pointer',
                  color: 'var(--text-secondary)', fontSize: '16px',
                }}
              >
                {showApiKey ? '🙈' : '👁'}
              </button>
            </div>
            {apiKeyExists && <span data-testid="model-api-key-masked" style={{ display: 'none' }} />}
          </ConfigRow>

          {/* Model Name */}
          <ConfigRow label="Model Name" tid="model-name">
            <select data-testid="model-name-select" value={modelName} onChange={(e) => setModelName(e.target.value)}
              style={inputStyle}>
              <option value="gpt-4o">GPT-4o</option>
              <option value="gpt-4o-mini">GPT-4o-mini</option>
              <option value="claude-3.5-sonnet">Claude 3.5 Sonnet</option>
              <option value="gemini-2.0-flash">Gemini 2.0 Flash</option>
            </select>
          </ConfigRow>

          {/* Context Length */}
          <ConfigRow label="上下文长度限制" tid="model-context-len">
            <Stepper
              value={contextLen}
              onPlus={() => setContextLen((v) => v + 32000)}
              onMinus={() => setContextLen((v) => Math.max(32000, v - 32000))}
              plusTid="model-context-len-plus"
              minusTid="model-context-len-minus"
              tid="model-context-len"
            />
          </ConfigRow>

          {/* Max Output */}
          <ConfigRow label="最大输出长度" tid="model-max-output">
            <Stepper
              value={maxOutput}
              onPlus={() => setMaxOutput((v) => v + 4000)}
              onMinus={() => setMaxOutput((v) => Math.max(4000, v - 4000))}
              plusTid="model-max-output-plus"
              minusTid="model-max-output-minus"
              tid="model-max-output"
            />
          </ConfigRow>

          {/* Temperature */}
          <ConfigRow label="Temperature" tid="model-temperature">
            <input data-testid="model-temperature" type="number" step="0.1" min="0" max="2"
              value={temperature} onChange={(e) => setTemperature(e.target.value)} style={{ ...inputStyle, width: '120px' }} />
          </ConfigRow>

          {/* Top-P */}
          <ConfigRow label="Top-P" tid="model-top-p">
            <input data-testid="model-top-p" type="number" step="0.05" min="0" max="1"
              value={topP} onChange={(e) => setTopP(e.target.value)} style={{ ...inputStyle, width: '120px' }} />
          </ConfigRow>
        </div>

        {/* Hermes Config Card */}
        <div className="glass" data-testid="model-hermes-card" style={{ padding: '24px', marginBottom: '20px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '20px' }}>
            <h3 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)' }}>
              Hermes 自由探索模式
            </h3>
            <span data-testid="model-hermes-badge" style={{
              display: 'inline-block', padding: '2px 10px', borderRadius: '10px',
              background: 'rgba(16,185,129,0.15)', color: '#10b981', fontSize: '11px', fontWeight: 500,
            }}>
              独立服务
            </span>
          </div>

          <ConfigRow label="Hermes API URL" tid="model-hermes-url">
            <input data-testid="model-hermes-url" value={hermesUrl} onChange={(e) => setHermesUrl(e.target.value)}
              style={inputStyle} />
          </ConfigRow>

          <ConfigRow label="Hermes API Key" tid="model-hermes-api-key">
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <input
                data-testid="model-hermes-api-key"
                type={showHermesKey ? 'text' : 'password'}
                value={hermesApiKey}
                onChange={(e) => setHermesApiKey(e.target.value)}
                placeholder="输入 Hermes API Key"
                style={{ ...inputStyle, flex: 1 }}
              />
              <button onClick={() => setShowHermesKey(!showHermesKey)}
                style={{ background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '6px',
                  padding: '6px 10px', cursor: 'pointer', color: 'var(--text-secondary)', fontSize: '16px' }}>
                {showHermesKey ? '🙈' : '👁'}
              </button>
            </div>
          </ConfigRow>

          <ConfigRow label="默认模型" tid="model-hermes-model">
            <input value="hermes-3-70b" disabled
              style={{ ...inputStyle, opacity: 0.5, cursor: 'not-allowed' }} />
          </ConfigRow>
        </div>

        {/* Save Button */}
        <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
          <button
            data-testid="model-save-btn"
            onClick={handleSave}
            disabled={saving}
            style={{
              background: saving ? 'rgba(92,124,250,0.5)' : 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
              color: '#fff', border: 'none', borderRadius: '8px',
              padding: '10px 24px', fontSize: '14px', fontWeight: 600, cursor: 'pointer',
            }}
          >
            {saving ? '保存中...' : '保存配置'}
          </button>
        </div>
      </div>
    </AppLayout>
  );
}

// ── Reusable Components ──

function ConfigRow({ label, tid, children }: { label: string; tid: string; children: React.ReactNode }) {
  return (
    <div style={{ padding: '12px 0', borderBottom: '1px solid rgba(255,255,255,0.06)' }}>
      <label style={{ display: 'block', fontSize: '14px', color: '#7A7A7A', marginBottom: '8px' }}>
        {label}
      </label>
      {children}
    </div>
  );
}

function Stepper({ value, onPlus, onMinus, plusTid, minusTid, tid }: {
  value: number;
  onPlus: () => void;
  onMinus: () => void;
  plusTid: string;
  minusTid: string;
  tid: string;
}) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
      <button data-testid={minusTid} onClick={onMinus}
        style={{ width: '28px', height: '28px', borderRadius: '6px', border: '1px solid rgba(255,255,255,0.1)',
          background: 'rgba(255,255,255,0.06)', color: 'var(--text-secondary)',
          fontSize: '16px', cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        −
      </button>
      <span data-testid={tid} style={{
        fontFamily: "'IBM Plex Mono', monospace", fontSize: '14px', color: 'var(--text-primary)',
        minWidth: '60px', textAlign: 'center',
      }}>
        {value.toLocaleString()} tokens
      </span>
      <button data-testid={plusTid} onClick={onPlus}
        style={{ width: '28px', height: '28px', borderRadius: '6px', border: '1px solid rgba(255,255,255,0.1)',
          background: 'rgba(255,255,255,0.06)', color: 'var(--text-secondary)',
          fontSize: '16px', cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        +
      </button>
    </div>
  );
}

const inputStyle: React.CSSProperties = {
  width: '100%', padding: '8px 12px', background: 'rgba(255,255,255,0.06)',
  border: '1px solid rgba(255,255,255,0.1)', borderRadius: '8px', fontSize: '14px',
  color: 'var(--text-primary)', outline: 'none', boxSizing: 'border-box',
};
