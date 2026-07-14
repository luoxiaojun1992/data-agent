'use client';

import React, { useState, useEffect, useCallback } from 'react';

export interface AuthState {
  token: string | null;
  userId: string | null;
  username: string | null;
  role: string | null;
  needChangePw: boolean;
  hydrated: boolean;
}

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1';

function boolLS(key: string): boolean {
  return localStorage.getItem(key) === 'true';
}

export function useAuth() {
  const [auth, setAuth] = useState<AuthState>({
    token: null,
    userId: null,
    username: null,
    role: null,
    needChangePw: false,
    hydrated: false,
  });

  useEffect(() => {
    const token = localStorage.getItem('token');
    const userId = localStorage.getItem('userId');
    const username = localStorage.getItem('username');
    const role = localStorage.getItem('role');
    const needChangePw = boolLS('needChangePw');
    if (token) {
      setAuth({ token, userId, username, role, needChangePw, hydrated: true });
    } else {
      setAuth({ token: null, userId: null, username: null, role: null, needChangePw: false, hydrated: true });
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
    setAuth({
      token: data.access_token,
      userId: data.user_id,
      username: data.username,
      role: data.role,
      needChangePw: !!data.need_change_pw,
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
    setAuth({ token: null, userId: null, username: null, role: null, needChangePw: false, hydrated: true });
  }, []);

  const apiFetch = useCallback(async (path: string, options: RequestInit = {}) => {
    const headers: Record<string, string> = {
      ...(options.headers as Record<string, string> || {}),
    };
    // Only set Content-Type if not already set by caller and not FormData (browser auto-sets multipart boundary)
    if (!headers['Content-Type'] && !(options.body instanceof FormData)) {
      headers['Content-Type'] = 'application/json';
    }
    if (auth.token) {
      headers['Authorization'] = `Bearer ${auth.token}`;
    }
    const res = await fetch(`${API_BASE}${path}`, { ...options, headers });
    if (res.status === 401 && auth.token) {
      logout();
      throw new Error('Session expired');
    }
    return res;
  }, [auth.token, logout]);

  return { auth, login, logout, apiFetch };
}
