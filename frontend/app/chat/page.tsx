'use client';

import React, { useState, useRef, useEffect, useCallback } from 'react';
import AppLayout from '../providers';
import { useAuth } from '@/lib/api';
import ModelSelector from '../components/ModelSelector';

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1';

interface Message {
  role: 'user' | 'assistant';
  content: string;
  timestamp: Date;
  type?: 'text' | 'tool_call' | 'tool_result';
  eventId?: string;
  name?: string;
  args?: Record<string, any>;
  result?: unknown;
  toolCall?: { name: string; input: string; output: string };
  table?: { headers: string[]; rows: string[][] };
}

type WireChatEvent = {
  type?: 'text' | 'tool_call' | 'tool_result';
  role?: string;
  event_id?: string;
  content?: string;
  name?: string;
  args?: Record<string, any>;
  result?: unknown;
  response?: unknown; // backwards compatibility with older servers
  timestamp?: string;
  choices?: { delta?: { content?: string } }[];
};

function normalizeChatMessage(raw: WireChatEvent): Message {
  const result = raw.result !== undefined ? raw.result : raw.response;
  return {
    role: raw.role === 'user' ? 'user' : 'assistant',
    content: raw.content || '',
    type: raw.type || 'text',
    eventId: raw.event_id,
    name: raw.name,
    args: raw.args,
    result,
    timestamp: new Date(raw.timestamp || Date.now()),
  };
}

/** Apply one canonical event exactly as the history endpoint does. */
function appendChatEvent(messages: Message[], raw: WireChatEvent): Message[] {
  const message = normalizeChatMessage(raw);
  if (message.type === 'text' && !message.content) return messages;

  const last = messages[messages.length - 1];
  if (message.type === 'text' && last && last.type === 'text' && last.role === message.role) {
    return [...messages.slice(0, -1), { ...last, content: last.content + message.content }];
  }
  return [...messages, message];
}

function formatPayload(value: unknown): string {
  if (typeof value === 'string') return value;
  const text = JSON.stringify(value, null, 2);
  return text === undefined ? String(value) : text;
}

function hasPayload(value: unknown): boolean {
  return value !== undefined && value !== null;
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
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [copyMsg, setCopyMsg] = useState<string | null>(null);
  const [showPromptModal, setShowPromptModal] = useState(false);
  const [customPrompts, setCustomPrompts] = useState<string[]>(() => {
    try { return JSON.parse(localStorage.getItem('customPrompts') || '[]'); } catch { return []; }
  });
  const [newPromptText, setNewPromptText] = useState('');
  const [enhancing, setEnhancing] = useState(false);
  const [sessions, setSessions] = useState<any[]>([]);
  const [sessionsTotal, setSessionsTotal] = useState(0);
  const [sessionsPage, setSessionsPage] = useState(1);
  const PAGE_SIZE = 15;
  const [showSessions, setShowSessions] = useState(false);
  const [sessionSearch, setSessionSearch] = useState('');
  const [selectedModel, setSelectedModel] = useState<string>(''); // SPEC-062: model bound to new session
  const pendingEventsRef = useRef<WireChatEvent[]>([]);
  const flushTimerRef = useRef<NodeJS.Timeout | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  useEffect(() => () => {
    if (flushTimerRef.current) clearTimeout(flushTimerRef.current);
  }, []);

  const createSession = async () => {
    try {
      const res = await apiFetch('/sessions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ model_id: selectedModel }),
      });
      const data = await res.json();
      setSessionId(data.session_id);
      if (data.model_id) setSelectedModel(data.model_id); // sync resolved default
      return data.session_id;
    } catch (err) {
      console.error('Failed to create session:', err);
      return null;
    }
  };

  const newSession = () => { setMessages([]); setSessionId(null); setInput(''); setSelectedModel(''); };

  const fetchSessions = useCallback(async () => {
    if (!auth.token) return;
    try {
      const res = await apiFetch(`/sessions?page=${sessionsPage}&page_size=${PAGE_SIZE}`);
      const data = await res.json();
      setSessions(data.sessions || []);
      setSessionsTotal(data.total || 0);
    } catch { /* ignore */ }
  }, [sessionsPage, auth.token]);

  useEffect(() => {
    if (showSessions) fetchSessions();
  }, [showSessions, sessionsPage]);

  const loadSessionMessages = async (id: string, preserveOnError = false) => {
    try {
      const res = await apiFetch(`/sessions/${id}/messages`);
      if (!res.ok) throw new Error(`Failed to load session messages (${res.status})`);
      const data = await res.json();
      // The history endpoint and the SSE stream share the same canonical event
      // schema. Do not cap or otherwise reshape the uncompressed transcript.
      const msgs: Message[] = (data.messages || []).map((m: WireChatEvent) => normalizeChatMessage(m));
      setMessages(msgs);
      return true;
    } catch (err) {
      console.error('Failed to load session messages:', err);
      if (!preserveOnError) setMessages([]);
      return false;
    }
  };

  const selectSession = (id: string) => {
    setSessionId(id);
    loadSessionMessages(id);
    setShowSessions(false);
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
    const userMsg: Message = { role: 'user', content: input, type: 'text', timestamp: new Date() };
    setMessages(prev => [...prev, userMsg]);
    setInput('');
    setStreaming(true);
    pendingEventsRef.current = [];

    let sid = sessionId;
    if (!sid) sid = await createSession();
    if (!sid) { setStreaming(false); return; }

    const FLUSH_INTERVAL = 80; // ms — batch updates without changing event order
    let streamCompleted = false;
    let streamErrored = false;

    const flushToState = () => {
      const pending = pendingEventsRef.current.splice(0);
      if (pending.length === 0) return;
      setMessages(prev => pending.reduce((next, event) => appendChatEvent(next, event), prev));
    };

    const queueEvent = (event: WireChatEvent) => {
      if (event.type === 'text' || event.type === 'tool_call' || event.type === 'tool_result') {
        pendingEventsRef.current.push(event);
        return;
      }
      // Preserve compatibility with OpenAI-style text deltas from older
      // gateways while still routing them through the canonical message path.
      const chunk = event.content || event.choices?.[0]?.delta?.content;
      if (chunk) {
        pendingEventsRef.current.push({ type: 'text', content: chunk });
      }
    };

    try {
      const base = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1';
      const endpoint = `${base}/chat`;
      const res = await fetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${auth.token}` },
        body: JSON.stringify({ session_id: sid, message: userMsg.content, stream: true, model: selectedModel }),
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
          if (!line.startsWith('data: ')) continue;
          const data = line.slice(6).trim();
          if (data === '[DONE]') {
            streamCompleted = true;
            continue;
          }
          try {
            const parsed = JSON.parse(data) as WireChatEvent & { error?: string; session_id?: string };
            if (parsed.session_id) {
              setSessionId(parsed.session_id);
              continue;
            }
            if (parsed.error) {
              streamErrored = true;
              queueEvent({ type: 'text', content: `Error: ${parsed.error}` });
              continue;
            }
            queueEvent(parsed);
          } catch {
            // Ignore malformed keep-alive lines; the completed persisted
            // transcript is reloaded below when the stream finishes.
          }
        }
        if (flushTimerRef.current) clearTimeout(flushTimerRef.current);
        flushTimerRef.current = setTimeout(flushToState, FLUSH_INTERVAL);
      }
    } catch (err: any) {
      streamErrored = true;
      queueEvent({ type: 'text', content: err?.message || 'Error: Failed to get response from server.' });
    } finally {
      if (flushTimerRef.current) clearTimeout(flushTimerRef.current);
      flushToState();
      // The server persists every ADK event before yielding it. Reloading the
      // canonical history after [DONE] makes the final live transcript byte-
      // for-byte equivalent to opening the same session later, including all
      // currently uncompressed text/tool events. When the stream errored we
      // keep the live transcript (which already contains the error message)
      // instead of overwriting it with whatever is in MongoDB.
      if (streamCompleted && !streamErrored) await loadSessionMessages(sid, true);
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
          {/* Header */}
          <div className="mb-4 flex items-center justify-between" data-testid="chat-header">
            <div>
              <h2 className="text-2xl font-bold text-[var(--text-primary)]">Chat 对话</h2>
              <p className="text-sm text-[var(--text-secondary)] mt-1" data-testid="chat-session-info">
                {sessionId ? `Session: ${sessionId.slice(0, 8)}...` : '创建新会话'}
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
              <ModelSelector
                value={selectedModel}
                onChange={setSelectedModel}
                disabled={!!sessionId}
                token={auth.token}
              />
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
                  {msg.role === 'assistant' && msg.type === 'tool_call' ? (
                    <div className="text-xs" data-testid={`chat-live-tool-call-${i}`}>
                      <span className="text-[var(--accent)]">🔧 {msg.name || 'tool'}</span>
                      {msg.args && Object.keys(msg.args).length > 0 && (
                        <details className="mt-1">
                          <summary className="cursor-pointer text-[var(--text-secondary)]">参数</summary>
                          <pre className="mt-1 p-2 rounded text-[10px] bg-black/20 overflow-x-auto max-h-32">
                            {formatPayload(msg.args)}
                          </pre>
                        </details>
                      )}
                    </div>
                  ) : msg.role === 'assistant' && msg.type === 'tool_result' ? (
                    <div className="text-xs" data-testid={`chat-live-tool-result-${i}`}>
                      <span className="text-green-400">✅ {msg.name || 'tool'}</span>
                      {hasPayload(msg.result) && (
                        <details className="mt-1">
                          <summary className="cursor-pointer text-[var(--text-secondary)]">结果</summary>
                          <pre className="mt-1 p-2 rounded text-[10px] bg-black/20 overflow-x-auto max-h-32">
                            {formatPayload(msg.result)}
                          </pre>
                        </details>
                      )}
                    </div>
                  ) : msg.role === 'assistant' ? (
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
          <div className="w-72 flex-shrink-0 border-l border-[var(--border-glass)] h-full overflow-y-auto bg-[var(--bg-secondary)]/30" data-testid="session-sidebar">
            <div className="p-3">
              <div className="flex items-center justify-between mb-2">
                <p className="text-xs font-semibold text-[var(--text-primary)]">历史会话</p>
                <button onClick={() => setShowSessions(false)} className="text-xs text-[var(--text-secondary)]">关闭</button>
              </div>
              <input type="text" placeholder="搜索会话..."
                value={sessionSearch} onChange={e => setSessionSearch(e.target.value)}
                className="w-full px-3 py-1.5 text-xs rounded-lg bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-primary)] placeholder-[var(--text-secondary)] focus:outline-none mb-2"
                data-testid="session-search" />
            <div data-testid="session-list">
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
              {sessions.filter(s => !sessionSearch || (s.title || s.id).toLowerCase().includes(sessionSearch.toLowerCase())).map(s => (
                  <button key={s.id} onClick={() => selectSession(s.id)}
                    className={`w-full text-left px-2 py-1.5 text-xs hover:bg-white/5 rounded transition-colors ${s.id === sessionId ? 'bg-[var(--accent)]/10' : ''}`}
                    data-testid={`session-item-${s.id}`}>
                    <div className="flex items-start justify-between gap-1">
                      <span className="text-[var(--text-primary)] line-clamp-2 break-all" data-testid="session-item-title">
                        {s.title || `Session ${s.id.slice(-8)}`}
                      </span>
                      <button onClick={e => deleteSession(s.id, e)}
                        className="flex-shrink-0 text-[10px] text-red-400 hover:text-red-300"
                        data-testid={`session-delete-${s.id}`}>删除</button>
                    </div>
                    <span className="text-[var(--text-secondary)] text-[10px]" data-testid="session-item-meta">{new Date(s.created_at).toLocaleDateString()}</span>
                  </button>
                ))}
              </div>

              {/* Pagination */}
              {sessionsTotal > PAGE_SIZE && (
                <div className="flex items-center justify-between mt-2 text-xs" data-testid="session-pagination">
                  <button onClick={() => setSessionsPage(p => Math.max(1, p - 1))} disabled={sessionsPage === 1}
                    className="px-2 py-1 rounded border border-[var(--border-glass)] disabled:opacity-40 text-[var(--text-secondary)]">上一页</button>
                  <span className="text-[var(--text-secondary)]">
                    {sessionsPage} / {Math.ceil(sessionsTotal / PAGE_SIZE)}（共 {sessionsTotal} 条）
                  </span>
                  <button onClick={() => setSessionsPage(p => Math.min(Math.ceil(sessionsTotal / PAGE_SIZE), p + 1))} disabled={sessionsPage >= Math.ceil(sessionsTotal / PAGE_SIZE)}
                    className="px-2 py-1 rounded border border-[var(--border-glass)] disabled:opacity-40 text-[var(--text-secondary)]">下一页</button>
                </div>
              )}
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
