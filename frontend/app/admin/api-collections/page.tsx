'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import AppLayout from '../../providers';
import { useAuth } from '../../../lib/api';
import Pagination from '../../components/Pagination';
import { primaryButtonStyle, modalOverlayStyle } from '../../components/ui';

const STATUS_LABELS: Record<string, { text: string; color: string }> = {
  pending: { text: '待审核', color: '#f59e0b' },
  approved: { text: '已通过', color: '#10b981' },
  rejected: { text: '已拒绝', color: '#ef4444' },
};

export default function APICollectionsPage() {
  const { auth, apiFetch } = useAuth();
  const router = useRouter();
  const [collections, setCollections] = useState<any[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [showUpload, setShowUpload] = useState(false);
  const [uploadName, setUploadName] = useState('');
  const [uploadDesc, setUploadDesc] = useState('');
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState('');
  const [pageSize, setPageSize] = useState(20);

  const load = useCallback(async () => {
    const res = await apiFetch(`/admin/api-collections?page=${page}&page_size=${pageSize}`);
    const data = await res.json();
    setCollections(data.items || []);
    setTotal(data.total || 0);
  }, [apiFetch, page, pageSize]);

  useEffect(() => { load(); }, [load]);

  const handleUpload = async () => {
    if (!uploadFile || !uploadName) return;
    setUploading(true);
    setError('');
    const formData = new FormData();
    formData.append('name', uploadName);
    formData.append('description', uploadDesc);
    formData.append('file', uploadFile);
    const res = await apiFetch('/admin/api-collections', { method: 'POST', body: formData });
    if (!res.ok) {
      const e = await res.json();
      setError(e.error || '上传失败');
      setUploading(false);
      return;
    }
    setShowUpload(false);
    setUploadName('');
    setUploadDesc('');
    setUploadFile(null);
    setUploading(false);
    load();
  };

  const handleDelete = async (id: string) => {
    if (!confirm('确定删除该 API 集合？')) return;
    await apiFetch(`/admin/api-collections/${id}`, { method: 'DELETE' });
    load();
  };

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">API 管理</h2>
          <button onClick={() => setShowUpload(true)} style={primaryButtonStyle}>
            + 上传 OpenAPI 文件
          </button>
        </div>

        {showUpload && (
          <div style={modalOverlayStyle} onClick={() => setShowUpload(false)}>
            <div className="rounded-xl p-6 w-full max-w-md"
              style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border-glass)' }}
              onClick={e => e.stopPropagation()}>
              <h3 className="text-lg font-semibold text-[var(--text-primary)] mb-4">上传 API 集合</h3>
              <div className="space-y-3">
                <div>
                  <input placeholder="名称" value={uploadName} onChange={e => setUploadName(e.target.value)}
                    className="w-full px-3 py-2 rounded-lg bg-white/5 border border-white/10 text-[var(--text-primary)] text-sm" />
                </div>
                <div>
                  <input placeholder="描述" value={uploadDesc} onChange={e => setUploadDesc(e.target.value)}
                    className="w-full px-3 py-2 rounded-lg bg-white/5 border border-white/10 text-[var(--text-primary)] text-sm" />
                </div>
                <div>
                  <input type="file" accept=".json,.yaml,.yml" onChange={e => setUploadFile(e.target.files?.[0] || null)}
                    className="w-full text-sm text-[var(--text-secondary)]" />
                </div>
                {error && <p className="text-red-400 text-sm">{error}</p>}
                <button onClick={handleUpload} disabled={uploading}
                  className="w-full py-2 bg-[#B1E2FF] text-black rounded-lg text-sm font-medium hover:opacity-80 disabled:opacity-50">
                  {uploading ? '上传中...' : '确认上传'}
                </button>
              </div>
            </div>
          </div>
        )}

        <div className="overflow-x-auto">
          <table className="w-full text-sm text-[var(--text-primary)]">
            <thead>
              <tr className="border-b border-white/10 text-left text-[var(--text-secondary)]">
                <th className="py-3 px-4">名称</th>
                <th className="py-3 px-4">描述</th>
                <th className="py-3 px-4">状态</th>
                <th className="py-3 px-4">API 数</th>
                <th className="py-3 px-4">上传时间</th>
                <th className="py-3 px-4">操作</th>
              </tr>
            </thead>
            <tbody>
              {collections.map((c: any) => (
                <tr key={c.id} className="border-b border-white/5 hover:bg-white/5">
                  <td className="py-3 px-4">{c.name}</td>
                  <td className="py-3 px-4 text-[var(--text-secondary)] max-w-[200px] truncate">{c.description}</td>
                  <td className="py-3 px-4">
                    <span style={{ color: STATUS_LABELS[c.status]?.color || '#888' }}>{STATUS_LABELS[c.status]?.text || c.status}</span>
                  </td>
                  <td className="py-3 px-4">{c.api_count}</td>
                  <td className="py-3 px-4 text-[var(--text-secondary)]">{new Date(c.created_at).toLocaleDateString()}</td>
                  <td className="py-3 px-4 space-x-2">
                    <button onClick={() => router.push(`/admin/api-collections/${c.id}`)} className="text-[#B1E2FF] hover:underline text-xs">详情</button>
                    <button onClick={() => handleDelete(c.id)} className="text-red-400 hover:underline text-xs">删除</button>
                  </td>
                </tr>
              ))}
              {collections.length === 0 && <tr><td colSpan={6} className="py-8 text-center text-[var(--text-secondary)]">暂无数据</td></tr>}
            </tbody>
          </table>
        </div>
        <Pagination
          page={page}
          total={total}
          pageSize={pageSize}
          onChange={setPage}
          onPageSizeChange={(s) => { setPageSize(s); setPage(1); }}
          testIdPrefix="api-collections"
        />
      </div>
    </AppLayout>
  );
}
