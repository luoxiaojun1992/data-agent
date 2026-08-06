# SPEC-065: API 注册与 MCP 工具集成

> 版本: 1.0 | 日期: 2026-08-06 | 状态: Draft

## 1. 概述

允许管理员上传 OpenAPI 3.0 规范文档，将外部 HTTP API 注册为可被 Agent Skill 调用的工具函数。

## 2. 数据模型

### 2.1 API 集合 (api_collections)

```json
{
  "_id": "uuid",
  "name": "string",
  "description": "string",
  "status": "pending | approved | rejected",
  "openapi_spec": { /* 解析后的 OpenAPI 对象 */ },
  "raw_spec_file_id": "seaweedfs_file_id",
  "user_id": "uploader_user_id",
  "api_count": 0,
  "created_at": "ISODate",
  "updated_at": "ISODate"
}
```

### 2.2 状态枚举

```
pending    — 待审核（默认）
approved   — 审核通过
rejected   — 拒绝
```

## 3. API 设计

### 3.1 管理接口（/api/v1/admin/api-collections）

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | / | user:view | 列表（分页）。admin 只能看自己的，system_admin 看全部 |
| POST | / | user:create | 上传 OpenAPI 文件（multipart/form-data） |
| GET | /:id | user:view | 详情（含解析后的 API 文档） |
| PUT | /:id | user:edit | 编辑 name/description |
| DELETE | /:id | user:edit | 删除（仅 own） |
| POST | /:id/approve | system:edit | 审批通过/拒绝（body: {status:"approved|rejected"}） |

### 3.2 Skill 工具接口（/api/v1/tools/api）

| 工具名 | 权限 | 说明 |
|--------|------|------|
| external_api_search | chat:view | 模糊搜索 API 集合描述（limit=10，审核通过的集合） |
| external_api_summary | chat:view | 查询某个集合有哪些 API（分页：max 100 pages, max 10/page） |
| external_api_method | chat:view | 查询某个 API 方法详情（入参/出参） |
| external_api_call | chat:view | 调用外部 API（透传参数） |

## 4. RBAC 权限

### 4.1 权限常量

```go
PermAPICollectionView   = "api:collection:view"   // → admin_role
PermAPICollectionEdit   = "api:collection:edit"   // → admin_role
PermAPICollectionApprove = "api:collection:approve" // → sysAdmin_role only
```

### 4.2 Sidebar / Admin 菜单

- Sidebar: 不需要（API 管理仅在管理后台）
- Admin 菜单: `admin:menu:api-collections` → admin_role + sysAdmin_role (继承)

### 4.3 数据隔离

- API handler 入口：admin 用户只能操作 `user_id == current_user_id` 的记录
- system_admin 无限制（可查看/操作所有 API 集合）

## 5. 前端

### 5.1 管理后台入口

- 位置：`/admin` 页面，"API 管理"卡片，在 "Skill 管理" 之后
- 前端权限过滤：`admin:menu:api-collections`

### 5.2 API 集合列表页 (`/admin/api-collections`)

- 分页列表（name, description, status badge, api_count, 上传时间）
- 上传按钮 → 弹窗（选文件 + 填写 name/description）
- 点击进入详情
- 操作：编辑 name/description、删除

### 5.3 API 集合详情页 (`/admin/api-collections/:id`)

- 展示解析后的 OpenAPI 文档（路径、方法、参数、响应）
- system_admin 可审批（通过/拒绝按钮）
- 审批后可重新修改状态
- 编辑 name/description

## 6. Skill 工具实现

### 6.1 external_api_search

```
输入: query (string) — 模糊匹配 description 字段
输出: [{name, description, id}] (最多 10 条)
过滤: status == "approved"
```

### 6.2 external_api_summary

```
输入: collection_id (string), page (int, default 1, max 100), page_size (int, default 10, max 10)
输出: {paths: [{path, method, summary}], page, page_size, total}
```

### 6.3 external_api_method

```
输入: collection_id (string), path (string), method (string)
输出: {path, method, summary, description, parameters, request_body, responses}
```

### 6.4 external_api_call

```
输入: collection_id (string), path (string), method (string),
      params (map), body (object), headers (map)
输出: {status, headers, body} (透传外部 API 响应)
```

## 7. 实现步骤

1. ✅ 添加权限常量 + seed
2. ✅ 数据模型 + Repository
3. ✅ OpenAPI 解析 Service
4. ✅ API Collection Handler（CRUD + 审批）
5. ✅ Skill 工具函数注册
6. ✅ 路由注册
7. ✅ 前端页面（列表 + 详情 + 上传）
8. ✅ 前端 admin 页面入口

## 8. 数据库索引

```
api_collections: {user_id: 1, created_at: -1}
api_collections: {status: 1}
```
