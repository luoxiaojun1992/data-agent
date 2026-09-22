'use client';

import React, { useState, useEffect, useCallback } from 'react';
import AppLayout from '../../providers';
import { useAuth } from '../../../lib/api';
import Pagination from '../../components/Pagination';
import { primaryButtonStyle, modalOverlayStyle } from '../../components/ui';

interface AuditLog {
  id: string;
  action: string;
  action_desc?: string;
  user_id: string;
  resource: string;
  details: string;
  ip: string;
  status_code: number;
  created_at: string;
}

export default function AuditPage() {
  const { auth, apiFetch } = useAuth();
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [filterQ, setFilterQ] = useState('');
  const [filterPath, setFilterPath] = useState('');
  const [filterStatus, setFilterStatus] = useState('');
  const [dateStart, setDateStart] = useState('');
  const [dateEnd, setDateEnd] = useState('');
  const [showExport, setShowExport] = useState(false);
  const [exportLimit, setExportLimit] = useState(5000);
  const [error, setError] = useState('');
  const [toast, setToast] = useState<{ msg: string; type: 'success' | 'error' } | null>(null);
  const [refresh, setRefresh] = useState(0);

  const notify = (msg: string, type: 'success' | 'error') => {
    setToast({ msg, type });
    setTimeout(() => setToast(null), 3000);
  };

  const fetchLogs = useCallback(async () => {
    try {
      const params = new URLSearchParams();
      params.set('page', String(page));
      params.set('page_size', String(pageSize));
      if (filterQ) params.set('q', filterQ);
      if (filterPath) params.set('path', filterPath);
      if (filterStatus) params.set('status_class', filterStatus);
      if (dateStart) params.set('start', dateStart);
      if (dateEnd) params.set('end', dateEnd);

      const res = await apiFetch(`/admin/audit/logs?${params}`);
      if (res.ok) {
        const data = await res.json();
        setLogs(data.logs || []);
        setTotal(data.total || 0);
        setError('');
      } else {
        setError('加载失败');
      }
    } catch (err: any) {
      setError(`审计日志加载失败 — ${err?.message || err?.status || '未知错误'} (${err?.status ? `HTTP ${err.status}` : '/api/v1/audit'})`);
    }
  }, [apiFetch, page, pageSize, filterQ, filterPath, filterStatus, dateStart, dateEnd]);

  useEffect(() => {
    if (auth.hydrated) fetchLogs();
  }, [auth.hydrated, fetchLogs, refresh]);

  const handleFilter = () => {
    setPage(1);
    setRefresh((r) => r + 1);
  };

  const handleReset = () => {
    setFilterQ('');
    setFilterPath('');
    setFilterStatus('');
    setDateStart('');
    setDateEnd('');
    setPage(1);
    setRefresh((r) => r + 1);
  };

  const handleExport = async () => {
    if (exportLimit > 50000) {
      setError('单次导出最多 50,000 条');
      return;
    }
    try {
      const res = await apiFetch('/admin/audit/export', {
        method: 'POST',
        body: JSON.stringify({
          q: filterQ, path: filterPath, status_class: filterStatus,
          start: dateStart, end: dateEnd,
          limit: exportLimit, format: 'csv',
        }),
      });
      if (res.ok) {
        const blob = await res.blob();
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `audit_logs_${dateStart}_${dateEnd}.csv`;
        a.click();
        URL.revokeObjectURL(url);
        notify('导出成功', 'success');
        setShowExport(false);
      } else {
        const d = await res.json();
        setError(d.error || '导出失败');
      }
    } catch {
      notify('导出失败', 'error');
    }
  };

  return (
    <AppLayout>
      <div className="animate-fade-in" data-testid="audit-page-header">
        <div className="mb-8" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">审计日志</h2>
          <button data-testid="audit-export-btn" onClick={() => setShowExport(true)}
            style={primaryButtonStyle}>
            导出日志
          </button>
        </div>

        {/* Toast */}
        {toast && (
          <div data-testid="audit-export-success-toast" style={{ position: 'fixed', top: 20, right: 20, zIndex: 9999,
            background: toast.type === 'success' ? 'rgba(16,185,129,0.9)' : 'rgba(239,68,68,0.9)',
            color: '#fff', padding: '12px 20px', borderRadius: '8px', fontSize: '14px' }}>
            {toast.msg}
          </div>
        )}

        {/* Error */}
        {error && (
          <div data-testid="audit-export-limit-error" style={{ padding: '8px 16px', marginBottom: '12px',
            background: 'rgba(239,68,68,0.1)', borderRadius: '8px', color: '#ef4444', fontSize: '13px' }}>
            {error}
          </div>
        )}

        {/* Filter Bar */}
        <div data-testid="audit-filter-bar" className="glass" style={{ padding: '16px', marginBottom: '16px', display: 'flex', gap: '12px', flexWrap: 'wrap', alignItems: 'flex-end' }}>
          <div>
            <label style={labelStyle}>操作人邮箱</label>
            <input data-testid="audit-user-select" placeholder="按操作人邮箱搜索" value={filterQ}
              onChange={(e) => setFilterQ(e.target.value)} style={inputStyle} />
          </div>
          <div>
            <label style={labelStyle}>API 路径</label>
            <input data-testid="audit-path-input" placeholder="按 API 路径搜索" value={filterPath}
              onChange={(e) => setFilterPath(e.target.value)} style={inputStyle} />
          </div>
          <div>
            <label style={labelStyle}>状态码</label>
            <select data-testid="audit-status-select" value={filterStatus}
              onChange={(e) => setFilterStatus(e.target.value)} style={{ ...inputStyle, width: '120px' }}>
              <option value="">全部</option>
              <option value="1xx">1xx</option>
              <option value="2xx">2xx</option>
              <option value="3xx">3xx</option>
              <option value="4xx">4xx</option>
              <option value="5xx">5xx</option>
            </select>
          </div>
          <div>
            <label style={labelStyle}>开始日期</label>
            <input data-testid="audit-date-start" type="date" value={dateStart} onChange={(e) => setDateStart(e.target.value)}
              style={inputStyle} />
          </div>
          <div>
            <label style={labelStyle}>结束日期</label>
            <input data-testid="audit-date-end" type="date" value={dateEnd} onChange={(e) => setDateEnd(e.target.value)}
              style={inputStyle} />
          </div>
          <button data-testid="audit-filter-apply" onClick={handleFilter}
            style={{ padding: '8px 20px', background: '#5c7cfa', color: '#fff', border: 'none', borderRadius: '8px',
              fontSize: '13px', cursor: 'pointer', height: '36px' }}>筛选</button>
          <button data-testid="audit-filter-reset" onClick={handleReset}
            style={{ padding: '8px 20px', background: 'var(--surface-6)', border: '1px solid var(--surface-10)',
              borderRadius: '8px', fontSize: '13px', color: '#7A7A7A', cursor: 'pointer', height: '36px' }}>重置</button>
        </div>

        {/* Table */}
        <div className="glass" style={{ overflow: 'hidden' }}>
          <table data-testid="audit-table" style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ background: 'var(--surface-3)' }}>
                <th style={thStyle}>时间</th>
                <th style={thStyle}>操作人</th>
                <th style={thStyle}>操作类型</th>
                <th style={thStyle}>查询参数</th>
                <th style={thStyle}>IP</th>
              </tr>
            </thead>
            <tbody>
              {logs.map((log) => (
                <tr key={log.id} data-testid={`audit-row-${log.id}`} style={{ borderBottom: '1px solid var(--surface-6)' }}>
                  <td data-testid="audit-row-time" style={tdStyle}>{log.created_at ? new Date(log.created_at).toLocaleString('zh-CN') : '—'}</td>
                  <td data-testid="audit-row-user" style={tdStyle}>{log.user_id || '—'}</td>
                  <td data-testid="audit-row-type" style={tdStyle}><span style={actionPill}>{log.action_desc || log.action}</span></td>
                  <td data-testid="audit-row-detail" style={tdStyle}>{log.details?.slice(0, 30) || '—'}</td>
                  <td data-testid="audit-row-ip" style={tdStyle}>{log.ip || '—'}</td>
                </tr>
              ))}
              {logs.length === 0 && (
                <tr><td colSpan={5} style={{ ...tdStyle, textAlign: 'center', padding: '40px' }}>
                  <span className="text-sm text-[var(--text-secondary)]">暂无审计日志</span>
                </td></tr>
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        <Pagination
          page={page}
          total={total}
          pageSize={pageSize}
          onChange={setPage}
          onPageSizeChange={(s) => { setPageSize(s); setPage(1); }}
          testIdPrefix="audit"
        />

        {/* Export Modal */}
        {showExport && (
          <div data-testid="audit-export-modal" style={{ ...modalOverlayStyle, zIndex: 999 }}
            onClick={(e) => { if (e.target === e.currentTarget) setShowExport(false); }}>
            <div className="glass" style={{ padding: '24px', maxWidth: '440px', width: '90%' }}>
              <h3 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '16px' }}>导出审计日志</h3>
              <div style={{ display: 'flex', gap: '12px', marginBottom: '12px' }}>
                <div style={{ flex: 1 }}>
                  <label style={labelStyle}>开始日期</label>
                  <input data-testid="audit-export-date-start" type="date" value={dateStart} onChange={(e) => setDateStart(e.target.value)} style={inputStyle} />
                </div>
                <div style={{ flex: 1 }}>
                  <label style={labelStyle}>结束日期</label>
                  <input data-testid="audit-export-date-end" type="date" value={dateEnd} onChange={(e) => setDateEnd(e.target.value)} style={inputStyle} />
                </div>
              </div>
              <div style={{ marginBottom: '12px' }}>
                <label style={labelStyle}>导出条数上限（最大 50,000）</label>
                <input data-testid="audit-export-limit" type="number" value={exportLimit} min={1} max={50000}
                  onChange={(e) => setExportLimit(Number(e.target.value))} style={inputStyle} />
              </div>
              <div style={{ marginBottom: '16px' }}>
                <label style={labelStyle}>导出格式</label>
                <div style={{ display: 'flex', gap: '10px', marginTop: '4px' }}>
                  <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
                    <input data-testid="audit-export-format-csv" type="radio" name="exportFormat" value="csv"
                      checked onChange={() => {}} />
                    <span style={{ fontSize: '13px', color: '#7A7A7A' }}>CSV</span>
                  </label>
                </div>
              </div>
              <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
                <button onClick={() => setShowExport(false)}
                  style={{ padding: '8px 20px', background: 'var(--surface-6)', border: '1px solid var(--surface-10)',
                    borderRadius: '8px', fontSize: '13px', color: '#7A7A7A', cursor: 'pointer' }}>取消</button>
                <button data-testid="audit-export-submit" onClick={handleExport}
                  style={{ padding: '8px 20px', background: 'linear-gradient(135deg, #5c7cfa, #7c3aed)', color: '#fff',
                    border: 'none', borderRadius: '8px', fontSize: '13px', cursor: 'pointer' }}>确认导出</button>
              </div>
            </div>
          </div>
        )}
      </div>
    </AppLayout>
  );
}

const actionPill: React.CSSProperties = {
  display: 'inline-block', padding: '2px 10px', borderRadius: '10px', fontSize: '12px', fontWeight: 500,
  background: 'rgba(92,124,250,0.15)', color: '#5c7cfa',
};

const labelStyle: React.CSSProperties = { display: 'block', fontSize: '11px', color: '#666', marginBottom: '4px' };
const inputStyle: React.CSSProperties = {
  width: '100%', padding: '8px 12px', background: 'var(--surface-6)',
  border: '1px solid var(--surface-10)', borderRadius: '8px', fontSize: '14px',
  color: 'var(--text-primary)', outline: 'none', boxSizing: 'border-box',
};
const thStyle: React.CSSProperties = { padding: '12px 10px', textAlign: 'left', fontSize: '11px', fontWeight: 700, textTransform: 'uppercase', color: '#666', borderBottom: '1px solid var(--surface-6)' };
const tdStyle: React.CSSProperties = { padding: '10px', fontSize: '13px', color: '#7A7A7A' };
