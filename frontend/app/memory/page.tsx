'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../providers';
import { useAuth } from '../../lib/api';

const PAGE_SIZE = 20;

export default function MemoryPage() {
  const { auth, apiFetch } = useAuth();
  const [memories, setMemories] = useState<any[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [searchQuery, setSearchQuery] = useState('');
  const [detailId, setDetailId] = useState<string | null>(null);

  const loadList = useCallback(async () => {
    const q = searchQuery.trim();
    const qs = q ? `&q=${encodeURIComponent(q)}` : '';
    const res = await apiFetch(`/memory/list?page=${page}&page_size=${PAGE_SIZE}${qs}`);
    const data = await res.json();
    setMemories(data.items || []);
    setTotal(data.total || 0);
  }, [apiFetch, page, searchQuery]);

  // Debounced search on every keystroke
  useEffect(() => {
    if (!auth.hydrated) return;
    const t = setTimeout(() => {
      setPage(1);
      loadList();
    }, 200);
    return () => clearTimeout(t);
  }, [searchQuery, auth.hydrated]);

  useEffect(() => {
    if (auth.hydrated) loadList();
  }, [loadList, auth.hydrated]);

  const extractText = (m: any): string => {
    if (m.content?.parts?.length) {
      return m.content.parts.map((p: any) => p.text || '').join('');
    }
    return m.text || '';
  };

  const truncate = (s: string, n: number): string => {
    if (s.length <= n) return s;
    return s.slice(0, n) + '…';
  };

  if (detailId) {
    const detail = memories.find(m => m.id === detailId);
    return (
      <AppLayout>
        <div className="animate-fade-in max-w-4xl">
          <button onClick={() => setDetailId(null)} className="mb-4 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)]">
            ← 返回列表
          </button>
          <h2 className="text-2xl font-bold text-[var(--text-primary)] mb-4">记忆详情</h2>
          {detail ? (
            <div className="space-y-4">
              <div className="p-4 rounded-lg bg-white/5 border border-white/10">
                <div className="text-xs text-[var(--text-secondary)] mb-1">ID</div>
                <div className="text-sm font-mono text-[var(--text-primary)] break-all">{detail.id}</div>
              </div>
              <div className="p-4 rounded-lg bg-white/5 border border-white/10">
                <div className="text-xs text-[var(--text-secondary)] mb-1">用户</div>
                <div className="text-sm font-mono text-[var(--text-primary)]">{detail.user_id}</div>
              </div>
              <div className="p-4 rounded-lg bg-white/5 border border-white/10">
                <div className="text-xs text-[var(--text-secondary)] mb-1">更新时间</div>
                <div className="text-sm text-[var(--text-primary)]">{new Date(detail.updated_at).toLocaleString()}</div>
              </div>
              <div className="p-4 rounded-lg bg-white/5 border border-white/10">
                <div className="text-xs text-[var(--text-secondary)] mb-2">内容</div>
                <div className="text-sm text-[var(--text-primary)] whitespace-pre-wrap leading-relaxed">
                  {extractText(detail)}
                </div>
              </div>
            </div>
          ) : (
            <p className="text-[var(--text-secondary)] text-sm">记录不存在或已被删除</p>
          )}
        </div>
      </AppLayout>
    );
  }

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <h2 className="text-2xl font-bold text-[var(--text-primary)] mb-4">Memory 检索</h2>

        <div className="mb-4">
          <input
            value={searchQuery}
            onChange={e => setSearchQuery(e.target.value)}
            placeholder="搜索记忆内容"
            className="w-full px-3 py-2 rounded-lg bg-white/5 border border-white/10 text-[var(--text-primary)] text-sm focus:outline-none focus:border-[#B1E2FF]/50"
          />
        </div>

        <div className="space-y-2">
          {memories.map((m: any) => {
            const text = extractText(m);
            return (
              <div
                key={m.id}
                onClick={() => setDetailId(m.id)}
                className="p-3 rounded-lg bg-white/5 border border-white/10 cursor-pointer hover:bg-white/10 transition-colors"
              >
                <div className="text-sm text-[var(--text-primary)] font-mono break-all">
                  {truncate(text, 20)}
                </div>
              </div>
            );
          })}
          {memories.length === 0 && (
            <p className="text-[var(--text-secondary)] text-sm">
              {searchQuery ? '无匹配结果' : '无记忆数据'}
            </p>
          )}
        </div>

        {total > PAGE_SIZE && (
          <div className="flex justify-center gap-2 mt-4">
            {Array.from({ length: Math.ceil(total / PAGE_SIZE) }, (_, i) => (
              <button key={i} onClick={() => setPage(i + 1)}
                className={`px-3 py-1 rounded text-sm ${
                  page === i + 1 ? 'bg-[#B1E2FF] text-black' : 'bg-white/5 text-[var(--text-secondary)]'
                }`}>
                {i + 1}
              </button>
            ))}
          </div>
        )}
      </div>
    </AppLayout>
  );
}