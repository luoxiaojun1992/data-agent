'use client';

import React, { useState, useEffect, useCallback } from 'react';

export interface AuthState {
  token: string | null;
  userId: string | null;
  username: string | null;
  role: string | null;
  needChangePw: boolean;
  permissions: string[];
  hydrated: boolean;
}

// Sidebar + admin menu permission keys used by canAccess.
export const SIDEBAR_PERMS = {
  dashboard: 'sidebar:dashboard',
  chat: 'sidebar:chat',
  hermes: 'sidebar:hermes',
  agent: 'sidebar:agent',
  knowledge: 'sidebar:knowledge',
  artifact: 'sidebar:artifact',
  im: 'sidebar:im',
  stats: 'sidebar:stats',
  memory: 'sidebar:memory',
  admin: 'sidebar:admin',
} as const;

export const ADMIN_MENU_PERMS = {
  models: 'admin:menu:models',
  skills: 'admin:menu:skills',
  users: 'admin:menu:users',
  rbac: 'admin:menu:rbac',
  invites: 'admin:menu:invites',
  audit: 'admin:menu:audit',
  settings: 'admin:menu:settings',
  apiCollections: 'admin:menu:api-collections',
} as const;

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1';

function boolLS(key: string): boolean {
  return localStorage.getItem(key) === 'true';
}

export function useAuth() {
  // Always start with the same state on server and client to avoid
  // hydration mismatches (React #418). The useEffect below loads the
  // real values from localStorage after mount.
  const [auth, setAuth] = useState<AuthState>({
    token: null,
    userId: null,
    username: null,
    role: null,
    needChangePw: false,
    permissions: [],
    hydrated: false,
  });

  useEffect(() => {
    // Load token from localStorage on mount (client only).
    const token = localStorage.getItem('token');
    const userId = localStorage.getItem('userId');
    const username = localStorage.getItem('username');
    const role = localStorage.getItem('role');
    const needChangePw = boolLS('needChangePw');
    let perms: string[] = [];
    try { perms = JSON.parse(localStorage.getItem('permissions') || '[]'); } catch { }
    if (token) {
      setAuth({ token, userId, username, role, needChangePw, permissions: perms, hydrated: true });
    } else {
      setAuth({ token: null, userId: null, username: null, role: null, needChangePw: false, permissions: [], hydrated: true });
    }
  }, []);

  const login = useCallback(async (username: string, password: string) => {
    const res = await fetch(`${API_BASE}/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || 'Login failed');
    }
    const data = await res.json();
    localStorage.setItem('token', data.access_token);
    localStorage.setItem('userId', data.user_id);
    localStorage.setItem('username', data.username);
    localStorage.setItem('role', data.role);
    localStorage.setItem('needChangePw', String(!!data.need_change_pw));
    // Load RBAC permissions
    let perms: string[] = [];
    try {
      const pRes = await fetch(`${API_BASE}/rbac/my-permissions`, {
        headers: { 'Authorization': `Bearer ${data.access_token}` },
      });
      if (pRes.ok) {
        const pData = await pRes.json();
        perms = pData.permissions || [];
      }
    } catch { /* ignore — sidebar will show no items if perms unavailable */ }
    localStorage.setItem('permissions', JSON.stringify(perms));
    setAuth({
      token: data.access_token,
      userId: data.user_id,
      username: data.username,
      role: data.role,
      needChangePw: !!data.need_change_pw,
      permissions: perms,
      hydrated: true,
    });
    return data;
  }, []);

  const logout = useCallback(() => {
    localStorage.removeItem('token');
    localStorage.removeItem('userId');
    localStorage.removeItem('username');
    localStorage.removeItem('role');
    localStorage.removeItem('needChangePw');
    setAuth({ token: null, userId: null, username: null, role: null, needChangePw: false, permissions: [], hydrated: true });
  }, []);

  const apiFetch = useCallback(async (path: string, options: RequestInit = {}) => {
    const headers: Record<string, string> = {
      ...(options.headers as Record<string, string> || {}),
    };
    // Only set Content-Type if not already set by caller and not FormData (browser auto-sets multipart boundary)
    if (!headers['Content-Type'] && !(options.body instanceof FormData)) {
      headers['Content-Type'] = 'application/json';
    }
    // Refuse to send if auth isn't hydrated yet — prevents 401 races where
    // a useEffect fires before localStorage token is loaded. Callers should
    // gate their fetch behind `auth.hydrated === true`.
    if (!auth.hydrated) {
      throw new Error('auth not hydrated yet');
    }
    if (auth.token) {
      headers['Authorization'] = `Bearer ${auth.token}`;
    }
    // Strip leading /api/v1 from path to avoid double-prefix (API_BASE already includes it).
    const normalized = path.startsWith('/api/v1') ? path.slice(7) : path;
    const res = await fetch(`${API_BASE}${normalized}`, { ...options, headers });
    if (res.status === 401 && auth.token) {
      logout();
      throw new Error('Session expired');
    }
    return res;
  }, [auth.token, logout]);

  const canAccess = useCallback((perm: string) => {
    return auth.permissions.includes(perm);
  }, [auth.permissions]);

  return { auth, login, logout, apiFetch, canAccess };
}
