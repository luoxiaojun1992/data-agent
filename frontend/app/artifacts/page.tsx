'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../providers';
import { useAuth } from '@/lib/api';

interface ArtifactItem {
  id: string;
  name: string;
  mime_type: string;
  size_bytes: number;
  session_id: string;
  created_at: string;
}

const PAGE_SIZE = 20;

export default function ArtifactsPage() {
  const { auth, apiFetch } = useAuth();
  const [artifacts, setArtifacts] = useState<ArtifactItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);

  const fetchArtifacts = useCallback(async () => {
    setLoading(true);
    try {
      const res = await apiFetch(`/artifacts/user?page=${page}&page_size=${PAGE_SIZE}`);
      if (res.ok) {
        const data = await res.json();
        setArtifacts(data.artifacts || []);
        setTotal(data.total || 0);
      }
    } catch (e) {
      console.error('[artifacts] fetch failed:', e);
    }
    setLoading(false);
  }, [apiFetch, page]);

  useEffect(() => {
    if (!auth.hydrated || !auth.token) return;
    fetchArtifacts();
  }, [fetchArtifacts, auth.hydrated, auth.token]);

  const downloadArtifact = async (id: string, name: string) => {
    try {
      const res = await apiFetch(`/artifacts/${id}/download-url`);
      if (res.ok) {
        const { url } = await res.json();
        window.open(url, '_blank');
      }
    } catch (e) {
      console.error('[artifacts] download-url failed:', e);
    }
  };

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  const formatDate = (d: string) => new Date(d).toLocaleString();

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-6 flex items-center justify-between" data-testid="artifacts-header">
          <div>
            <h2 className="text-2xl font-bold text-[var(--text-primary)]">产出物</h2>
            <p className="text-sm text-[var(--text-secondary)] mt-1">AI 生成的文件和报告</p>
          </div>
        </div>

        {loading ? (
          <div className="text-center py-12 text-[var(--text-secondary)]" data-testid="artifacts-loading">加载中...</div>
        ) : artifacts.length === 0 ? (
          <div className="glass p-12 text-center" data-testid="artifacts-empty">
            <span className="text-5xl block mb-4">📦</span>
            <p className="text-lg text-[var(--text-primary)] mb-2">暂无产出物</p>
            <p className="text-sm text-[var(--text-secondary)]">通过 Agent 任务或对话中的 PPT 生成 / Artifact 保存功能创建</p>
          </div>
        ) : (
          <div>
            <div className="space-y-3" data-testid="artifacts-list">
              {artifacts.map((a) => (
                <div key={a.id} className="glass p-4 flex items-center justify-between hover:bg-white/5 transition-colors" data-testid={`artifact-row-${a.id}`}>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-[var(--text-primary)] truncate">{a.name}</p>
                    <div className="flex items-center gap-3 mt-1 text-xs text-[var(--text-secondary)]">
                      <span>{formatSize(a.size_bytes)}</span>
                      <span>·</span>
                      <span>{formatDate(a.created_at)}</span>
                      <span>·</span>
                      <span>Session: {a.session_id?.slice(0, 12)}</span>
                    </div>
                  </div>
                  <button
                    onClick={() => downloadArtifact(a.id, a.name)}
                    className="px-3 py-1.5 text-xs rounded-lg bg-[var(--accent)] text-white hover:opacity-90 flex-shrink-0"
                    data-testid={`artifact-download-${a.id}`}
                  >
                    ⬇ 下载
                  </button>
                </div>
              ))}
            </div>

            <div className="flex items-center justify-between mt-4" data-testid="artifacts-pagination">
              <span className="text-xs text-[var(--text-secondary)]">
                共 {total} 个 · 第 {page} / {Math.max(1, Math.ceil(total / PAGE_SIZE))} 页
              </span>
              <div className="flex gap-2">
                <button onClick={() => setPage(p => Math.max(1, p - 1))} disabled={page <= 1}
                  className="px-3 py-1 text-xs rounded-lg border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] disabled:opacity-30"
                  data-testid="artifacts-prev">← 上一页</button>
                <button onClick={() => setPage(p => p + 1)} disabled={page * PAGE_SIZE >= total}
                  className="px-3 py-1 text-xs rounded-lg border border-[var(--border-glass)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] disabled:opacity-30"
                  data-testid="artifacts-next">下一页 →</button>
              </div>
            </div>
          </div>
        )}
      </div>
    </AppLayout>
  );
}
