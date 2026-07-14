'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '../../../lib/api';

interface Doc {
  id: string;
  user_id: string;
  title: string;
  file_name: string;
  file_type: string;
  size_bytes: number;
  status: string;
  chunk_count: number;
  tags: string[];
  created_at: string;
}

const PAGE_SIZE = 10;

export default function KnowledgePage() {
  const { auth, apiFetch } = useAuth();
  const [docs, setDocs] = useState<Doc[]>([]);
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [tagFilter, setTagFilter] = useState('');
  const [showUpload, setShowUpload] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [selectedFiles, setSelectedFiles] = useState<File[]>([]);
  const [uploadProgress, setUploadProgress] = useState<number[]>([]);
  const [uploadComplete, setUploadComplete] = useState<boolean[]>([]);
  const fileInputRef = React.useRef<HTMLInputElement>(null);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  const showToast = (msg: string, type: 'success' | 'error') => {
    setToast({ message: msg, type });
    setTimeout(() => setToast(null), 3000);
  };

  const fetchDocs = useCallback(async () => {
    try {
      const res = await apiFetch('/admin/knowledge/docs');
      if (res.ok) {
        const data = await res.json();
        setDocs(Array.isArray(data) ? data : []);
      }
    } catch { /* ignore */ }
  }, [apiFetch]);

  useEffect(() => {
    if (auth.hydrated) fetchDocs();
  }, [auth.hydrated, fetchDocs]);

  const filtered = docs
    .filter((d) => {
      if (search) {
        const q = search.toLowerCase();
        return (d.title || '').toLowerCase().includes(q) || (d.file_name || '').toLowerCase().includes(q);
      }
      return true;
    })
    .filter((d) => (tagFilter ? (d.tags || []).includes(tagFilter) : true));

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const paged = filtered.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);

  const handleUpload = async () => {
    if (selectedFiles.length === 0) return;
    setUploading(true);
    setUploadProgress(new Array(selectedFiles.length).fill(0));
    setUploadComplete(new Array(selectedFiles.length).fill(false));

    for (let i = 0; i < selectedFiles.length; i++) {
      const file = selectedFiles[i];
      const formData = new FormData();
      formData.append('title', file.name);
      formData.append('file_name', file.name);
      formData.append('file_type', file.name.split('.').pop() || 'unknown');
      formData.append('size_bytes', String(file.size));

        try {
          const res = await apiFetch('/knowledge/docs', {
            method: 'POST',
            body: formData,
          });
        if (res.ok) {
          setUploadProgress(prev => { const p = [...prev]; p[i] = 100; return p; });
          setUploadComplete(prev => { const c = [...prev]; c[i] = true; return c; });
        }
      } catch {
        // upload failed silently
      }
    }
    setUploading(false);
    fetchDocs();
    showToast('上传完成', 'success');
  };

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files || []);
    if (files.length > 0) {
      setSelectedFiles(files);
      setShowUpload(true);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('确定要删除该文档吗？')) return;
    try {
      await apiFetch(`/knowledge/docs/${id}`, { method: 'DELETE' });
      showToast('已删除', 'success');
      setDocs((prev) => prev.filter((d) => d.id !== id));
    } catch {
      showToast('删除失败', 'error');
    }
  };

  // Collect all unique tags
  const allTags = Array.from(new Set(docs.flatMap((d) => d.tags || [])));

  return (
    <AppLayout>
      <div className="animate-fade-in" data-testid="kb-page-header">
        <div className="mb-8">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">
            知识库管理 · LLM 索引
          </h2>
        </div>

        {/* Toast */}
        {toast && (
          <div style={{ position: 'fixed', top: 20, right: 20, zIndex: 9999,
            background: toast.type === 'success' ? 'rgba(16,185,129,0.9)' : 'rgba(239,68,68,0.9)',
            color: '#fff', padding: '12px 20px', borderRadius: '8px', fontSize: '14px',
          }}>{toast.message}</div>
        )}

        {/* Info Banner */}
        <div data-testid="kb-info-banner" style={{ padding: '12px 16px', marginBottom: '20px',
          background: 'rgba(59,130,246,0.1)', borderRadius: '10px', color: '#3b82f6', fontSize: '13px',
        }}>
          📌 文档上传后由 LLM 自动分析语义段落边界并拆分索引（复用异步 Agent 任务），索引数据与文档 ID 强绑定
        </div>

        {/* Toolbar: Upload + Search */}
        <div style={{ display: 'flex', gap: '12px', marginBottom: '20px', flexWrap: 'wrap' }}>
          <button data-testid="kb-upload-btn" onClick={() => setShowUpload(true)}
            style={{ padding: '10px 20px', background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
              color: '#fff', border: 'none', borderRadius: '8px', fontSize: '14px', fontWeight: 600, cursor: 'pointer' }}>
            + 上传文档
          </button>
          <button data-testid="kb-batch-upload-btn" onClick={() => setShowUpload(true)}
            style={{ padding: '10px 20px', background: 'rgba(255,255,255,0.06)',
              color: '#7A7A7A', border: '1px solid rgba(255,255,255,0.1)', borderRadius: '8px', fontSize: '14px', cursor: 'pointer' }}>
            📦 批量上传
          </button>
          <input data-testid="kb-search-input" placeholder="搜索文档..." value={search}
            onChange={(e) => { setSearch(e.target.value); setPage(1); }}
            style={{ padding: '10px 16px', background: 'rgba(255,255,255,0.06)', border: '1px solid rgba(255,255,255,0.1)',
              borderRadius: '8px', fontSize: '14px', color: 'var(--text-primary)', outline: 'none', flex: 1, minWidth: '200px' }} />
        </div>

        {/* Upload Modal */}
        {showUpload && (
          <div data-testid="kb-upload-modal" style={{ position: 'fixed', inset: 0, zIndex: 999,
            background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}
            onClick={(e) => { if (e.target === e.currentTarget) { setShowUpload(false); setSelectedFiles([]); } }}>
            <div className="glass" style={{ padding: '24px', maxWidth: '420px', width: '90%' }}>
              <h3 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '16px' }}>
                上传文档
              </h3>
              <div data-testid="kb-drop-zone" style={{ padding: '40px', textAlign: 'center',
                border: '2px dashed rgba(255,255,255,0.15)', borderRadius: '12px', marginBottom: '16px',
                color: '#7A7A7A', fontSize: '14px', cursor: 'pointer' }}
                onClick={() => fileInputRef.current?.click()}>
                📤 拖拽文件到此处或点击选择
              </div>
              {/* File list with progress */}
              {selectedFiles.length > 0 && (
                <div style={{ marginBottom: '16px' }}>
                  {selectedFiles.map((f, i) => (
                    <div key={i} data-testid={`kb-file-item-${i}`}
                      style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                        padding: '6px 8px', fontSize: '13px', color: 'var(--text-primary)',
                        background: 'rgba(255,255,255,0.04)', borderRadius: '6px', marginBottom: '4px' }}>
                      <span>{f.name}</span>
                      {uploadComplete[i] ? (
                        <span data-testid={`kb-file-done-${i}`}>✅</span>
                      ) : uploadProgress[i] > 0 ? (
                        <span data-testid={`kb-upload-progress-${i}`}
                          style={{ fontSize: '12px', color: '#5c7cfa' }}>{uploadProgress[i]}%</span>
                      ) : uploading ? (
                        <span>⏳</span>
                      ) : null}
                    </div>
                  ))}
                </div>
              )}
              <button onClick={handleUpload} disabled={uploading || selectedFiles.length === 0}
                style={{ width: '100%', padding: '10px', background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
                  color: '#fff', border: 'none', borderRadius: '8px', fontSize: '14px', cursor: 'pointer' }}>
                {uploading ? '上传中...' : '确认上传'}
              </button>
            </div>
          </div>
        )}

        {/* Hidden file input */}
        <input ref={fileInputRef} type="file" multiple data-testid="kb-upload-file-input"
          accept=".txt,.pdf,.json,.csv,.bin,.png,.jpg"
          style={{ display: 'none' }} onChange={handleFileSelect} />

        {/* Tag Filter */}
        {allTags.length > 0 && (
          <div style={{ display: 'flex', gap: '8px', marginBottom: '16px', flexWrap: 'wrap' }}>
            {allTags.map((tag) => (
              <button key={tag} data-testid={`kb-tag-filter-${tag}`}
                onClick={() => setTagFilter(tagFilter === tag ? '' : tag)}
                style={{ padding: '4px 14px', borderRadius: '20px', cursor: 'pointer', fontSize: '12px',
                  background: tagFilter === tag ? 'var(--accent)' : 'rgba(255,255,255,0.06)',
                  color: tagFilter === tag ? '#fff' : '#7A7A7A', border: 'none' }}>
                {tag}
              </button>
            ))}
          </div>
        )}

        {/* Document Cards */}
        <div data-testid="kb-search-results">
          {paged.map((doc) => (
            <div key={doc.id} data-testid={`kb-doc-card-${doc.id}`} className="glass"
              style={{ padding: '20px 24px', marginBottom: '12px', display: 'flex', alignItems: 'center', gap: '16px' }}>
              {/* File Icon */}
              <div style={{ width: '44px', height: '44px', borderRadius: '10px',
                background: 'rgba(236,72,153,0.12)', display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: '20px' }}>
                📄
              </div>
              {/* Info */}
              <div style={{ flex: 1 }}>
                <p data-testid="kb-doc-name" style={{ fontSize: '15px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '4px' }}>
                  {doc.title || doc.file_name || '未命名文档'}
                </p>
                <p data-testid="kb-doc-meta" style={{ fontSize: '12px', color: '#7A7A7A' }}>
                  {(doc.size_bytes / 1024).toFixed(1)} KB · {doc.chunk_count || 0} 分片 · {doc.user_id?.slice(0, 8) || '—'}
                </p>
              </div>
              {/* Status */}
              <div>
                <span data-testid={`kb-doc-status-${doc.id}`} data-status={doc.status}
                  style={{ display: 'inline-block', padding: '3px 10px', borderRadius: '10px', fontSize: '12px', fontWeight: 500,
                    background: statusBg(doc.status), color: statusColor(doc.status) }}>
                  {statusIcon(doc.status)} {statusLabel(doc.status)}
                </span>
              </div>
              {/* Tags */}
              <div data-testid="kb-doc-tags" style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
                {(doc.tags || []).map((t) => (
                  <span key={t} style={{ padding: '2px 8px', borderRadius: '6px', background: 'rgba(92,124,250,0.1)', color: '#5c7cfa', fontSize: '11px' }}>{t}</span>
                ))}
              </div>
              {/* Delete */}
              <button data-testid={`kb-doc-delete-${doc.id}`} onClick={() => handleDelete(doc.id)}
                style={{ background: 'transparent', border: '1px solid rgba(239,68,68,0.3)', color: '#ef4444',
                  borderRadius: '6px', padding: '4px 10px', fontSize: '12px', cursor: 'pointer' }}>
                🗑
              </button>
            </div>
          ))}
          {paged.length === 0 && (
            <div className="glass p-12 text-center">
              <p className="text-sm text-[var(--text-secondary)]">暂无文档</p>
            </div>
          )}
        </div>

        {/* Pagination */}
        {totalPages > 1 && (
          <div data-testid="kb-pagination" style={{ display: 'flex', justifyContent: 'center', gap: '8px', marginTop: '24px' }}>
            <button onClick={() => setPage((p) => Math.max(1, p - 1))} disabled={page === 1}
              style={pageBtnStyle}>上一页</button>
            <span style={{ padding: '8px 12px', fontSize: '13px', color: '#7A7A7A' }}>
              {page} / {totalPages}（共 {filtered.length} 条）
            </span>
            <button onClick={() => setPage((p) => Math.min(totalPages, p + 1))} disabled={page === totalPages}
              style={pageBtnStyle}>下一页</button>
          </div>
        )}
      </div>
    </AppLayout>
  );
}

const statusLabel = (s: string) => {
  const m: Record<string, string> = { ready: '已索引 ✓', indexing: '索引中 ⟳', uploaded: '已上传', failed: '索引失败', pending: '等待索引' };
  return m[s] || s;
};
const statusIcon = (s: string) => {
  const m: Record<string, string> = { ready: '✅', indexing: '🔄', uploaded: '📤', failed: '❌' };
  return m[s] || '';
};
const statusBg = (s: string) => {
  const m: Record<string, string> = { ready: 'rgba(52,211,153,0.15)', indexing: 'rgba(251,191,36,0.15)', uploaded: 'rgba(59,130,246,0.15)', failed: 'rgba(251,113,133,0.15)' };
  return m[s] || 'rgba(107,114,128,0.15)';
};
const statusColor = (s: string) => {
  const m: Record<string, string> = { ready: '#34D399', indexing: '#FBBF24', uploaded: '#3b82f6', failed: '#FB7185' };
  return m[s] || '#6b7280';
};
const pageBtnStyle: React.CSSProperties = {
  padding: '6px 14px', background: 'rgba(255,255,255,0.06)', border: '1px solid rgba(255,255,255,0.1)',
  borderRadius: '8px', color: '#7A7A7A', fontSize: '13px', cursor: 'pointer',
};
