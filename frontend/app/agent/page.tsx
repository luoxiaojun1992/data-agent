'use client';

import React, { useState, useEffect } from 'react';
import AppLayout from '../providers';
import { useAuth } from '@/lib/api';

interface AgentTask {
  task_id: string;
  title: string;
  status: string;
  created_at: string;
}

export default function AgentPage() {
  const { apiFetch } = useAuth();
  const [tasks, setTasks] = useState<AgentTask[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadTasks();
  }, []);

  const loadTasks = async () => {
    try {
      const res = await apiFetch('/agent/tasks');
      const data = await res.json();
      setTasks(data.tasks || []);
    } catch (err) {
      console.error('Failed to load tasks:', err);
    } finally {
      setLoading(false);
    }
  };

  const statusColor = (status: string) => {
    switch (status) {
      case 'running': return 'text-blue-400 bg-blue-400/10';
      case 'completed': return 'text-emerald-400 bg-emerald-400/10';
      case 'failed': return 'text-red-400 bg-red-400/10';
      default: return 'text-[var(--text-secondary)] bg-[var(--glass-bg)]';
    }
  };

  const statusLabel = (status: string) => {
    switch (status) {
      case 'pending': return '等待中';
      case 'running': return '运行中';
      case 'completed': return '已完成';
      case 'failed': return '失败';
      default: return status;
    }
  };

  const skills = ['sql_executor', 'stats_engine', 'knowledge_search', 'save_report'];

  return (
    <AppLayout>
      <div className="animate-fade-in">
        <div className="mb-8">
          <h2 className="text-2xl font-bold text-[var(--text-primary)]">Agent 任务</h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">批量数据分析任务管理与执行</p>
        </div>

        {/* Available skills */}
        <div className="glass p-5 mb-6">
          <h3 className="text-sm font-semibold text-[var(--text-primary)] mb-3">可用技能</h3>
          <div className="flex flex-wrap gap-2">
            {skills.map((skill) => (
              <span
                key={skill}
                className="px-3 py-1.5 text-xs rounded-full bg-[var(--glass-bg)] border border-[var(--border-glass)] text-[var(--text-secondary)]"
              >
                ⚡ {skill}
              </span>
            ))}
          </div>
        </div>

        {/* Task list */}
        {loading ? (
          <div className="text-center py-12 text-[var(--text-secondary)]">加载中...</div>
        ) : tasks.length === 0 ? (
          <div className="glass p-12 text-center">
            <span className="text-5xl block mb-4">⚡</span>
            <p className="text-lg text-[var(--text-primary)] mb-2">暂无任务</p>
            <p className="text-sm text-[var(--text-secondary)] mb-6">
              Agent 任务将通过 Chat 对话自动创建
            </p>
          </div>
        ) : (
          <div className="space-y-3">
            {tasks.map((task) => (
              <div key={task.task_id} className="glass p-4 glass-hover">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-sm font-medium text-[var(--text-primary)]">{task.title}</p>
                    <p className="text-xs text-[var(--text-secondary)] mt-1">ID: {task.task_id?.slice(0, 12)}...</p>
                  </div>
                  <span className={`text-xs px-2.5 py-1 rounded-full ${statusColor(task.status)}`}>
                    {statusLabel(task.status)}
                  </span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </AppLayout>
  );
}
