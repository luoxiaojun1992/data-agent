package audit

// actionDescriptions maps an audit log's raw `Action` value (written by
// middleware/audit.go as `"METHOD FullPath模板"`, e.g. `"POST /api/v1/chat"`)
// to a human-friendly Chinese description for the list view and CSV export.
//
// The key must exactly match gin's `c.FullPath()` template (path params are
// already generic `:id` / `:name` and never carry query strings). This map is
// used ONLY for display enrichment — the `path` search filter matches the raw
// `action` field directly (D4), never these descriptions.
//
// List compiled from internal/api/handler/routes.go and the per-feature route
// registration files (user/chat/session/voice/artifact/knowledge/audit/
// notification/task/feishu/imbind/modelconfig/config/skill_config/rbac/
// api_collection) — every non-GET/HEAD/OPTIONS route is covered.
var actionDescriptions = map[string]string{
	// ── Auth ────────────────────────────────────────────────
	"POST /api/v1/auth/login":                 "登录",
	"POST /api/v1/auth/complete-registration": "完成注册",
	"POST /api/v1/auth/refresh":               "刷新令牌",
	"POST /api/v1/auth/change-password":       "修改密码",

	// ── Chat ────────────────────────────────────────────────
	"POST /api/v1/chat":                                             "Chat 对话",
	"POST /api/v1/chat/enhance":                                     "内容增强",
	"POST /api/v1/chat/redact":                                      "输入脱敏",
	"POST /api/v1/chat/:session_id/human-channel/:request_id/reply": "人工信道回复",

	// ── Session ─────────────────────────────────────────────
	"PUT /api/v1/sessions/:id":          "会话续期",
	"DELETE /api/v1/sessions/:id":       "删除会话",
	"DELETE /api/v1/sessions/:id/history": "清空会话历史",
	"POST /api/v1/sessions/:id/restore": "恢复会话",

	// ── Voice ───────────────────────────────────────────────
	"POST /api/v1/voice/start":  "语音上传开始",
	"POST /api/v1/voice/chunk":  "语音分片上传",
	"POST /api/v1/voice/finish": "语音上传完成",

	// ── Artifact ────────────────────────────────────────────
	"POST /api/v1/artifacts/upload":  "上传产出物",
	"DELETE /api/v1/artifacts/:id":   "删除产出物",

	// ── Knowledge ───────────────────────────────────────────
	"POST /api/v1/knowledge/docs":             "上传知识文档",
	"POST /api/v1/knowledge/import-url":       "URL 导入文档",
	"PUT /api/v1/knowledge/docs/:id/public":   "切换文档公开",
	"DELETE /api/v1/knowledge/docs/:id":       "删除知识文档",

	// ── Audit ───────────────────────────────────────────────
	"POST /api/v1/admin/audit/export": "导出审计日志",

	// ── Notification ────────────────────────────────────────
	"POST /api/v1/notifications":            "发送通知",
	"POST /api/v1/notifications/broadcast":  "广播通知",
	"PUT /api/v1/notifications/:id/read":    "标记通知已读",
	"PUT /api/v1/notifications/read-all":    "全部通知已读",

	// ── Task ────────────────────────────────────────────────
	"POST /api/v1/tasks":                    "创建任务",
	"POST /api/v1/tasks/:task_id/run":       "创建任务运行",
	"DELETE /api/v1/tasks/:task_id":         "删除任务",
	"PATCH /api/v1/tasks/:task_id/enabled":  "启停任务",

	// ── Task run ────────────────────────────────────────────
	"PUT /api/v1/task-runs/:run_id/cancel": "取消任务运行",

	// ── Feishu ──────────────────────────────────────────────
	"POST /api/v1/im/feishu/configs":     "创建飞书配置",
	"PUT /api/v1/im/feishu/configs/:id":  "更新飞书配置",
	"DELETE /api/v1/im/feishu/configs/:id": "删除飞书配置",
	"POST /api/v1/im/feishu/webhook":     "飞书 webhook 回调",

	// ── IM bind ─────────────────────────────────────────────
	"PUT /api/v1/im/bind":    "更新 IM 绑定",
	"DELETE /api/v1/im/bind": "解绑 IM",

	// ── Admin: invites ──────────────────────────────────────
	"POST /api/v1/admin/invites":            "创建邀请",
	"DELETE /api/v1/admin/invites/:id":      "撤销邀请",
	"PUT /api/v1/admin/invites/hmac-secret": "更新 HMAC 密钥",

	// ── Admin: sysconfig ────────────────────────────────────
	"PUT /api/v1/admin/sysconfig/system": "更新系统配置",

	// ── Admin: models ───────────────────────────────────────
	"POST /api/v1/admin/models":              "新增模型",
	"PUT /api/v1/admin/models":               "更新模型配置",
	"PATCH /api/v1/admin/models/:id":         "更新模型",
	"PATCH /api/v1/admin/models/:id/default": "设为默认模型",
	"DELETE /api/v1/admin/models/:id":        "删除模型",
	"DELETE /api/v1/admin/models/default":    "取消默认模型",

	// ── Admin: skills ───────────────────────────────────────
	"PUT /api/v1/admin/skills/:name": "更新技能配置",

	// ── Admin: api-collections ──────────────────────────────
	"POST /api/v1/admin/api-collections":            "创建 API 集合",
	"PUT /api/v1/admin/api-collections/:id":         "更新 API 集合",
	"DELETE /api/v1/admin/api-collections/:id":      "删除 API 集合",
	"POST /api/v1/admin/api-collections/:id/approve": "审批 API 集合",

	// ── Admin: rbac ────────────────────────────────────────
	"POST /api/v1/admin/rbac/roles":                     "创建角色",
	"PUT /api/v1/admin/rbac/roles/:id":                  "更新角色",
	"DELETE /api/v1/admin/rbac/roles/:id":               "删除角色",
	"POST /api/v1/admin/rbac/permissions":               "创建权限",
	"DELETE /api/v1/admin/rbac/permissions/:id":         "删除权限",
	"POST /api/v1/admin/rbac/roles/:id/permissions":     "角色添加权限",
	"DELETE /api/v1/admin/rbac/roles/:id/permissions/:permId": "角色移除权限",

	// ── Admin: user-rbac association ───────────────────────
	"POST /api/v1/admin/users/:userId/rbac-roles":    "用户添加角色",
	"DELETE /api/v1/admin/users/:userId/rbac-roles/:id": "用户移除角色",

	// ── Users ──────────────────────────────────────────────
	"POST /api/v1/users":             "创建用户",
	"PUT /api/v1/users/:id":          "编辑用户",
	"PUT /api/v1/users/:id/role":     "编辑用户角色",
	"PATCH /api/v1/users/:id/status": "切换用户状态",
	"DELETE /api/v1/users/:id":       "删除用户",
}

// describeAction returns the human-friendly description for an audit action,
// falling back to the raw action string when no mapping entry exists.
func describeAction(action string) string {
	if desc, ok := actionDescriptions[action]; ok {
		return desc
	}
	return action
}
