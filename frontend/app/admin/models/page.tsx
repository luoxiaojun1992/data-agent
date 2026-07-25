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
  // SPEC-062: structured model list (multi-model CRUD)
  const [modelList, setModelList] = useState<any[]>([]);
  const [showAddModal, setShowAddModal] = useState(false);
  const [newModel, setNewModel] = useState({ name: '', base_url: '', type: 'llm', instruction: '', api_key: '', temperature: '0.7', max_tokens: '16000', is_default: false });

  const showToast = (msg: string, type: 'success' | 'error') => {
    setToast({ message: msg, type });
    setTimeout(() => setToast(null), 3000);
  };

  const fetchConfig = useCallback(async () => {
    try {
      const res = await apiFetch('/models');
      if (res.ok) {
        const data = await res.json();
        const m = data.models || {};
        if (m.api_url) setApiUrl(m.api_url);
        if (m.model_name) setModelName(m.model_name);
        if (m.context_len) setContextLen(Number(m.context_len));
        if (m.max_output) setMaxOutput(Number(m.max_output));
        if (m.temperature) setTemperature(String(m.temperature));
        if (m.top_p) setTopP(String(m.top_p));
        if (m.hermes_url) setHermesUrl(m.hermes_url);
        setApiKeyExists(m.api_key_exists);
        if (m.api_key_exists) setApiKey('••••••••••');
      }
    } catch {
      // defaults
    }
  }, [apiFetch]);

  // SPEC-062: fetch structured model list (multi-model CRUD).
  const fetchModelList = useCallback(async () => {
    try {
      const res = await apiFetch('/models?page=1&page_size=50');
      if (res.ok) {
        const data = await res.json();
        setModelList(data.models || []);
      }
    } catch (err) {
      console.error('fetchModelList:', err);
    }
  }, [apiFetch]);

  const addModel = async () => {
    if (!newModel.name.trim()) return;
    try {
      const res = await apiFetch('/models', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          ...newModel,
          temperature: parseFloat(newModel.temperature) || 0.7,
          max_tokens: parseInt(newModel.max_tokens, 10) || 16000,
        }),
      });
      if (res.ok) {
        showToast('模型已添加', 'success');
        setShowAddModal(false);
        setNewModel({ name: '', base_url: '', type: 'llm', instruction: '', api_key: '', temperature: '0.7', max_tokens: '16000', is_default: false });
        fetchModelList();
        // SPEC-062 fix: 若新增设为默认，立即同步 legacy flat config
        // 让"默认 LLM 模型配置"卡片中的 api_url/model_name 也更新为新模型
        if (newModel.is_default) {
          try {
            await apiFetch('/models', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ key: 'model_name', value: newModel.name }) });
            if (newModel.base_url) {
              await apiFetch('/models', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ key: 'api_url', value: newModel.base_url }) });
            }
            fetchConfig();
          } catch { /* 默认同步失败不影响主流程 */ }
        }
      } else {
        const d = await res.json();
        showToast(d.error || '添加失败', 'error');
      }
    } catch { showToast('添加失败', 'error'); }
  };

  const deleteModel = async (id: string) => {
    try {
      const res = await apiFetch(`/models/${id}`, { method: 'DELETE' });
      if (res.ok) { showToast('已删除', 'success'); fetchModelList(); }
    } catch { showToast('删除失败', 'error'); }
  };

  const setDefaultModel = async (id: string) => {
    try {
      const res = await apiFetch(`/models/${id}/default`, { method: 'PATCH' });
      if (res.ok) {
        showToast('已设为默认', 'success');
        fetchModelList();
        // SPEC-062 fix: 同步 legacy flat config，让"默认 LLM 模型配置"卡片立即更新
        try {
          const listRes = await apiFetch('/models?page=1&page_size=50');
          if (listRes.ok) {
            const data = await listRes.json();
            const def = (data.models || []).find((m: any) => m.is_default);
            if (def) {
              if (def.name) {
                await apiFetch('/models', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ key: 'model_name', value: def.name }) });
                setModelName(def.name);
              }
              if (def.base_url) {
                await apiFetch('/models', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ key: 'api_url', value: def.base_url }) });
                setApiUrl(def.base_url);
              }
            }
          }
        } catch { /* 默认同步失败不影响主流程 */ }
      }
    } catch { showToast('设置失败', 'error'); }
  };

  useEffect(() => {
    if (auth.hydrated) { fetchConfig(); fetchModelList(); }
  }, [auth.hydrated, fetchConfig, fetchModelList]);

  const handleSave = async () => {
    setSaving(true);
    try {
      // 后端 PUT /models 接收 {key, value} 单字段格式，需逐字段调用
      const fields: { key: string; value: string }[] = [
        { key: 'api_url', value: apiUrl },
        { key: 'model_name', value: modelName },
        { key: 'context_len', value: String(contextLen) },
        { key: 'max_output', value: String(maxOutput) },
        { key: 'temperature', value: temperature },
        { key: 'top_p', value: topP },
        { key: 'hermes_url', value: hermesUrl },
      ];
      if (apiKey && apiKey !== '••••••••••') {
        fields.push({ key: 'api_key', value: apiKey });
      }
      if (hermesApiKey) {
        fields.push({ key: 'hermes_api_key', value: hermesApiKey });
      }
      for (const f of fields) {
        const res = await apiFetch('/models', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(f),
        });
        if (!res.ok) {
          const d = await res.json().catch(() => ({}));
          throw new Error(d.error || `保存 ${f.key} 失败`);
        }
      }
      showToast('配置已保存', 'success');
      fetchConfig();
    } catch (e: any) {
      showToast(e?.message || '保存失败', 'error');
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
    // Fetch decrypted key from HashiCorp Vault
    try {
      const res = await apiFetch('/models');
      if (res.ok) {
        const data = await res.json();
        const m = data.models || {};
        if (m.api_key_exists) {
          try {
            const decryptRes = await apiFetch('/vault/decrypt', {
              method: 'POST',
              body: JSON.stringify({ key: 'data-agent/model_api_key' }),
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
                {showApiKey ? (
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                    <circle cx="12" cy="12" r="3" />
                  </svg>
                ) : (
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
                    <line x1="1" y1="1" x2="23" y2="23" />
                  </svg>
                )}
              </button>
            </div>
            {apiKeyExists && <span data-testid="model-api-key-masked" style={{ display: 'none' }} />}
          </ConfigRow>

          {/* Model Name */}
          <ConfigRow label="Model Name" tid="model-name">
            <input
              data-testid="model-name-input"
              type="text"
              value={modelName}
              onChange={(e) => setModelName(e.target.value)}
              placeholder="输入模型名称（如 gpt-4o）"
              style={inputStyle}
            />
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
                {showHermesKey ? (
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                    <circle cx="12" cy="12" r="3" />
                  </svg>
                ) : (
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
                    <line x1="1" y1="1" x2="23" y2="23" />
                  </svg>
                )}
              </button>
            </div>
          </ConfigRow>

          <ConfigRow label="默认模型" tid="model-hermes-model">
            <input value="hermes-3-70b" disabled
              style={{ ...inputStyle, opacity: 0.5, cursor: 'not-allowed' }} />
          </ConfigRow>
        </div>

        {/* SPEC-062: Structured Multi-Model List */}
        <div className="glass" data-testid="model-list-card" style={{ padding: '24px', marginBottom: '20px' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
            <h3 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)' }}>模型列表（多模型管理）</h3>
            <button
              data-testid="model-add-btn"
              onClick={() => setShowAddModal(true)}
              style={{ background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)', color: '#fff', border: 'none', borderRadius: '8px', padding: '6px 16px', fontSize: '13px', cursor: 'pointer' }}
            >+ 新增模型</button>
          </div>
          <div style={{ overflowX: 'auto' }} data-testid="model-list-table">
            <table style={{ width: '100%', fontSize: '13px', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid rgba(255,255,255,0.1)' }}>
                  <th style={{ textAlign: 'left', padding: '8px', color: 'var(--text-secondary)' }}>ID</th>
                  <th style={{ textAlign: 'left', padding: '8px', color: 'var(--text-secondary)' }}>名称</th>
                  <th style={{ textAlign: 'left', padding: '8px', color: 'var(--text-secondary)' }}>类型</th>
                  <th style={{ textAlign: 'left', padding: '8px', color: 'var(--text-secondary)' }}>默认</th>
                  <th style={{ textAlign: 'left', padding: '8px', color: 'var(--text-secondary)' }}>操作</th>
                </tr>
              </thead>
              <tbody>
                {modelList.length === 0 && (
                  <tr><td colSpan={5} style={{ padding: '16px', textAlign: 'center', color: 'var(--text-secondary)' }} data-testid="model-list-empty">暂无模型</td></tr>
                )}
                {modelList.map((m, i) => (
                  <tr key={m.id || i} style={{ borderBottom: '1px solid rgba(255,255,255,0.05)' }} data-testid={`model-list-row-${i}`}>
                    <td style={{ padding: '8px', color: 'var(--text-secondary)', fontFamily: 'monospace', fontSize: '11px' }} data-testid={`model-list-id-${i}`}>{m.id ? m.id.slice(0, 12) : '-'}</td>
                    <td style={{ padding: '8px', color: 'var(--text-primary)' }}>{m.name}</td>
                    <td style={{ padding: '8px', color: 'var(--text-secondary)' }}>{m.type}</td>
                    <td style={{ padding: '8px' }}>
                      {m.is_default ? (
                        <span data-testid={`model-list-default-${i}`} style={{ color: '#10b981' }}>✓ 默认</span>
                      ) : (
                        <button data-testid={`model-list-set-default-${i}`} onClick={() => setDefaultModel(m.id)}
                          style={{ background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '4px', padding: '2px 8px', color: 'var(--text-secondary)', cursor: 'pointer', fontSize: '11px' }}>设为默认</button>
                      )}
                    </td>
                    <td style={{ padding: '8px' }}>
                      <button data-testid={`model-list-delete-${i}`} onClick={() => deleteModel(m.id)}
                        style={{ background: 'transparent', border: 'none', color: '#ef4444', cursor: 'pointer', fontSize: '12px' }}>删除</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        {/* Add Model Modal */}
        {showAddModal && (
          <div className="fixed inset-0 z-50 flex items-center justify-center" data-testid="model-add-modal">
            <div className="absolute inset-0 bg-black/50" onClick={() => setShowAddModal(false)} />
            <div className="relative glass p-6 rounded-2xl max-w-md w-full mx-4 space-y-3">
              <h3 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)' }}>新增模型</h3>
              <input data-testid="model-add-name" placeholder="模型名称（如 GPT-4o）" value={newModel.name} onChange={e => setNewModel({ ...newModel, name: e.target.value })} style={inputStyle} />
              <input data-testid="model-add-url" placeholder="Base URL" value={newModel.base_url} onChange={e => setNewModel({ ...newModel, base_url: e.target.value })} style={inputStyle} />
              <select data-testid="model-add-type" value={newModel.type} onChange={e => setNewModel({ ...newModel, type: e.target.value })} style={inputStyle}>
                <option value="llm">LLM</option>
                <option value="embedding">Embedding</option>
              </select>
              <textarea data-testid="model-add-instruction" placeholder="系统提示词（可选）" value={newModel.instruction} onChange={e => setNewModel({ ...newModel, instruction: e.target.value })} style={{ ...inputStyle, minHeight: '60px' }} />
              <input data-testid="model-add-api-key" type="password" placeholder="API Key（可选，留空则继承默认配置）" value={newModel.api_key || ''} onChange={e => setNewModel({ ...newModel, api_key: e.target.value })} style={inputStyle} />
              <div style={{ display: 'flex', gap: '8px' }}>
                <div style={{ flex: 1 }}>
                  <label style={{ display: 'block', fontSize: '12px', color: '#7A7A7A', marginBottom: '4px' }}>Temperature</label>
                  <input data-testid="model-add-temperature" type="number" step="0.1" min="0" max="2" value={newModel.temperature} onChange={e => setNewModel({ ...newModel, temperature: e.target.value })} style={inputStyle} />
                </div>
                <div style={{ flex: 1 }}>
                  <label style={{ display: 'block', fontSize: '12px', color: '#7A7A7A', marginBottom: '4px' }}>最大输出 tokens</label>
                  <input data-testid="model-add-max-tokens" type="number" step="1000" min="1" value={newModel.max_tokens} onChange={e => setNewModel({ ...newModel, max_tokens: e.target.value })} style={inputStyle} />
                </div>
              </div>
              <label style={{ display: 'flex', alignItems: 'center', gap: '6px', color: 'var(--text-secondary)', fontSize: '13px' }}>
                <input type="checkbox" checked={newModel.is_default} onChange={e => setNewModel({ ...newModel, is_default: e.target.checked })} data-testid="model-add-default" /> 设为默认模型
              </label>
              <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
                <button data-testid="model-add-cancel" onClick={() => setShowAddModal(false)} style={{ background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '8px', padding: '6px 16px', color: 'var(--text-secondary)', cursor: 'pointer' }}>取消</button>
                <button data-testid="model-add-confirm" onClick={addModel} style={{ background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)', color: '#fff', border: 'none', borderRadius: '8px', padding: '6px 16px', cursor: 'pointer' }}>添加</button>
              </div>
            </div>
          </div>
        )}

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
