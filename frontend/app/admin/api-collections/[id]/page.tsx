'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { useParams, useRouter } from 'next/navigation';
import AppLayout from '../../../providers';
import { useAuth } from '../../../../lib/api';

const STATUS_LABELS: Record<string, { text: string; color: string }> = {
  pending: { text: '待审核', color: '#f59e0b' },
  approved: { text: '已通过', color: '#10b981' },
  rejected: { text: '已拒绝', color: '#ef4444' },
};

export default function APICollectionDetailPage() {
  const { auth, apiFetch } = useAuth();
  const { id } = useParams() as { id: string };
  const router = useRouter();
  const [collection, setCollection] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState('');
  const [editDesc, setEditDesc] = useState('');

  const load = useCallback(async () => {
    const res = await apiFetch(`/admin/api-collections/${id}`);
    if (res.ok) {
      const data = await res.json();
      setCollection(data);
      setEditName(data.name);
      setEditDesc(data.description);
    }
    setLoading(false);
  }, [apiFetch, id]);

  useEffect(() => { load(); }, [load]);

  const handleApprove = async (status: string) => {
    await apiFetch(`/admin/api-collections/${id}/approve`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status }),
    });
    load();
  };

  const handleUpdate = async () => {
    await apiFetch(`/admin/api-collections/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: editName, description: editDesc }),
    });
    setEditing(false);
    load();
  };

  if (loading) return <AppLayout><div className="animate-fade-in p-8 text-[var(--text-secondary)]">加载中...</div></AppLayout>;
  if (!collection) return <AppLayout><div className="animate-fade-in p-8 text-[var(--text-secondary)]">未找到</div></AppLayout>;

  const isSysAdmin = auth.role === 'system_admin';

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <button onClick={() => router.back()} className="text-[var(--text-secondary)] hover:text-[var(--text-primary)] mb-4 text-sm">← 返回</button>

        <div className="mb-6">
          {editing ? (
            <div className="space-y-3 mb-4">
              <input value={editName} onChange={e => setEditName(e.target.value)} className="w-full px-3 py-2 rounded-lg bg-white/5 border border-white/10 text-[var(--text-primary)] text-lg font-bold" />
              <input value={editDesc} onChange={e => setEditDesc(e.target.value)} className="w-full px-3 py-2 rounded-lg bg-white/5 border border-white/10 text-[var(--text-primary)] text-sm" />
              <div className="flex gap-2">
                <button onClick={handleUpdate} className="px-4 py-1.5 bg-[#B1E2FF] text-black rounded text-sm">保存</button>
                <button onClick={() => setEditing(false)} className="px-4 py-1.5 bg-white/10 text-[var(--text-secondary)] rounded text-sm">取消</button>
              </div>
            </div>
          ) : (
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-2xl font-bold text-[var(--text-primary)]">{collection.name}</h2>
                <p className="text-sm text-[var(--text-secondary)] mt-1">{collection.description}</p>
                <span style={{ color: STATUS_LABELS[collection.status]?.color }} className="text-xs mt-2 inline-block">
                  {STATUS_LABELS[collection.status]?.text}
                </span>
              </div>
              <div className="flex gap-2">
                <button onClick={() => setEditing(true)} className="px-3 py-1.5 bg-white/10 text-[var(--text-secondary)] rounded text-sm hover:bg-white/20">编辑</button>
                {isSysAdmin && (
                  <div className="flex gap-2">
                    {collection.status !== 'approved' && (
                      <button onClick={() => handleApprove('approved')} className="px-3 py-1.5 bg-green-600 text-white rounded text-sm hover:bg-green-700">通过</button>
                    )}
                    {collection.status !== 'rejected' && (
                      <button onClick={() => handleApprove('rejected')} className="px-3 py-1.5 bg-red-600 text-white rounded text-sm hover:bg-red-700">拒绝</button>
                    )}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>

        <div className="mt-6">
          <h3 className="text-lg font-semibold text-[var(--text-primary)] mb-3">API 列表 ({collection.api_count})</h3>
          {collection.openapi_spec?.paths && Object.keys(collection.openapi_spec.paths).length > 0 ? (
            <div className="space-y-2">
              {Object.entries(collection.openapi_spec.paths as Record<string, any>).map(([path, methods]: [string, any]) =>
                Object.keys(methods || {}).filter(m => !['parameters','servers','summary','description'].includes(m)).map(method => (
                  <div key={`${path}-${method}`} className="p-3 rounded-lg bg-white/5 border border-white/10 flex items-center gap-3">
                    <span className="uppercase text-xs px-2 py-0.5 rounded bg-[#B1E2FF]/20 text-[#B1E2FF] font-mono">{method}</span>
                    <span className="text-sm text-[var(--text-primary)] font-mono">{path}</span>
                    {methods[method]?.summary && <span className="text-xs text-[var(--text-secondary)] ml-auto">{methods[method].summary}</span>}
                  </div>
                ))
              )}
            </div>
          ) : (
            <p className="text-sm text-[var(--text-secondary)]">暂无 API 路径</p>
          )}
        </div>
      </div>
    </AppLayout>
  );
}
