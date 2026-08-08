package migration

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

const defaultSystemAdminUserID = "6a64aba51214fe22b2cb917d"

// SeedRBAC inserts built-in roles, permissions, role-permission links, and assigns
// the default system_admin user to the system_admin_role. Idempotent — skips if
// data already exists.
func SeedRBAC(ctx context.Context, db *mongo.Database) error {
	if err := seedRoles(ctx, db); err != nil {
		return err
	}
	if err := seedPermissions(ctx, db); err != nil {
		return err
	}
	if err := seedDefaultUserRole(ctx, db); err != nil {
		return err
	}
	return nil
}

func seedRoles(ctx context.Context, db *mongo.Database) error {
	coll := db.Collection("rbac_roles")
	coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("rbac_role_name_unique"),
	})

	count, err := coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	roles := []interface{}{
		&model.RBACRole{
			ID:          "rbac_role_user",
			Name:        "user_role",
			DisplayName: "普通用户",
			Description: "基本功能的查看和使用权限",
			ParentID:    "",
			Level:       0,
			Type:        model.RBACRoleTypeBuiltin,
			ChildCount:  1, // admin_role
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		&model.RBACRole{
			ID:          "rbac_role_admin",
			Name:        "admin_role",
			DisplayName: "管理员",
			Description: "管理用户、模型、任务、审计等系统功能",
			ParentID:    "rbac_role_user",
			Level:       1,
			Type:        model.RBACRoleTypeBuiltin,
			ChildCount:  1, // system_admin_role
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		&model.RBACRole{
			ID:          "rbac_role_system_admin",
			Name:        "system_admin_role",
			DisplayName: "系统管理员",
			Description: "拥有所有权限的超级管理员角色",
			ParentID:    "rbac_role_admin",
			Level:       2,
			Type:        model.RBACRoleTypeBuiltin,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
	}

	if _, err := coll.InsertMany(ctx, roles); err != nil {
		log.Printf("[rbac-seed] roles insert error: %v", err)
		return err
	}
	log.Println("[rbac-seed] inserted 3 builtin roles")
	return nil
}

func seedPermissions(ctx context.Context, db *mongo.Database) error {
	permColl := db.Collection("rbac_permissions")
	permColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "key", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("rbac_perm_key_unique"),
	})

	count, err := permColl.CountDocuments(ctx, bson.M{})
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	type permSeed struct {
		perm    model.RBACPermission
		roleIDs []string
	}

	admin := "rbac_role_admin"
	user := "rbac_role_user"
	sysAdmin := "rbac_role_system_admin"

	perms := []permSeed{
		// admin_role
		{perm: RBACPerm("rbac_perm_dashboard_view", model.PermDashboardView, "查看仪表盘", "dashboard"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_chat_delete", model.PermChatDelete, "删除对话", "chat"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_agent_view", model.PermAgentView, "查看 Agent 任务", "agent"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_agent_create", model.PermAgentCreate, "创建 Agent 任务", "agent"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_agent_edit", model.PermAgentEdit, "操作 Agent 任务（切换/执行/取消）", "agent"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_agent_delete", model.PermAgentDelete, "删除 Agent 任务", "agent"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_kb_delete", model.PermKBDelete, "删除知识库文档", "knowledge"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_artifact_delete", model.PermArtifactDelete, "删除产出物", "artifact"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_im_view", model.PermIMView, "查看 IM 配置", "im"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_im_edit", model.PermIMEdit, "编辑 IM 配置", "im"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_im_delete", model.PermIMDelete, "删除 IM 配置", "im"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_user_view", model.PermUserView, "查看用户列表", "user"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_user_create", model.PermUserCreate, "创建用户", "user"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_user_edit", model.PermUserEdit, "编辑用户", "user"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_user_delete", model.PermUserDelete, "删除用户", "user"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_model_config_view", model.PermModelConfigView, "查看模型详细配置", "model"), roleIDs: []string{sysAdmin}},
		{perm: RBACPerm("rbac_perm_model_edit", model.PermModelEdit, "编辑模型配置", "model"), roleIDs: []string{sysAdmin}},
		{perm: RBACPerm("rbac_perm_system_view", model.PermSystemView, "查看系统配置", "system"), roleIDs: []string{sysAdmin}},
		{perm: RBACPerm("rbac_perm_system_edit", model.PermSystemEdit, "编辑系统配置", "system"), roleIDs: []string{sysAdmin}},
		{perm: RBACPerm("rbac_perm_audit_view", model.PermAuditView, "查看审计日志", "audit"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_invite_view", model.PermInviteView, "查看邀请", "invite"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_invite_create", model.PermInviteCreate, "创建邀请", "invite"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_skills_view", model.PermSkillsView, "查看技能管理", "skills"), roleIDs: []string{sysAdmin}},
		{perm: RBACPerm("rbac_perm_skills_edit", model.PermSkillsEdit, "编辑技能", "skills"), roleIDs: []string{sysAdmin}},
		{perm: RBACPerm("rbac_perm_stats_view", model.PermStatsView, "查看统计分析", "stats"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_memory_view", model.PermMemoryView, "查看 Memory 列表", "memory"), roleIDs: []string{user}},
		// user_role
		{perm: RBACPerm("rbac_perm_model_view", model.PermModelList, "查看模型配置", "model"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_chat_send", model.PermChatSend, "发送消息", "chat"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_chat_view", model.PermChatView, "查看对话历史", "chat"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_kb_view", model.PermKBView, "查看知识库", "knowledge"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_kb_upload", model.PermKBUpload, "上传知识库文档", "knowledge"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_hermes_view", model.PermHermesView, "Hermes 探索", "hermes"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_artifact_view", model.PermArtifactView, "查看产出物", "artifact"), roleIDs: []string{user}},
		// system_admin_role
		{perm: RBACPerm("rbac_perm_rbac_view", model.PermRBACView, "查看 RBAC 角色与权限", "rbac"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_api_collection_view", model.PermAPICollectionView, "查看 API 集合", "api"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_api_collection_edit", model.PermAPICollectionEdit, "编辑 API 集合", "api"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_api_collection_delete", model.PermAPICollectionDelete, "删除 API 集合", "api"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_api_collection_approve", model.PermAPICollectionApprove, "审批 API 集合", "api"), roleIDs: []string{sysAdmin}},
		{perm: RBACPerm("rbac_perm_rbac_manage", model.PermRBACManage, "RBAC 管理", "rbac"), roleIDs: []string{sysAdmin}},
		// Sidebar menu visibility
		{perm: RBACPerm("rbac_perm_sidebar_dashboard", model.PermSidebarDashboard, "仪表盘菜单", "sidebar"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_sidebar_chat", model.PermSidebarChat, "对话菜单", "sidebar"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_sidebar_hermes", model.PermSidebarHermes, "Hermes菜单", "sidebar"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_sidebar_agent", model.PermSidebarAgent, "Agent菜单", "sidebar"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_sidebar_knowledge", model.PermSidebarKnowledge, "知识库菜单", "sidebar"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_sidebar_artifact", model.PermSidebarArtifact, "产出物菜单", "sidebar"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_sidebar_im", model.PermSidebarIM, "飞书菜单", "sidebar"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_sidebar_memory", model.PermSidebarMemory, "Memory菜单", "sidebar"), roleIDs: []string{user}},
		{perm: RBACPerm("rbac_perm_sidebar_admin", model.PermSidebarAdmin, "管理后台菜单", "sidebar"), roleIDs: []string{admin}},
		// Admin dashboard menu entries
		{perm: RBACPerm("rbac_perm_admin_menu_models", model.PermAdminMenuModels, "模型配置入口", "admin"), roleIDs: []string{sysAdmin}},
		{perm: RBACPerm("rbac_perm_admin_menu_skills", model.PermAdminMenuSkills, "Skill管理入口", "admin"), roleIDs: []string{sysAdmin}},
		{perm: RBACPerm("rbac_perm_admin_menu_users", model.PermAdminMenuUsers, "用户管理入口", "admin"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_admin_menu_rbac", model.PermAdminMenuRBAC, "RBAC管理入口", "admin"), roleIDs: []string{sysAdmin}},
		{perm: RBACPerm("rbac_perm_admin_menu_invites", model.PermAdminMenuInvites, "邀请管理入口", "admin"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_admin_menu_audit", model.PermAdminMenuAudit, "审计日志入口", "admin"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_admin_menu_api_collections", model.PermAdminMenuAPICollections, "API 管理入口", "admin"), roleIDs: []string{admin}},
		{perm: RBACPerm("rbac_perm_admin_menu_settings", model.PermAdminMenuSettings, "系统设置入口", "admin"), roleIDs: []string{sysAdmin}},
	}

	permDocs := make([]interface{}, 0, len(perms))
	rolePermDocs := make([]interface{}, 0)

	for _, ps := range perms {
		permDocs = append(permDocs, &ps.perm)
		for _, roleID := range ps.roleIDs {
			rolePermDocs = append(rolePermDocs, &model.RBACRolePermission{
				ID:           "rbac_rp_" + uuid.New().String()[:8],
				RoleID:       roleID,
				PermissionID: ps.perm.ID,
				CreatedAt:    time.Now(),
			})
		}
	}

	if _, err := permColl.InsertMany(ctx, permDocs); err != nil {
		log.Printf("[rbac-seed] permissions insert error: %v", err)
		return err
	}

	rpColl := db.Collection("rbac_role_permissions")
	rpColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "role_id", Value: 1}, {Key: "permission_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("rp_role_perm_unique"),
	})

	if _, err := rpColl.InsertMany(ctx, rolePermDocs); err != nil {
		log.Printf("[rbac-seed] role-permissions insert error: %v", err)
		return err
	}

	// Update PermissionCount on roles
	permCounts := map[string]int{}
	for _, ps := range perms {
		for _, rid := range ps.roleIDs {
			permCounts[rid]++
		}
	}
	for rid, cnt := range permCounts {
		db.Collection("rbac_roles").UpdateOne(ctx,
			bson.M{"_id": rid},
			bson.M{"$set": bson.M{"permission_count": cnt}},
		)
	}

	log.Printf("[rbac-seed] inserted %d permissions, %d role-permission links", len(permDocs), len(rolePermDocs))
	return nil
}

func seedDefaultUserRole(ctx context.Context, db *mongo.Database) error {
	coll := db.Collection("user_rbac_roles")
	coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "role_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("ur_user_role_unique"),
	})

	count, err := coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	_, err = coll.InsertOne(ctx, &model.UserRBACRole{
		ID:        "rbac_ur_" + uuid.New().String()[:8],
		UserID:    defaultSystemAdminUserID,
		RoleID:    "rbac_role_system_admin",
		CreatedAt: time.Now(),
	})
	if err != nil {
		log.Printf("[rbac-seed] default user-role insert error: %v", err)
		return err
	}

	db.Collection("users").UpdateOne(ctx,
		bson.M{"_id": defaultSystemAdminUserID},
		bson.M{"$set": bson.M{"rbac_role_count": 1}},
	)

	log.Println("[rbac-seed] assigned system_admin_role to default system_admin user")
	return nil
}

func RBACPerm(id, key, name, module string) model.RBACPermission {
	return model.RBACPermission{
		ID:          id,
		Key:         key,
		Name:        name,
		Module:      module,
		Type:        model.RBACPermTypeBuiltin,
		CreatedAt:   time.Now(),
	}
}
