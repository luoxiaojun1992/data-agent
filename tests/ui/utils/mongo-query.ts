/**
 * SPEC-046: MongoDB query helpers for E2E integration tests.
 * Uses the backend API to verify data state (no direct DB access in tests).
 */

const API_BASE = 'http://data-agent:8080/api/v1';

export interface KBDocStatus {
  id: string;
  status: string;
  chunk_count: number;
  indexed_at?: string;
}

export interface DashboardKPI {
  active_sessions: number;
  agent_tasks: number;
  kb_docs: number;
  system_availability: number;
  task_stats: {
    total: number;
    pending: number;
    running: number;
    completed: number;
    failed: number;
    cancelled: number;
  };
  token_stats?: {
    total_input: number;
    total_output: number;
    roi: number;
  };
}

/**
 * Get KB document indexing status via backend API.
 */
export async function getKBDocStatus(token: string, docId: string): Promise<KBDocStatus | null> {
  try {
    const res = await fetch(`${API_BASE}/knowledge/docs/${docId}/status`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!res.ok) return null;
    return res.json();
  } catch {
    return null;
  }
}

/**
 * Wait for KB document indexing to complete.
 * Retries every 2s up to timeoutMs.
 */
export async function waitForKBIndex(
  token: string,
  docId: string,
  timeoutMs = 30000
): Promise<boolean> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    const status = await getKBDocStatus(token, docId);
    if (status && (status.status === 'ready' || status.status === 'indexed')) {
      return true;
    }
    if (status && status.status === 'failed') {
      return false;
    }
    await new Promise((r) => setTimeout(r, 2000));
  }
  return false;
}

/**
 * Real KB search via backend API.
 */
export async function getKBSearchHits(
  token: string,
  query: string,
  topK = 5
): Promise<Array<{ doc_id: string; chunk_text: string; score: number }>> {
  try {
    const res = await fetch(`${API_BASE}/knowledge/search`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ query, top_k: topK }),
    });
    if (!res.ok) return [];
    const data = await res.json();
    return data.results || data.hits || [];
  } catch {
    return [];
  }
}

/**
 * Get Dashboard KPIs via backend API.
 */
export async function getDashboardKPIs(token: string): Promise<DashboardKPI | null> {
  try {
    const res = await fetch(`${API_BASE}/dashboard/stats`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!res.ok) return null;
    return res.json();
  } catch {
    return null;
  }
}

/**
 * Get task details via backend API.
 */
export async function getTaskById(token: string, taskId: string): Promise<any> {
  try {
    const res = await fetch(`${API_BASE}/tasks/${taskId}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!res.ok) return null;
    return res.json();
  } catch {
    return null;
  }
}
