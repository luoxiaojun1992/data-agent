/**
 * SPEC-046: Dashboard data fixture — seeds real tasks, sessions, and KB docs
 * via the backend API so Dashboard shows non-zero KPIs.
 */
const API_BASE = 'http://data-agent:8080/api/v1';

export interface DashboardSeedOpts {
  taskCount?: number;
  sessionCount?: number;
  docCount?: number;
}

export interface SeedResult {
  taskIds: string[];
  sessionIds: string[];
  docIds: string[];
}

/**
 * Seed dashboard data: create tasks, sessions, and KB docs.
 * Returns created IDs for cleanup.
 */
export async function seedDashboardData(
  request: any,
  token: string,
  opts: DashboardSeedOpts = {}
): Promise<SeedResult> {
  const { taskCount = 5, sessionCount = 3, docCount = 3 } = opts;
  const taskIds: string[] = [];
  const sessionIds: string[] = [];
  const docIds: string[] = [];

  // Create sessions
  for (let i = 0; i < sessionCount; i++) {
    const res = await request.post(`${API_BASE}/sessions`, {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      data: { title: `Dashboard Session ${i + 1}` },
    });
    if (res.ok()) {
      const s = await res.json();
      sessionIds.push(s.id || s.session_id);
    }
  }

  // Create tasks
  for (let i = 0; i < taskCount; i++) {
    const res = await request.post(`${API_BASE}/tasks`, {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      data: {
        title: `Dashboard Task ${i + 1}`,
        skills: ['sql_executor'],
        async: false,
      },
    });
    if (res.ok()) {
      const t = await res.json();
      taskIds.push(t.task_id || t.id);
    }
  }

  // Create KB docs
  for (let i = 0; i < docCount; i++) {
    const res = await request.post(`${API_BASE}/knowledge/docs`, {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      data: {
        title: `Dashboard Doc ${i + 1}`,
        file_name: `dashboard-doc-${i + 1}.md`,
        file_type: 'markdown',
        size_bytes: 1000,
      },
    });
    if (res.ok()) {
      const d = await res.json();
      docIds.push(d.id);
    }
  }

  return { taskIds, sessionIds, docIds };
}

/**
 * Cleanup seeded dashboard data.
 */
export async function cleanupDashboardData(
  request: any,
  token: string,
  seed: SeedResult
): Promise<void> {
  const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };

  for (const id of seed.taskIds) {
    await request.delete(`${API_BASE}/tasks/${id}`, { headers }).catch(() => {});
  }
  for (const id of seed.sessionIds) {
    await request.delete(`${API_BASE}/sessions/${id}`, { headers }).catch(() => {});
  }
  for (const id of seed.docIds) {
    await request.delete(`${API_BASE}/knowledge/docs/${id}`, { headers }).catch(() => {});
  }
}
