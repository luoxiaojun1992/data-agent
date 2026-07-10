'use client';

import React, { useState, useRef, useEffect } from 'react';
import AppLayout from '../providers';
import { useAuth } from '@/lib/api';

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
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

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

  const sendMessage = async () => {
    if (!input.trim() || streaming) return;
    const userMsg: Message = { role: 'user', content: input, timestamp: new Date() };
    setMessages((prev) => [...prev, userMsg]);
    setInput('');
    setStreaming(true);

    let sid = sessionId;
    if (!sid) sid = await createSession();
    if (!sid) { setStreaming(false); return; }

    const assistantMsg: Message = { role: 'assistant', content: '', timestamp: new Date() };
    setMessages((prev) => [...prev, assistantMsg]);

    try {
      const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1'}/chat`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${auth.token}`,
        },
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
              if (chunk) {
                setMessages((prev) => {
                  const updated = [...prev];
                  const last = updated[updated.length - 1];
                  if (last && last.role === 'assistant') last.content += chunk;
                  return [...updated];
                });
              }
            } catch { /* skip */ }
          }
        }
      }
    } catch {
      setMessages((prev) => {
        const updated = [...prev];
        const last = updated[updated.length - 1];
        if (last && last.role === 'assistant') last.content = 'Error: Failed to get response from server.';
        return [...updated];
      });
    } finally {
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
        return <div key={i} className="whitespace-pre-wrap leading-relaxed">{block.text}</div>;
      })}
    </div>
  );
}
