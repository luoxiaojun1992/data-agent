'use client';

import React, { useState, useEffect, useCallback } from 'react';

export interface AuthState {
  token: string | null;
  userId: string | null;
  username: string | null;
  role: string | null;
}

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080/api/v1';

export function useAuth() {
  const [auth, setAuth] = useState<AuthState>({
    token: null,
    userId: null,
    username: null,
    role: null,
  });

  useEffect(() => {
    const token = localStorage.getItem('token');
    const userId = localStorage.getItem('userId');
    const username = localStorage.getItem('username');
    const role = localStorage.getItem('role');
    if (token) {
      setAuth({ token, userId, username, role });
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
    setAuth({
      token: data.access_token,
      userId: data.user_id,
      username: data.username,
      role: data.role,
    });
    return data;
  }, []);

  const logout = useCallback(() => {
    localStorage.removeItem('token');
    localStorage.removeItem('userId');
    localStorage.removeItem('username');
    localStorage.removeItem('role');
    setAuth({ token: null, userId: null, username: null, role: null });
  }, []);

  const apiFetch = useCallback(async (path: string, options: RequestInit = {}) => {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(options.headers as Record<string, string> || {}),
    };
    if (auth.token) {
      headers['Authorization'] = `Bearer ${auth.token}`;
    }
    const res = await fetch(`${API_BASE}${path}`, { ...options, headers });
    if (res.status === 401) {
      logout();
      throw new Error('Session expired');
    }
    return res;
  }, [auth.token, logout]);

  return { auth, login, logout, apiFetch };
}
