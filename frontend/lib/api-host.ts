'use client';

// SPEC-097: runtime-resolved API base. The backend address is no longer a
// build-time constant — it is fetched from GET /api/v1/api-host (a fully
// public endpoint) at runtime, falling back to the current frontend origin
// when the request fails or the configured value is empty (D4). The `/api/v1`
// suffix is appended exactly once here (D2): the configured api_host and the
// fallback origin are both pure origins, so every consumer gets the full
// base from this single exit point.

// cachedBase holds the resolved base once known; pending deduplicates
// concurrent resolution so a burst of pre-auth requests only hits
// /api/v1/api-host once.
let cachedBase: string | null = null;
let pending: Promise<string> | null = null;

async function resolveApiBase(): Promise<string> {
  let origin = '';
  try {
    // Relative path on purpose: before login there is no known API host yet,
    // so the probe itself must ride the same-origin nginx proxy.
    const res = await fetch('/api/v1/api-host', { cache: 'no-store' });
    if (res.ok) {
      const data = await res.json();
      if (data && typeof data.api_host === 'string' && data.api_host !== '') {
        origin = data.api_host;
      }
    }
  } catch {
    // Network/proxy failure → fall back to the frontend origin below.
  }
  if (!origin) {
    origin = window.location.origin;
  }
  return origin + '/api/v1';
}

export function getApiHost(): Promise<string> {
  if (cachedBase) {
    return Promise.resolve(cachedBase);
  }
  if (!pending) {
    pending = resolveApiBase().then((base) => {
      cachedBase = base;
      return base;
    }).finally(() => {
      pending = null;
    });
  }
  return pending;
}
