'use client';

import React, { useState, useRef, useEffect } from 'react';
import AppLayout from '../providers';
import { useAuth } from '@/lib/api';

interface Message {
  role: 'user' | 'assistant';
  content: string;
  timestamp: Date;
}

export default function ChatPage() {
  const { auth, apiFetch } = useAuth();
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [streaming, setStreaming] = useState(false);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const createSession = async () => {
    try {
      const res = await apiFetch('/sessions', {
        method: 'POST',
      });
      const data = await res.json();
      setSessionId(data.session_id);
      return data.session_id;
    } catch (err) {
      console.error('Failed to create session:', err);
      return null;
    }
  };

  const sendMessage = async () => {
    if (!input.trim() || streaming) return;

    const userMsg: Message = { role: 'user', content: input, timestamp: new Date() };
    setMessages((prev) => [...prev, userMsg]);
    setInput('');
    setStreaming(true);

    let sid = sessionId;
    if (!sid) {
      sid = await createSession();
    }
    if (!sid) {
      setStreaming(false);
      return;
    }

    const assistantMsg: Message = { role: 'assistant', content: '', timestamp: new Date() };
    setMessages((prev) => [...prev, assistantMsg]);

    try {
      const token = auth.token;
      const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1'}/chat`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`,
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
                  if (last && last.role === 'assistant') {
                    last.content += chunk;
                  }
                  return [...updated];
                });
              }
            } catch {
              // Skip unparseable chunks
            }
          }
        }
      }
    } catch (err) {
      setMessages((prev) => {
        const updated = [...prev];
        const last = updated[updated.length - 1];
        if (last && last.role === 'assistant') {
          last.content = 'Error: Failed to get response from server.';
        }
        return [...updated];
      });
    } finally {
      setStreaming(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  return (
    <AppLayout>
      <div className="flex flex-col h-[calc(100vh-64px)] animate-fade-in">
        {/* Header */}
        <div className="mb-4" data-testid="chat-header">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">Chat 对话</h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1" data-testid="chat-session-info">
            {sessionId ? `Session: ${sessionId.slice(0, 8)}...` : '创建新会话'}
          </p>
        </div>

        {/* Messages area */}
        <div className="flex-1 overflow-y-auto mb-4 space-y-4" data-testid="chat-messages">
          {messages.length === 0 && (
            <div className="flex items-center justify-center h-full">
              <div className="text-center">
                <span className="text-5xl block mb-4">💬</span>
                <p className="text-lg text-[var(--text-primary)]">开始数据分析对话</p>
                <p className="text-sm text-[var(--text-secondary)] mt-2">
                  输入你的数据分析需求，AI 将为你提供帮助
                </p>
                <div className="flex flex-wrap justify-center gap-2 mt-4" data-testid="chat-prompt-row">
                  {['统计最近一周的销售数据', '分析客户流失原因', '生成月度分析报告'].map((hint) => (
                    <button
                      key={hint}
                      onClick={() => setInput(hint)}
                      className="px-3 py-1.5 text-xs rounded-full bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:border-[var(--accent)]/50 transition-all"
                    >
                      {hint}
                    </button>
                  ))}
                </div>
              </div>
            </div>
          )}

          {messages.map((msg, i) => (
            <div key={i} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
              <div
                className={`max-w-[70%] p-4 rounded-2xl ${
                  msg.role === 'user'
                    ? 'bg-[var(--accent)] text-white rounded-br-md'
                    : 'glass text-[var(--text-primary)] rounded-bl-md'
                }`}
              >
                <div className="text-sm whitespace-pre-wrap">{msg.content || (streaming && msg.role === 'assistant' ? '...' : '')}</div>
                <p className="text-xs opacity-60 mt-1">
                  {msg.timestamp.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}
                </p>
              </div>
            </div>
          ))}
          <div ref={messagesEndRef} />
        </div>

        {/* Input area */}
        <div className="glass p-4">
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
            >
              {streaming ? '发送中...' : '发送'}
            </button>
          </div>
        </div>
      </div>
    </AppLayout>
  );
}
