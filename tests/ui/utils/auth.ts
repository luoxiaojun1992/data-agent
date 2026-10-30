import { expect, type APIRequestContext } from '@playwright/test';

// 与 docker-compose.ui-test.yml / docker/seed-admin.js 预置的 system_admin 对齐。
// SPEC-084 移除了自助注册（POST /auth/register），用户改由 admin 经 POST /users 创建。
export const API_BASE = 'http://data-agent:8080/api/v1';
export const ADMIN = { username: 'admin@admin.com', password: 'Admin@12345' };

// adminLogin 返回 system_admin 的 access_token。
export async function adminToken(request: APIRequestContext): Promise<string> {
  const res = await request.post(`${API_BASE}/auth/login`, { data: ADMIN });
  expect(res.status()).toBe(200);
  const body = await res.json();
  return body.access_token as string;
}

// registerUser 等价于 SPEC-084 移除前的 POST /auth/register 自助注册：
// 先以 system_admin 登录，再 POST /users 创建用户，并绑定默认 RBAC 角色。
// 返回创建响应，由调用方自行断言状态码（与原 register 断言保持一致）。
export async function registerUser(
  request: APIRequestContext,
  user: { username: string; password: string; role?: string },
) {
  const token = await adminToken(request);
  const headers = { Authorization: `Bearer ${token}` };
  const res = await request.post(`${API_BASE}/users`, { data: user, headers });
  // POST /users 不会自动绑定 RBAC 角色，而未绑定角色的用户在 SPEC-084 的
  // 权限门禁下访问任何功能都会 403。按 user role 绑定默认角色：
  //   admin -> rbac_role_admin，其余 -> rbac_role_user
  if (res.status() === 201) {
    const created = await res.json();
    const roleId = user.role === 'admin' ? 'rbac_role_admin' : 'rbac_role_user';
    await request.post(`${API_BASE}/users/${created.id}/rbac-roles`, {
      data: { role_id: roleId },
      headers,
    });
  }
  return res;
}
