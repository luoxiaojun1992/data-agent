'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useTranslations } from 'next-intl';
import AppLayout from '../providers';
import Pagination from '../components/Pagination';
import { primaryButtonStyle, modalOverlayStyle } from '../components/ui';
import { useAuth } from '../../lib/api';
import { parsePdf, isPdfFile, isTxtFile, isImageFile, imageMimeType } from '../../lib/pdf';
import { parseExcel, isExcelFile } from '../../lib/excel';

interface Doc {
  id: string;
  user_id: string;
  title: string;
  file_name: string;
  file_type: string;
  size_bytes: number;
  status: string;
  chunk_count: number;
  progress_percent: number;
  tags: string[];
  is_public: boolean;
  created_at: string;
}

const PAGE_SIZE = 10;

export default function KnowledgePage() {
  const { auth, apiFetch } = useAuth();
  const t = useTranslations('knowledge');
  const [docs, setDocs] = useState<Doc[]>([]);
  const [total, setTotal] = useState(0);
  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [page, setPage] = useState(1);
  const [showUpload, setShowUpload] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [selectedFiles, setSelectedFiles] = useState<File[]>([]);
  const [uploadProgress, setUploadProgress] = useState<number[]>([]);
  const [uploadComplete, setUploadComplete] = useState<boolean[]>([]);
  const fileInputRef = React.useRef<HTMLInputElement>(null);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [uploadError, setUploadError] = useState('');
  const [showImportUrl, setShowImportUrl] = useState(false);
  const [importUrlValue, setImportUrlValue] = useState('');
  const [importingUrl, setImportingUrl] = useState(false);
  const [importUrlError, setImportUrlError] = useState('');

  const showToast = (msg: string, type: 'success' | 'error') => {
    setToast({ message: msg, type });
    setTimeout(() => setToast(null), 3000);
  };

  const fetchDocs = useCallback(async () => {
    try {
      const params = new URLSearchParams({ page: String(page), page_size: String(PAGE_SIZE) });
      if (debouncedSearch) params.set('q', debouncedSearch);
      const res = await apiFetch(`/knowledge/docs?${params.toString()}`);
      if (res.ok) {
        const data = await res.json();
        setDocs(data.docs || []);
        setTotal(data.total || 0);
      }
    } catch { /* ignore */ }
  }, [apiFetch, page, debouncedSearch]);

  // 搜索防抖：输入停止 300ms 后再发起后端 q 过滤（SPEC-075：后端化）。
  useEffect(() => {
    const t = setTimeout(() => {
      setDebouncedSearch(search);
      setPage(1);
    }, 300);
    return () => clearTimeout(t);
  }, [search]);

  useEffect(() => {
    if (auth.hydrated) fetchDocs();
  }, [auth.hydrated, fetchDocs]);

  // 上传一个 txt 文档（multipart）
  const uploadTxtDoc = async (title: string, fileName: string, content: string) => {
    const blob = new Blob([content], { type: 'text/plain' });
    const fd = new FormData();
    fd.append('title', title);
    fd.append('file', blob, fileName);
    fd.append('file_name', fileName);
    fd.append('file_type', 'txt');
    fd.append('size_bytes', String(blob.size));
    const res = await apiFetch('/knowledge/docs', { method: 'POST', body: fd });
    if (!res.ok) {
      const d = await res.json().catch(() => ({}));
      throw new Error(d.error || t('uploadFailed', { status: res.status }));
    }
  };

  // 上传一个图片文档（base64）
  const uploadImageDoc = async (title: string, fileName: string, dataUrl: string, mimeType: string) => {
    const fd = new FormData();
    fd.append('title', title);
    fd.append('file_name', fileName);
    fd.append('file_type', 'image');
    fd.append('file_base64', dataUrl);
    fd.append('mime_type', mimeType);
    const res = await apiFetch('/knowledge/docs', { method: 'POST', body: fd });
    if (!res.ok) {
      const d = await res.json().catch(() => ({}));
      throw new Error(d.error || t('uploadFailed', { status: res.status }));
    }
  };

  const handleUpload = async () => {
    if (selectedFiles.length === 0) return;
    setUploading(true);
    setUploadProgress(new Array(selectedFiles.length).fill(0));
    setUploadComplete(new Array(selectedFiles.length).fill(false));
    setUploadError('');

    let uploadedCount = 0;
    let hadError = false;
    for (let i = 0; i < selectedFiles.length; i++) {
      const file = selectedFiles[i];
      try {
        if (isPdfFile(file.name)) {
          // PDF：浏览器端解析成纯文本 + 每页图片，标题和文件名都加 -{编号} 自增
          const { text, images } = await parsePdf(file);
          const baseName = file.name.replace(/\.pdf$/i, '');
          let counter = 1;
          if (text.trim()) {
            await uploadTxtDoc(`${baseName}-${counter}`, `${baseName}-${counter}.txt`, text);
            uploadedCount++;
            counter++;
          }
          for (const img of images) {
            await uploadImageDoc(`${baseName}-${counter}`, `${baseName}-${counter}.${img.ext}`, img.dataUrl, img.mimeType);
            uploadedCount++;
            counter++;
          }
        } else if (isTxtFile(file.name)) {
          const text = await file.text();
          await uploadTxtDoc(file.name.replace(/\.txt$/i, ''), file.name, text);
          uploadedCount++;
        } else if (isExcelFile(file.name)) {
          // Excel：浏览器端解析为纯文本建 txt doc（SPEC-096 D6：复用 txt，不新增 xlsx 类型）
          const { text } = await parseExcel(file);
          const baseName = file.name.replace(/\.xlsx$/i, '');
          if (text.trim()) {
            await uploadTxtDoc(`${baseName}`, `${baseName}.txt`, text);
            uploadedCount++;
          } else {
            hadError = true;
            setUploadError(t('excelEmpty', { name: file.name }));
          }
        } else if (isImageFile(file.name)) {
          const dataUrl = await fileToDataUrl(file, t('readFileFailed'));
          await uploadImageDoc(
            file.name.replace(/\.[^.]+$/, ''),
            file.name,
            dataUrl,
            file.type || imageMimeType(file.name),
          );
          uploadedCount++;
        } else {
          hadError = true;
          setUploadError(t('unsupportedType', { name: file.name }));
        }
        setUploadProgress(prev => { const p = [...prev]; p[i] = 100; return p; });
        setUploadComplete(prev => { const c = [...prev]; c[i] = true; return c; });
      } catch (e: any) {
        hadError = true;
        setUploadError(e?.message || t('networkError'));
      }
    }
    setUploading(false);
    fetchDocs();
    if (hadError) {
      showToast(t('partialUpload'), 'error');
    } else {
      showToast(t('uploadDone', { count: uploadedCount }), 'success');
      setShowUpload(false);
      setSelectedFiles([]);
    }
  };

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files || []);
    if (files.length > 0) {
      setSelectedFiles(files);
      setShowUpload(true);
    }
  };

  // 导入网址（SPEC-081）：后端渲染解析，耗时可能达 120s，需 loading 反馈。
  const handleImportUrl = async () => {
    const url = importUrlValue.trim();
    if (!url) return;
    setImportingUrl(true);
    setImportUrlError('');
    try {
      const res = await apiFetch('/knowledge/import-url', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url }),
      });
      if (!res.ok) {
        const d = await res.json().catch(() => ({}));
        throw new Error(d.error || t('importFailed', { status: res.status }));
      }
      const data = await res.json();
      const n = (data.doc_ids || []).length;
      const skipped = data.skipped_images || 0;
      const msg = skipped > 0
        ? t('importDoneSkipped', { count: n, skipped })
        : t('importDone', { count: n });
      showToast(msg, 'success');
      setShowImportUrl(false);
      setImportUrlValue('');
      fetchDocs();
    } catch (e: any) {
      setImportUrlError(e?.message || t('networkError'));
      showToast(t('importFailedToast'), 'error');
    } finally {
      setImportingUrl(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm(t('deleteConfirm'))) return;
    try {
      await apiFetch(`/knowledge/docs/${id}`, { method: 'DELETE' });
      showToast(t('deleted'), 'success');
      setDocs((prev) => prev.filter((d) => d.id !== id));
    } catch {
      showToast(t('deleteFailed'), 'error');
    }
  };

  const togglePublic = async (id: string, isPublic: boolean) => {
    try {
      await apiFetch(`/knowledge/docs/${id}/public`, {
        method: 'PUT',
        body: JSON.stringify({ is_public: !isPublic }),
      });
      setDocs((prev) => prev.map((d) => d.id === id ? { ...d, is_public: !isPublic } : d));
      showToast(isPublic ? t('setPrivate') : t('setPublic'), 'success');
    } catch {
      showToast(t('opFailed'), 'error');
    }
  };

  return (
    <AppLayout>
      <div className="animate-fade-in" data-testid="kb-page-header">
        <div className="mb-8">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">{t('title')}</h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">{t('desc')}</p>
        </div>

        {/* Toast */}
        {toast && (
          <div style={{ position: 'fixed', top: 20, right: 20, zIndex: 9999,
            background: toast.type === 'success' ? 'rgba(16,185,129,0.9)' : 'rgba(239,68,68,0.9)',
            color: '#fff', padding: '12px 20px', borderRadius: '8px', fontSize: '14px',
          }}>{toast.message}</div>
        )}

        {/* Toolbar: Upload + Import URL + Search */}
        <div style={{ display: 'flex', gap: '12px', marginBottom: '20px', flexWrap: 'wrap' }}>
          <button data-testid="kb-upload-btn" onClick={() => setShowUpload(true)}
            style={primaryButtonStyle}>
            {t('uploadDoc')}
          </button>
          <button data-testid="kb-import-url-btn" onClick={() => { setShowImportUrl(true); setImportUrlError(''); }}
            style={{ ...primaryButtonStyle, background: 'transparent', border: '1px solid var(--surface-20)', color: 'var(--text-primary)' }}>
            {t('importUrl')}
          </button>
          <input data-testid="kb-search-input" placeholder={t('searchPlaceholder')} value={search}
            onChange={(e) => { setSearch(e.target.value); setPage(1); }}
            style={{ padding: '10px 16px', background: 'var(--surface-6)', border: '1px solid var(--surface-10)',
              borderRadius: '8px', fontSize: '14px', color: 'var(--text-primary)', outline: 'none', flex: 1, minWidth: '200px' }} />
        </div>

        {/* Upload Modal */}
        {showUpload && (
          <div data-testid="kb-upload-modal" style={{ ...modalOverlayStyle, zIndex: 999 }}
            onClick={(e) => { if (e.target === e.currentTarget) { setShowUpload(false); setSelectedFiles([]); } }}>
            <div className="glass" style={{ padding: '24px', maxWidth: '420px', width: '90%' }}>
              <h3 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '16px' }}>
                {t('uploadModalTitle')}
              </h3>
              <div data-testid="kb-drop-zone" style={{ padding: '40px', textAlign: 'center',
                border: '2px dashed var(--surface-15)', borderRadius: '12px', marginBottom: '16px',
                color: '#7A7A7A', fontSize: '14px', cursor: 'pointer' }}
                onClick={() => fileInputRef.current?.click()}>
                {t('dropZone')}
              </div>
              {selectedFiles.length > 0 && (
                <div style={{ marginBottom: '16px' }}>
                  {selectedFiles.map((f, i) => (
                    <div key={i} data-testid={`kb-file-item-${i}`}
                      style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                        padding: '6px 8px', fontSize: '13px', color: 'var(--text-primary)',
                        background: 'var(--surface-4)', borderRadius: '6px', marginBottom: '4px' }}>
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
              {uploadError && (
                <div data-testid="kb-upload-error" style={{ color: '#ef4444', fontSize: '12px', marginBottom: '8px' }}>{uploadError}</div>
              )}
              <button onClick={handleUpload} disabled={uploading || selectedFiles.length === 0}
                style={{ width: '100%', padding: '10px', background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
                  color: '#fff', border: 'none', borderRadius: '8px', fontSize: '14px', cursor: 'pointer' }}>
                {uploading ? t('uploading') : t('confirmUpload')}
              </button>
            </div>
          </div>
        )}

        {/* Import URL Modal (SPEC-081) */}
        {showImportUrl && (
          <div data-testid="kb-import-url-modal" style={{ ...modalOverlayStyle, zIndex: 999 }}
            onClick={(e) => { if (e.target === e.currentTarget && !importingUrl) { setShowImportUrl(false); setImportUrlError(''); } }}>
            <div className="glass" style={{ padding: '24px', maxWidth: '460px', width: '90%' }}>
              <h3 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '16px' }}>
                {t('importModalTitle')}
              </h3>
              <input
                data-testid="kb-import-url-input"
                placeholder="https://example.com/article"
                value={importUrlValue}
                onChange={(e) => { setImportUrlValue(e.target.value); setImportUrlError(''); }}
                disabled={importingUrl}
                style={{ width: '100%', padding: '10px 12px', background: 'var(--surface-6)',
                  border: '1px solid var(--surface-15)', borderRadius: '8px', fontSize: '14px',
                  color: 'var(--text-primary)', outline: 'none', marginBottom: '12px', boxSizing: 'border-box' }}
              />
              {importUrlError && (
                <div data-testid="kb-import-url-error" style={{ color: '#ef4444', fontSize: '12px', marginBottom: '12px', wordBreak: 'break-all' }}>
                  {importUrlError}
                </div>
              )}
              <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
                <button onClick={() => { if (!importingUrl) { setShowImportUrl(false); setImportUrlError(''); } }}
                  disabled={importingUrl}
                  style={{ padding: '8px 16px', background: 'transparent', border: '1px solid var(--surface-15)',
                    borderRadius: '8px', color: 'var(--text-secondary)', fontSize: '14px', cursor: 'pointer' }}>
                  {t('cancel')}
                </button>
                <button data-testid="kb-import-url-submit" onClick={handleImportUrl}
                  disabled={importingUrl || importUrlValue.trim() === ''}
                  style={{ padding: '8px 20px', background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)',
                    color: '#fff', border: 'none', borderRadius: '8px', fontSize: '14px', cursor: 'pointer' }}>
                  {importingUrl ? t('importing') : t('import')}
                </button>
              </div>
            </div>
          </div>
        )}

        <input ref={fileInputRef} type="file" multiple data-testid="kb-upload-file-input"
          accept=".txt,.pdf,.xlsx,.png,.jpg,.jpeg,.gif,.webp,.bmp"
          style={{ display: 'none' }} onChange={handleFileSelect} />

        {/* Document Cards */}
        <div data-testid="kb-search-results">
          {docs.map((doc) => (
            <div key={doc.id} data-testid={`kb-doc-card-${doc.id}`} className="glass"
              style={{ padding: '20px 24px', marginBottom: '12px', display: 'flex', alignItems: 'center', gap: '16px' }}>
              <div style={{ width: '44px', height: '44px', borderRadius: '10px',
                background: 'rgba(236,72,153,0.12)', display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: '20px' }}>
                📄
              </div>
              <div style={{ flex: 1 }}>
                <p data-testid="kb-doc-name" style={{ fontSize: '15px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '4px' }}>
                  {doc.title || doc.file_name || t('unnamedDoc')}
                </p>
                <p data-testid="kb-doc-meta" style={{ fontSize: '12px', color: '#7A7A7A' }}>
                  {t('meta', { size: (doc.size_bytes / 1024).toFixed(1), chunks: doc.chunk_count || 0 })}
                </p>
              </div>
              <div>
                <span data-testid={`kb-doc-status-${doc.id}`} data-status={doc.status}
                  style={{ display: 'inline-block', padding: '3px 10px', borderRadius: '10px', fontSize: '12px', fontWeight: 500,
                    background: statusBg(doc.status), color: statusColor(doc.status) }}>
                  {statusIcon(doc.status)} {statusLabel(t, doc.status, doc.progress_percent)}
                </span>
                {/* Share toggle */}
                <label data-testid={`kb-doc-share-${doc.id}`} title={doc.is_public ? t('clickUnshare') : t('clickShare')} style={{ cursor: 'pointer', marginLeft: '8px', display: 'inline-flex', alignItems: 'center' }}>
                  <input type="checkbox" checked={doc.is_public} onChange={() => togglePublic(doc.id, doc.is_public)} style={{ display: 'none' }} />
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke={doc.is_public ? '#34d399' : '#94a3b8'} strokeWidth="2">
                    <circle cx="18" cy="5" r="3" />
                    <circle cx="6" cy="12" r="3" />
                    <circle cx="18" cy="19" r="3" />
                    <line x1="8.59" y1="13.51" x2="15.42" y2="17.49" />
                    <line x1="15.41" y1="6.51" x2="8.59" y2="10.49" />
                  </svg>
                </label>
              </div>
              <div data-testid="kb-doc-tags" style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
                {(doc.tags || []).map((t) => (
                  <span key={t} style={{ padding: '2px 8px', borderRadius: '6px', background: 'rgba(92,124,250,0.1)', color: '#5c7cfa', fontSize: '11px' }}>{t}</span>
                ))}
              </div>
              <button data-testid={`kb-doc-delete-${doc.id}`} onClick={() => handleDelete(doc.id)}
                style={{ background: 'transparent', border: '1px solid rgba(239,68,68,0.3)', color: '#ef4444',
                  borderRadius: '6px', padding: '4px 10px', fontSize: '12px', cursor: 'pointer' }}>
                🗑
              </button>
            </div>
          ))}
          {docs.length === 0 && (
            <div className="glass p-12 text-center">
              <p className="text-sm text-[var(--text-secondary)]">{t('emptyHint')}</p>
            </div>
          )}
        </div>

        {/* Pagination */}
        <Pagination page={page} total={total} pageSize={PAGE_SIZE} onChange={setPage} testIdPrefix="kb" />
      </div>
    </AppLayout>
  );
}

const statusLabel = (t: (key: string) => string, s: string, progress: number) => {
  const keys: Record<string, string> = { ready: 'status.ready', indexing: 'status.indexing', uploaded: 'status.uploaded', failed: 'status.failed', pending: 'status.pending' };
  const base = keys[s] ? t(keys[s]) : s;
  if (s === 'indexing') return `${base} ${progress}%`;
  return base;
};
const statusIcon = (s: string) => {
  const m: Record<string, string> = { ready: '', indexing: '🔄', uploaded: '📤', failed: '❌' };
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

// File → base64 data URL（浏览器端读取图片）。
function fileToDataUrl(file: File, errMsg: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(new Error(errMsg));
    reader.readAsDataURL(file);
  });
}
