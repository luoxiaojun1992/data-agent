'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../providers';
import { useAuth } from '../../lib/api';

export default function MemoryPage() {
  const { auth, apiFetch } = useAuth();
  const [memories, setMemories] = useState<any[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [searchQuery, setSearchQuery] = useState('');
  const PAGE_SIZE = 20;

  const loadList = useCallback(async () => {
    const q = searchQuery.trim();
    const qs = q ? `&q=${encodeURIComponent(q)}` : '';
    const res = await apiFetch(`/memory/list?page=${page}&page_size=${PAGE_SIZE}${qs}`);
    const data = await res.json();
    setMemories(data.items || []);
    setTotal(data.total || 0);
  }, [apiFetch, page, searchQuery]);

  useEffect(() => { loadList(); }, [loadList]);

  const handleSearch = () => {
    setPage(1); // reset to page 1 on new search
  };

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">Memory 检索</h2>
          <span className="text-xs text-[var(--text-secondary)]">
            {auth.role === 'system_admin' ? '可查看全部用户' : '仅查看自己的记忆'}
          </span>
        </div>

        <div className="flex gap-2 mb-4">
          <input
            value={searchQuery}
            onChange={e => setSearchQuery(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleSearch()}
            placeholder="搜索记忆内容（可选）"
            className="flex-1 px-3 py-2 rounded-lg bg-white/5 border border-white/10 text-[var(--text-primary)] text-sm"
          />
          <button
            onClick={handleSearch}
            className="px-4 py-2 bg-[#B1E2FF] text-black rounded-lg text-sm"
          >
            搜索
          </button>
        </div>

        <div className="space-y-2">
          {memories.map((m: any) => (
            <div key={m.id} className="p-4 rounded-lg bg-white/5 border border-white/10">
              <div className="text-xs text-[var(--text-secondary)] mb-1">
                {new Date(m.updated_at).toLocaleString()} | user: {m.user_id}
              </div>
              <div className="text-sm text-[var(--text-primary)] font-mono break-all">
                {m.content?.parts?.map((p: any) => p.text).join('') || m.text || JSON.stringify(m)}
              </div>
            </div>
          ))}
          {memories.length === 0 && <p className="text-[var(--text-secondary)] text-sm">
            {searchQuery ? '无匹配结果' : '无记忆数据'}
          </p>}
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