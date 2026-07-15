'use client';

import React, { useState, useRef, useEffect } from 'react';
import AppLayout from '../providers';
import { useAuth } from '@/lib/api';

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1';

interface Message {
  role: 'user' | 'assistant';
  content: string;
  timestamp: Date;
  toolCall?: { name: string; input: string; output: string };
  table?: { headers: string[]; rows: string[][] };
}

/** Parse SQL code blocks, tables, and tool calls from markdown content */
function parseBlocks(content: string) {
  const blocks: { type: 'text' | 'sql' | 'table' | 'tool'; text?: string; code?: string; headers?: string[]; rows?: string[][]; tool?: Message['toolCall'] }[] = [];
  const parts = content.split(/(```[\s\S]*?```)/g);
  for (const part of parts) {
    if (part.startsWith('```')) {
      const inner = part.replace(/^```[\w]*\n?/, '').replace(/\n?```$/, '');
      if (/select|insert|update|delete|create|alter|drop|with\b/i.test(inner.trim().slice(0, 10))) {
        blocks.push({ type: 'sql', code: inner.trim() });
      } else {
        try {
          const parsed = JSON.parse(inner.trim());
          if (parsed.type === 'tool_call') {
            blocks.push({ type: 'tool', tool: parsed });
          } else if (parsed.type === 'table') {
            blocks.push({ type: 'table', headers: parsed.headers, rows: parsed.rows });
          } else if (parsed.type === 'kpi') {
            blocks.push({ type: 'kpi', items: parsed.items } as any);
          } else if (parsed.type === 'chart') {
            blocks.push({ type: 'chart', title: parsed.title, labels: parsed.labels, values: parsed.values } as any);
          } else {
            blocks.push({ type: 'text', text: inner.trim() });
          }
        } catch {
          blocks.push({ type: 'text', text: inner.trim() });
        }
      }
    } else if (part.trim()) {
      blocks.push({ type: 'text', text: part.trim() });
    }
  }
  return blocks;
}

function copyToClipboard(text: string) {
  navigator.clipboard.writeText(text).catch(() => {});
}

export default function ChatPage() {
  const { auth, apiFetch } = useAuth();
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [streaming, setStreaming] = useState(false);
  const [mode, setMode] = useState<'analysis' | 'hermes'>('analysis');
  const [hermesOnline, setHermesOnline] = useState(false);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [copyMsg, setCopyMsg] = useState<string | null>(null);
  const [showPromptModal, setShowPromptModal] = useState(false);
  const [customPrompts, setCustomPrompts] = useState<string[]>(() => {
    try { return JSON.parse(localStorage.getItem('customPrompts') || '[]'); } catch { return []; }
  });
  const [newPromptText, setNewPromptText] = useState('');
  const [enhancing, setEnhancing] = useState(false);
  const [sessions, setSessions] = useState<any[]>([]);
  const [showSessions, setShowSessions] = useState(false);
  const [sessionSearch, setSessionSearch] = useState('');
  const streamingRef = useRef<string>('');
  const flushTimerRef = useRef<NodeJS.Timeout | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  useEffect(() => () => {
    if (flushTimerRef.current) clearTimeout(flushTimerRef.current);
  }, []);

  // Check Hermes online status when mode changes
  const checkHermesOnline = async () => {
    try {
      const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1'}/hermes/health`);
      setHermesOnline(res.ok);
    } catch { setHermesOnline(false); }
  };

  const switchMode = async (m: 'analysis' | 'hermes') => {
    setMode(m);
    setMessages([]);
    setSessionId(null);
    if (m === 'hermes') await checkHermesOnline();
  };

  const createSession = async () => {
    try {
      const res = await apiFetch('/sessions', { method: 'POST' });
      const data = await res.json();
      setSessionId(data.session_id);
      return data.session_id;
    } catch (err) {
      console.error('Failed to create session:', err);
      return null;
    }
  };

  const newSession = () => { setMessages([]); setSessionId(null); setInput(''); };

  const fetchSessions = async () => {
    try {
      const res = await apiFetch('/sessions');
      const data = await res.json();
      setSessions(data.sessions || []);
    } catch { /* ignore */ }
  };

  const toggleSessions = () => {
    const next = !showSessions;
    setShowSessions(next);
    if (next) { fetchSessions(); fetchDeletedSessions(); }
  };

  const deleteSession = async (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await apiFetch(`/sessions/${id}`, { method: 'DELETE' });
      setSessions(prev => prev.filter(s => s.id !== id));
      // Refresh deleted list
      fetchDeletedSessions();
    } catch { /* ignore */ }
  };

  const [deletedSessions, setDeletedSessions] = useState<any[]>([]);

  const fetchDeletedSessions = async () => {
    try {
      const res = await apiFetch('/sessions/deleted');
      const data = await res.json();
      setDeletedSessions(data.sessions || []);
    } catch { /* ignore */ }
  };

  const restoreSession = async (id: string) => {
    try {
      await apiFetch(`/sessions/${id}/restore`, { method: 'POST' });
      setDeletedSessions(prev => prev.filter(s => s.id !== id));
      fetchSessions();
    } catch { /* ignore */ }
  };

  const handleEnhance = async () => {
    if (!input.trim() || enhancing) return;
    setEnhancing(true);
    try {
      const headers: Record<string,string> = { 'Content-Type': 'application/json' };
      if (auth.token) headers['Authorization'] = `Bearer ${auth.token}`;
      const res = await fetch(`${API_BASE}/chat/enhance`, {
        method: 'POST', headers,
        body: JSON.stringify({ prompt: input }),
      });
      if (res.ok) {
        const data = await res.json();
        setInput(data.enhanced || input);
      }
    } catch { /* enhancement failed silently */ }
    setEnhancing(false);
  };

  const sendMessage = async () => {
    if (!input.trim() || streaming) return;
    const userMsg: Message = { role: 'user', content: input, timestamp: new Date() };
    setMessages(prev => [...prev.slice(-199), userMsg]); // cap at 200
    setInput('');
    setStreaming(true);

    let sid = sessionId;
    if (!sid) sid = await createSession();
    if (!sid) { setStreaming(false); return; }

    const assistantMsg: Message = { role: 'assistant', content: '', timestamp: new Date() };
    setMessages(prev => [...prev.slice(-199), assistantMsg]);

    streamingRef.current = '';
    const FLUSH_INTERVAL = 80; // ms — batch updates to avoid excessive renders

    const flushToState = () => {
      const content = streamingRef.current;
      setMessages(prev => {
        const copy = [...prev];
        const last = copy[copy.length - 1];
        if (last && last.role === 'assistant') {
          copy[copy.length - 1] = { ...last, content };
        }
        return copy;
      });
    };

    try {
      const base = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1';
      const endpoint = mode === 'hermes' ? `${base}/hermes/chat` : `${base}/chat`;
      const res = await fetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${auth.token}` },
        body: JSON.stringify({ session_id: sid, message: userMsg.content }),
      });
      if (!res.ok) throw new Error('Chat request failed');
      const reader = res.body?.getReader();
      if (!reader) throw new Error('No response stream');
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';
        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const data = line.slice(6);
            if (data === '[DONE]') continue;
            try {
              const parsed = JSON.parse(data);
              const chunk = parsed.content || parsed.choices?.[0]?.delta?.content || '';
              if (chunk) streamingRef.current += chunk;
            } catch { /* skip */ }
          }
        }
        // Batch flush
        if (flushTimerRef.current) clearTimeout(flushTimerRef.current);
        flushTimerRef.current = setTimeout(flushToState, FLUSH_INTERVAL);
      }
    } catch (err: any) {
      streamingRef.current = err?.message || 'Error: Failed to get response from server.';
    } finally {
      if (flushTimerRef.current) clearTimeout(flushTimerRef.current);
      flushToState(); // final flush
      setStreaming(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(); }
  };

  return (
    <AppLayout>
      <div className="flex h-[calc(100vh-64px)] animate-fade-in">
        {/* Main chat area */}
        <div className="flex-1 flex flex-col min-w-0">
          {/* Mode Toggle */}
          <div className="flex gap-1 mb-4" data-testid="mode-toggle">
            <button
              onClick={() => switchMode('analysis')}
              className={`flex-1 py-2 text-xs rounded-lg transition-colors font-medium ${
                mode === 'analysis' ? 'bg-[var(--accent)]/10 text-[var(--accent)] border border-[var(--accent)]/30' : 'text-[var(--text-secondary)] border border-transparent'
              }`}
              data-testid="mode-toggle-analysis"
            >📊 分析模式</button>
            <button
              onClick={() => switchMode('hermes')}
              className={`flex-1 py-2 text-xs rounded-lg transition-colors font-medium ${
                mode === 'hermes' ? 'bg-[var(--accent)]/10 text-[var(--accent)] border border-[var(--accent)]/30' : 'text-[var(--text-secondary)] border border-transparent'
              }`}
              data-testid="mode-toggle-hermes"
            >🔍 探索模式</button>
          </div>
          {/* Header */}
          <div className="mb-4 flex items-center justify-between" data-testid="chat-header">
            <div>
              <h2 className="text-2xl font-bold text-[var(--text-primary)]">{mode === 'hermes' ? 'Hermes 探索' : 'Chat 对话'}</h2>
              <p className="text-sm text-[var(--text-secondary)] mt-1" data-testid="chat-session-info">
                {mode === 'hermes' ? (hermesOnline ? 'Hermes Online' : 'Hermes Offline') : sessionId ? `Session: ${sessionId.slice(0, 8)}...` : '创建新会话'}
              </p>
            </div>
            <div className="flex items-center gap-3">
              <span className="flex items-center gap-2" data-testid="chat-online-badge">
                <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" data-testid="chat-online-dot" />
                <span className="text-xs text-[var(--text-secondary)]">在线</span>
              </span>
              <button
                onClick={newSession}
                className="px-3 py-1.5 text-xs rounded-lg border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors"
                data-testid="chat-new-session-btn"
              >新对话</button>
              <button onClick={toggleSessions}
                className="px-3 py-1.5 text-xs rounded-lg border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors"
                data-testid="chat-session-btn">📋 会话</button>
            </div>
          </div>

          {/* Messages */}
          <div className="flex-1 overflow-y-auto mb-4 space-y-4" data-testid="chat-messages">
            {messages.length === 0 && (
              <div className="flex items-center justify-center h-full">
                <div className="text-center">
                  <span className="text-5xl block mb-4">💬</span>
                  <p className="text-lg text-[var(--text-primary)]">开始数据分析对话</p>
                  <p className="text-sm text-[var(--text-secondary)] mt-2">输入你的数据分析需求，AI 将为你提供帮助</p>
                  <div className="flex flex-wrap justify-center gap-2 mt-4" data-testid="chat-prompt-row">
                    {[
                      { text: '今日数据概览', id: 'chat-prompt-chip-0' },
                      { text: '本月销售趋势', id: 'chat-prompt-chip-1' },
                      { text: '同比环比分析', id: 'chat-prompt-chip-2' },
                      { text: 'TOP10 产品', id: 'chat-prompt-chip-3' },
                    ].map((hint) => (
                      <button
                        key={hint.id}
                        onClick={() => { setInput(hint.text); }}
                        className="px-3 py-1.5 text-xs rounded-full bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:border-[var(--accent)]/50 transition-all"
                        data-testid={hint.id}
                      >{hint.text}</button>
                    ))}
                  </div>
                </div>
              </div>
            )}

            {messages.map((msg, i) => (
              <div key={i} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                {msg.role === 'assistant' && (
                  <div className="w-8 h-8 rounded-full bg-emerald-400/20 flex items-center justify-center text-xs font-bold text-emerald-400 mr-3 flex-shrink-0" data-testid="chat-msg-avatar">DA</div>
                )}
                <div
                  className={`max-w-[70%] p-4 rounded-2xl ${
                    msg.role === 'user'
                      ? 'bg-[var(--accent)] text-white rounded-br-md'
                      : 'glass text-[var(--text-primary)] rounded-bl-md'
                  }`}
                  data-testid={msg.role === 'user' ? `chat-msg-user-${i}` : `chat-msg-ai-${i}`}
                >
                  {msg.role === 'assistant' ? (
                    <ChatContent content={msg.content} copyMsg={copyMsg} setCopyMsg={setCopyMsg} />
                  ) : (
                    <div className="text-sm whitespace-pre-wrap">{msg.content}</div>
                  )}
                  {streaming && i === messages.length - 1 && msg.role === 'assistant' && !msg.content && (
                    <span className="text-sm text-[var(--text-secondary)]" data-testid="chat-loading-indicator">...</span>
                  )}
                  {streaming && i === messages.length - 1 && (
                    <span className="text-xs text-[var(--text-secondary)] ml-2" data-testid="chat-loading-text">分析中…</span>
                  )}
                  <p className="text-xs opacity-60 mt-1">
                    {msg.timestamp.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}
                  </p>
                </div>
              </div>
            ))}
            <div ref={messagesEndRef} />
          </div>

          {/* Prompt modal button + Input */}
          <div className="glass p-4">
            <div className="flex items-center gap-2 mb-2">
              <button
                className="px-3 py-1.5 text-xs rounded-lg border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors"
                data-testid="prompt-btn"
                onClick={() => setShowPromptModal(true)}
              >📋 提示词</button>
              <button
                className="px-3 py-1.5 text-xs rounded-lg border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors"
                data-testid="chat-enhance-btn"
                onClick={handleEnhance}
                disabled={enhancing}
              >{enhancing ? '⏳ 增强中...' : '✨ 增强'}</button>
            </div>
            <div className="flex gap-3">
              <textarea
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder="输入你的数据分析需求..."
                rows={2}
                className="flex-1 px-4 py-3 rounded-xl bg-transparent border-0 text-[var(--text-primary)] placeholder-[var(--text-secondary)] resize-none focus:outline-none"
                data-testid="chat-input"
                disabled={streaming}
              />
              <button
                onClick={sendMessage}
                disabled={streaming || !input.trim()}
                className="px-6 py-2 bg-[var(--accent)] text-white rounded-xl font-medium hover:opacity-90 disabled:opacity-40 transition-all self-end"
                data-testid="chat-send-btn"
              >{streaming ? '发送中...' : '发送'}</button>
            </div>
          </div>
        </div>

        {/* Session Panel */}
        {showSessions && (
          <div className="border-t border-[var(--border-glass)]" data-testid="session-sidebar">
            <div className="p-3">
              <div className="flex items-center justify-between mb-2">
                <p className="text-xs font-semibold text-[var(--text-primary)]">历史会话</p>
                <button onClick={() => setShowSessions(false)} className="text-xs text-[var(--text-secondary)]">关闭</button>
              </div>
              <input type="text" placeholder="搜索会话..."
                value={sessionSearch} onChange={e => setSessionSearch(e.target.value)}
                className="w-full px-3 py-1.5 text-xs rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] placeholder-[var(--text-secondary)] focus:outline-none mb-2"
                data-testid="session-search" />
            <div className="max-h-48 overflow-y-auto" data-testid="session-list">
              {/* Recovery banner for deleted sessions */}
              {deletedSessions.length > 0 && (
                <div data-testid="session-recovery-banner"
                  className="mb-2 p-2 rounded-lg bg-[var(--accent)]/10 border border-[var(--accent)]/20">
                  <p className="text-xs text-[var(--text-secondary)] mb-1">
                    已删除 {deletedSessions.length} 个会话，可在 24 小时内恢复
                  </p>
                  {deletedSessions.map(s => (
                    <button key={s.id} onClick={() => restoreSession(s.id)}
                      data-testid="session-recovery-restore-btn"
                      className="text-xs text-[var(--accent)] hover:underline">
                      恢复 Session {s.id.slice(-8)}
                    </button>
                  ))}
                </div>
              )}
              {sessions.filter(s => !sessionSearch || s.id.includes(sessionSearch)).map(s => (
                  <button key={s.id} onClick={() => { setSessionId(s.id); setMessages([]); }}
                    className={`w-full text-left px-2 py-1.5 text-xs hover:bg-white/5 rounded transition-colors ${s.id === sessionId ? 'bg-[var(--accent)]/10' : ''}`}
                    data-testid={`session-item-${s.id}`}>
                    <span className="text-[var(--text-primary)]" data-testid="session-item-title">Session {s.id.slice(-8)}</span>
                    <span className="text-[var(--text-secondary)] ml-2" data-testid="session-item-meta">{new Date(s.created_at).toLocaleDateString()}</span>
                    <button onClick={e => deleteSession(s.id, e)}
                      className="float-right text-[10px] text-red-400 hover:text-red-300"
                      data-testid={`session-delete-${s.id}`}>删除</button>
                  </button>
                ))}
              </div>
            </div>
          </div>
        )}

        {/* Prompt Modal */}
        {showPromptModal && (
          <div className="fixed inset-0 z-50 flex items-center justify-center" data-testid="prompt-modal">
            <div className="absolute inset-0 bg-black/50 backdrop-blur-sm" onClick={() => setShowPromptModal(false)} />
            <div className="relative glass p-6 rounded-2xl max-w-md w-full mx-4 space-y-4">
              <div className="flex items-center justify-between">
                <h3 className="text-lg font-semibold text-[var(--text-primary)]">提示词</h3>
                <button onClick={() => setShowPromptModal(false)} className="text-[var(--text-secondary)] hover:text-[var(--text-primary)]" data-testid="prompt-modal-close">✕</button>
              </div>
              <div>
                <p className="text-xs text-[var(--text-secondary)] mb-2 uppercase">系统预设</p>
                {['今日数据概览', '本月销售趋势', '同比环比分析', 'TOP10 产品'].map((p, i) => (
                  <button key={i} onClick={() => { setInput(p); setShowPromptModal(false); }}
                    className="w-full text-left px-3 py-2 rounded-lg text-sm text-[var(--text-primary)] hover:bg-white/5 transition-colors"
                    data-testid={`prompt-modal-chip-${i}`}>{p}</button>
                ))}
              </div>
              {customPrompts.length > 0 && (
                <div>
                  <p className="text-xs text-[var(--text-secondary)] mb-2 uppercase">我的常用</p>
                  {customPrompts.map((p, i) => (
                    <button key={i} onClick={() => { setInput(p); setShowPromptModal(false); }}
                      className="w-full text-left px-3 py-2 rounded-lg text-sm text-[var(--text-primary)] hover:bg-white/5 transition-colors"
                      data-testid={`prompt-modal-custom-${i}`}>{p}</button>
                  ))}
                </div>
              )}
              <div className="flex gap-2 pt-2 border-t border-[var(--border-glass)]">
                <input type="text" placeholder="输入自定义提示词..."
                  value={newPromptText} onChange={e => setNewPromptText(e.target.value)}
                  className="flex-1 px-3 py-1.5 text-xs rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] placeholder-[var(--text-secondary)] focus:outline-none"
                  data-testid="prompt-modal-custom-input" />
                <button onClick={() => {
                  if (!newPromptText.trim()) return;
                  const updated = [...customPrompts, newPromptText.trim()].slice(-5);
                  setCustomPrompts(updated);
                  localStorage.setItem('customPrompts', JSON.stringify(updated));
                  setNewPromptText('');
                }} className="px-3 py-1.5 text-xs rounded-lg bg-[var(--accent)] text-white hover:opacity-90"
                  data-testid="prompt-modal-save-btn">保存</button>
              </div>
            </div>
          </div>
        )}
      </div>
    </AppLayout>
  );
}

/** Render markdown-like content with SQL/tables inline */
function ChatContent({ content, copyMsg, setCopyMsg }: { content: string; copyMsg: string | null; setCopyMsg: (v: string | null) => void }) {
  const blocks = parseBlocks(content);
  return (
    <div className="text-sm space-y-3">
      {blocks.map((block, i) => {
        if (block.type === 'sql' && block.code) {
          return (
            <div key={i} className="rounded-lg border border-emerald-400/20 overflow-hidden" data-testid="chat-sql-block">
              <div className="flex items-center justify-between px-3 py-1.5 bg-black/20">
                <span className="text-xs text-emerald-400 flex items-center gap-1.5">
                  <span className="w-1.5 h-1.5 rounded-full bg-emerald-400" />SQL
                </span>
                <button
                  className="text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                  onClick={() => { copyToClipboard(block.code!); setCopyMsg('已复制'); setTimeout(() => setCopyMsg(null), 2000); }}
                  data-testid="chat-sql-copy-btn"
                >{copyMsg || '复制'}</button>
              </div>
              <pre className="p-3 text-xs font-mono text-[#B1E2FF] overflow-x-auto" data-testid="chat-sql-code"><code>{block.code}</code></pre>
            </div>
          );
        }
        if (block.type === 'table' && block.headers && block.rows) {
          return (
            <div key={i} className="overflow-x-auto" data-testid="chat-table">
              <table className="w-full text-xs">
                <thead>
                  <tr>
                    {block.headers.map((h, idx) => (
                      <th key={idx} className="px-2 py-1.5 text-left text-[var(--text-secondary)] font-medium uppercase" data-testid={`chat-table-header-${idx}`}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {block.rows.map((row, ri) => (
                    <tr key={ri} className={ri % 2 === 0 ? 'bg-white/5' : ''}>
                      {row.map((cell, ci) => (
                        <td key={ci} className="px-2 py-1 text-[var(--text-secondary)]">{cell}</td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          );
        }
        if (block.type === 'tool' && block.tool) {
          const [expanded, setExpanded] = React.useState(false);
          return (
            <div key={i} className="rounded-lg border border-[var(--border-glass)] overflow-hidden" data-testid={`chat-tool-call-card-${i}`}>
              <button
                className="w-full flex items-center justify-between px-3 py-2 hover:bg-white/5 transition-colors"
                onClick={() => setExpanded(!expanded)}
                data-testid="chat-tool-call-header"
              >
                <span className="text-xs flex items-center gap-2">
                  <span className="w-6 h-6 rounded-lg bg-[var(--accent)]/20 flex items-center justify-center text-[var(--accent)]">🔧</span>
                  <span className="font-medium">{block.tool.name}</span>
                </span>
                <span className={`text-xs transform transition-transform ${expanded ? 'rotate-180' : ''}`}>▼</span>
              </button>
              {expanded && (
                <div className="px-3 py-2 border-t border-[var(--border-glass)] text-xs space-y-2" data-testid="chat-tool-call-body">
                  <div><span className="text-[var(--text-secondary)]">输入参数：</span>{block.tool.input}</div>
                  <div><span className="text-[var(--text-secondary)]">输出结果：</span>{block.tool.output}</div>
                </div>
              )}
            </div>
          );
        }
        if ((block as any).type === 'kpi' && (block as any).items) {
          const items = (block as any).items as { label: string; value: string }[];
          return (
            <div key={i} className="flex flex-wrap gap-3 p-3 rounded-lg bg-white/5" data-testid="chat-inline-kpi">
              {items.map((item, idx) => (
                <div key={idx} className="text-center min-w-[80px]">
                  <div className="text-lg font-mono font-bold text-[var(--accent)]" data-testid="chat-inline-kpi-val">{item.value}</div>
                  <div className="text-[10px] text-[var(--text-secondary)]" data-testid="chat-inline-kpi-lbl">{item.label}</div>
                </div>
              ))}
            </div>
          );
        }
        if ((block as any).type === 'chart') {
          const { title, labels, values } = block as any;
          const max = Math.max(...values, 1);
          const h = (v: number) => Math.max(4, (v / max) * 72);
          return (
            <div key={i} className="p-3 rounded-lg bg-white/5" data-testid="chat-inline-chart">
              {title && <p className="text-xs font-medium text-[var(--text-secondary)] mb-2">{title}</p>}
              <div className="flex items-end gap-1" style={{ height: '80px' }}>
                {labels.map((l: string, idx: number) => (
                  <div key={idx} className="flex-1 flex flex-col items-center gap-1">
                    <div className="w-full bg-[var(--accent)]/60 rounded-t" style={{ height: `${h(values[idx])}px` }} />
                    <span className="text-[9px] text-[var(--text-secondary)]">{l}</span>
                  </div>
                ))}
              </div>
            </div>
          );
        }
        return <div key={i} className="whitespace-pre-wrap leading-relaxed">{block.text}</div>;
      })}
    </div>
  );
}
