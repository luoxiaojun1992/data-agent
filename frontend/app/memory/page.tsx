'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../providers';
import Pagination from '../components/Pagination';
import { useAuth } from '../../lib/api';

const PAGE_SIZE = 20;

export default function MemoryPage() {
  const { auth, apiFetch } = useAuth();
  const [memories, setMemories] = useState<any[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [searchQuery, setSearchQuery] = useState('');
  const [detailId, setDetailId] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);

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
    // Backend serializes adapter.Observation with Go-style capitalized fields.
    return m.Content || (m.content?.parts?.map((p: any) => p.text || '').join('')) || m.text || '';
  };

  const extractId = (m: any): string => m.id || m.ID || m._id || '';

  const truncate = (s: string, n: number): string => {
    if (s.length <= n) return s;
    return s.slice(0, n) + '…';
  };

  const uploadToKnowledge = async (m: any) => {
    const id = extractId(m);
    const content = extractText(m);
    if (!id || !content) return;
    setUploading(true);
    try {
      const blob = new Blob([content], { type: 'text/plain' });
      const fileName = `memory-${id}.txt`;
      // Use actual memory ID in title
      const titleSnippet = content.replace(/\s+/g, ' ').trim().slice(0, 60) || `Memory ${id}`;
      const fd = new FormData();
      fd.append('title', titleSnippet);
      fd.append('file_name', fileName);
      fd.append('file_type', 'txt');
      fd.append('size_bytes', String(blob.size));
      fd.append('file', blob, fileName);
      const res = await apiFetch('/knowledge/docs', { method: 'POST', body: fd });
      if (res.ok) {
        alert('已上传到知识库');
      } else {
        const err = await res.json().catch(() => ({ error: 'upload failed' }));
        alert('上传失败: ' + (err.error || res.status));
      }
    } catch (e: any) {
      alert('上传失败: ' + e.message);
    } finally {
      setUploading(false);
    }
  };

  if (detailId) {
    const detail = memories.find(m => extractId(m) === detailId);
    return (
      <AppLayout>
        <div className="animate-fade-in max-w-4xl">
          <button onClick={() => setDetailId(null)} className="mb-4 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)]">
            ← 返回列表
          </button>
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-2xl font-bold text-[var(--text-primary)]">记忆详情</h2>
            <button onClick={() => uploadToKnowledge(detail)} disabled={uploading}
              className="px-3 py-1.5 text-xs rounded-lg bg-[var(--accent)]/15 border border-[var(--accent)]/30 text-[var(--accent)] hover:bg-[var(--accent)]/25 disabled:opacity-40">
              {uploading ? '上传中...' : '📤 上传到知识库'}
            </button>
          </div>
          {detail ? (
            <div className="space-y-4">
              <div className="p-4 rounded-lg bg-white/5 border border-white/10">
                <div className="text-xs text-[var(--text-secondary)] mb-1">ID</div>
                <div className="text-sm font-mono text-[var(--text-primary)] break-all">{extractId(detail)}</div>
              </div>
              <div className="p-4 rounded-lg bg-white/5 border border-white/10">
                <div className="text-xs text-[var(--text-secondary)] mb-1">用户</div>
                <div className="text-sm font-mono text-[var(--text-primary)]">{detail.UserEmail || detail.user_email || detail.UserID || detail.user_id}</div>
              </div>
              {detail.SessionID && (
                <div className="p-4 rounded-lg bg-white/5 border border-white/10">
                  <div className="text-xs text-[var(--text-secondary)] mb-1">关联会话</div>
                  <div className="text-sm">
                    <div className="font-mono text-[var(--text-secondary)] text-xs mb-1">{detail.SessionID}</div>
                    <div className="text-[var(--text-primary)]">{detail.SessionTitle || detail.session_title || '(无标题)'}</div>
                  </div>
                </div>
              )}
              <div className="p-4 rounded-lg bg-white/5 border border-white/10">
                <div className="text-xs text-[var(--text-secondary)] mb-1">创建时间</div>
                <div className="text-sm text-[var(--text-primary)]">{new Date(detail.CreatedAt || detail.created_at || detail.updated_at).toLocaleString()}</div>
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
                key={extractId(m)}
                onClick={() => setDetailId(extractId(m))}
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

        <Pagination
          current={page}
          totalPages={Math.ceil(total / PAGE_SIZE)}
          onChange={setPage}
        />
      </div>
    </AppLayout>
  );
}