package mongo

import (
	"fmt"
	"reflect"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/apireview"
	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// NewDomainID generates a new domain entity ID as a 24-char hex string.
// It is used by infra repositories when creating new documents, so that
// ID generation stays inside the infra layer and is invisible to callers
// (domain/service/handler never import mongo-driver).
func NewDomainID() string {
	return primitive.NewObjectID().Hex()
}

// ObjectIDFromDomainID converts a domain string ID to a mongo ObjectID.
// Returns an error if id is empty or not a valid ObjectID hex.
func ObjectIDFromDomainID(id string) (primitive.ObjectID, error) {
	if id == "" {
		return primitive.NilObjectID, fmt.Errorf("empty id")
	}
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return primitive.NilObjectID, fmt.Errorf("invalid object id %q: %w", id, err)
	}
	return oid, nil
}

// DomainIDFromObjectID converts a mongo ObjectID to a domain string ID.
func DomainIDFromObjectID(oid primitive.ObjectID) string {
	return oid.Hex()
}

// ── Generic helpers for reading bson.M ──────────────────────────────

func getStr(d bson.M, key string) string {
	v, ok := d[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	// Handle named string types (e.g., model.UserRole, task.Status,
	// knowledge.DocStatus) — a direct v.(string) assertion fails because
	// the dynamic type is the named type, not string. Reflection on the
	// underlying kind recovers the string value.
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.String {
		return rv.String()
	}
	return ""
}

func getTime(d bson.M, key string) time.Time {
	v, ok := d[key]
	if !ok {
		return time.Time{}
	}
	switch t := v.(type) {
	case time.Time:
		return t
	case primitive.DateTime:
		return t.Time().UTC()
	}
	return time.Time{}
}

func getTimePtr(d bson.M, key string) *time.Time {
	v, ok := d[key]
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case time.Time:
		if t.IsZero() {
			return nil
		}
		tt := t
		return &tt
	case primitive.DateTime:
		tt := t.Time().UTC()
		if tt.IsZero() {
			return nil
		}
		return &tt
	}
	return nil
}

func getInt(d bson.M, key string) int {
	v, ok := d[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	}
	return 0
}

func getInt64(d bson.M, key string) int64 {
	v, ok := d[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case int32:
		return int64(n)
	}
	return 0
}

func getBool(d bson.M, key string) bool {
	v, ok := d[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

func getStrSlice(d bson.M, key string) []string {
	v, ok := d[key]
	if !ok {
		return nil
	}
	switch s := v.(type) {
	case []string:
		return s
	case []interface{}:
		out := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	case primitive.A:
		out := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func getMap(d bson.M, key string) map[string]interface{} {
	v, ok := d[key]
	if !ok {
		return nil
	}
	switch m := v.(type) {
	case map[string]interface{}:
		return m
	case bson.M:
		out := make(map[string]interface{}, len(m))
		for k, val := range m {
			out[k] = val
		}
		return out
	}
	return nil
}

// getSubDoc returns a sub-document as bson.M (used for embedded structs
// like TaskProgress). When mongo-driver decodes into a bson.M target,
// sub-documents are typed as bson.M.
func getSubDoc(d bson.M, key string) bson.M {
	v, ok := d[key]
	if !ok {
		return bson.M{}
	}
	switch m := v.(type) {
	case bson.M:
		return m
	case map[string]interface{}:
		out := bson.M{}
		for k, val := range m {
			out[k] = val
		}
		return out
	}
	return bson.M{}
}

// ── User ────────────────────────────────────────────────────────────

func userToDoc(u *model.User) bson.M {
	doc := bson.M{
		"_id":              u.ID,
		"username":         u.Username,
		"password_hash":    u.PasswordHash,
		"role":             u.Role,
		"status":           u.Status,
		"password_changed": u.PasswordChanged,
		"created_at":       u.CreatedAt,
		"updated_at":       u.UpdatedAt,
	}
	if u.DisplayName != "" {
		doc["display_name"] = u.DisplayName
	}
	if u.InvitedBy != "" {
		doc["invited_by"] = u.InvitedBy
	}
	if u.InviteID != "" {
		doc["invite_id"] = u.InviteID
	}
	if u.FeishuAppID != "" {
		doc["feishu_app_id"] = u.FeishuAppID
	}
	if u.FeishuAppSecret != "" {
		doc["feishu_app_secret"] = u.FeishuAppSecret
	}
	return doc
}

func docToUser(d bson.M) *model.User {
	return &model.User{
		ID:              getStr(d, "_id"),
		Username:        getStr(d, "username"),
		PasswordHash:    getStr(d, "password_hash"),
		Role:            model.UserRole(getStr(d, "role")),
		Status:          model.UserStatus(getStr(d, "status")),
		PasswordChanged: getBool(d, "password_changed"),
		DisplayName:     getStr(d, "display_name"),
		InvitedBy:       getStr(d, "invited_by"),
		InviteID:        getStr(d, "invite_id"),
		FeishuAppID:     getStr(d, "feishu_app_id"),
		FeishuAppSecret: getStr(d, "feishu_app_secret"),
		CreatedAt:       getTime(d, "created_at"),
		UpdatedAt:       getTime(d, "updated_at"),
	}
}

// ── Invite ──────────────────────────────────────────────────────────

func inviteToDoc(i *model.Invite) bson.M {
	doc := bson.M{
		"_id":        i.ID,
		"invite_id":  i.InviteID,
		"role":       i.Role,
		"status":     i.Status,
		"token_hash": i.TokenHash,
		"created_by": i.CreatedBy,
		"created_at": i.CreatedAt,
		"expires_at": i.ExpiresAt,
	}
	if i.Email != "" {
		doc["email"] = i.Email
	}
	if i.AcceptedAt != nil {
		doc["accepted_at"] = *i.AcceptedAt
	}
	if i.AcceptedBy != "" {
		doc["accepted_by"] = i.AcceptedBy
	}
	return doc
}

func docToInvite(d bson.M) *model.Invite {
	return &model.Invite{
		ID:         getStr(d, "_id"),
		InviteID:   getStr(d, "invite_id"),
		Email:      getStr(d, "email"),
		Role:       getStr(d, "role"),
		Status:     model.InviteStatus(getStr(d, "status")),
		TokenHash:  getStr(d, "token_hash"),
		CreatedBy:  getStr(d, "created_by"),
		CreatedAt:  getTime(d, "created_at"),
		ExpiresAt:  getTime(d, "expires_at"),
		AcceptedAt: getTimePtr(d, "accepted_at"),
		AcceptedBy: getStr(d, "accepted_by"),
	}
}

// ── Role ────────────────────────────────────────────────────────────

func roleToDoc(r *model.Role) bson.M {
	return bson.M{
		"_id":         r.ID,
		"name":        r.Name,
		"display_name": r.DisplayName,
		"description": r.Description,
		"permissions": r.Permissions,
		"type":        r.Type,
		"created_at":  r.CreatedAt,
		"updated_at":  r.UpdatedAt,
	}
}

func docToRole(d bson.M) *model.Role {
	return &model.Role{
		ID:          getStr(d, "_id"),
		Name:        getStr(d, "name"),
		DisplayName: getStr(d, "display_name"),
		Description: getStr(d, "description"),
		Permissions: getStrSlice(d, "permissions"),
		Type:        getStr(d, "type"),
		CreatedAt:   getTime(d, "created_at"),
		UpdatedAt:   getTime(d, "updated_at"),
	}
}

// ── AuditLog ────────────────────────────────────────────────────────

func auditLogToDoc(a *model.AuditLog) bson.M {
	return bson.M{
		"_id":         a.ID,
		"action":      a.Action,
		"user_id":     a.UserID,
		"resource":    a.Resource,
		"details":     a.Details,
		"ip":          a.IP,
		"user_agent":  a.UserAgent,
		"status_code": a.StatusCode,
		"created_at":  a.CreatedAt,
	}
}

func docToAuditLog(d bson.M) *model.AuditLog {
	return &model.AuditLog{
		ID:         getStr(d, "_id"),
		Action:     getStr(d, "action"),
		UserID:     getStr(d, "user_id"),
		Resource:   getStr(d, "resource"),
		Details:    getStr(d, "details"),
		IP:         getStr(d, "ip"),
		UserAgent:  getStr(d, "user_agent"),
		StatusCode: getInt(d, "status_code"),
		CreatedAt:  getTime(d, "created_at"),
	}
}

// ── Notification ────────────────────────────────────────────────────

func notificationToDoc(n *model.Notification) bson.M {
	doc := bson.M{
		"_id":        n.ID,
		"title":      n.Title,
		"content":    n.Content,
		"type":       n.Type,
		"target_all": n.TargetAll,
		"created_at": n.CreatedAt,
	}
	if len(n.TargetIDs) > 0 {
		doc["target_ids"] = n.TargetIDs
	}
	if len(n.ReadBy) > 0 {
		doc["read_by"] = n.ReadBy
	}
	return doc
}

func docToNotification(d bson.M) *model.Notification {
	return &model.Notification{
		ID:        getStr(d, "_id"),
		Title:     getStr(d, "title"),
		Content:   getStr(d, "content"),
		Type:      getStr(d, "type"),
		TargetAll: getBool(d, "target_all"),
		TargetIDs: getStrSlice(d, "target_ids"),
		ReadBy:    getStrSlice(d, "read_by"),
		CreatedAt: getTime(d, "created_at"),
	}
}

// ── SystemConfig ────────────────────────────────────────────────────

func systemConfigToDoc(c *model.SystemConfig) bson.M {
	return bson.M{
		"_id":        c.ID,
		"namespace":  c.Namespace,
		"key":        c.Key,
		"value":      c.Value,
		"updated_at": c.UpdatedAt,
	}
}

func docToSystemConfig(d bson.M) *model.SystemConfig {
	return &model.SystemConfig{
		ID:        getStr(d, "_id"),
		Namespace: getStr(d, "namespace"),
		Key:       getStr(d, "key"),
		Value:     getStr(d, "value"),
		UpdatedAt: getTime(d, "updated_at"),
	}
}

// ── Task + TaskProgress ─────────────────────────────────────────────

func taskProgressToDoc(p task.TaskProgress) bson.M {
	return bson.M{
		"current_step": p.CurrentStep,
		"total_steps":  p.TotalSteps,
		"message":      p.Message,
		"percent":      p.Percent,
	}
}

func docToTaskProgress(d bson.M) task.TaskProgress {
	return task.TaskProgress{
		CurrentStep: getInt(d, "current_step"),
		TotalSteps:  getInt(d, "total_steps"),
		Message:     getStr(d, "message"),
		Percent:     getInt(d, "percent"),
	}
}

func taskToDoc(t *task.Task) bson.M {
	doc := bson.M{
		"_id":          t.ID,
		"session_id":   t.SessionID,
		"user_id":      t.UserID,
		"type":         t.Type,
		"model_id":     t.ModelID,
		"status":       t.Status,
		"skill_chain":  t.SkillChain,
		"params":       t.Params,
		"progress":     taskProgressToDoc(t.Progress),
		"retry_count":  t.RetryCount,
		"max_retries":  t.MaxRetries,
		"created_at":   t.CreatedAt,
		"updated_at":   t.UpdatedAt,
		"duration_ms":  t.DurationMs,
	}
	if t.Result != nil {
		doc["result"] = t.Result
	}
	if t.Error != "" {
		doc["error"] = t.Error
	}
	if t.CompletedAt != nil {
		doc["completed_at"] = *t.CompletedAt
	}
	return doc
}

func docToTask(d bson.M) *task.Task {
	return &task.Task{
		ID:          getStr(d, "_id"),
		SessionID:   getStr(d, "session_id"),
		UserID:      getStr(d, "user_id"),
		Type:        getStr(d, "type"),
		ModelID:     getStr(d, "model_id"),
		Status:      task.Status(getStr(d, "status")),
		SkillChain:  getStrSlice(d, "skill_chain"),
		Params:      getMap(d, "params"),
		Result:      getMap(d, "result"),
		Error:       getStr(d, "error"),
		Progress:    docToTaskProgress(getSubDoc(d, "progress")),
		RetryCount:  getInt(d, "retry_count"),
		MaxRetries:  getInt(d, "max_retries"),
		CreatedAt:   getTime(d, "created_at"),
		UpdatedAt:   getTime(d, "updated_at"),
		CompletedAt: getTimePtr(d, "completed_at"),
		DurationMs:  getInt64(d, "duration_ms"),
	}
}

// ── Artifact ────────────────────────────────────────────────────────

func artifactToDoc(a *artifact.Artifact) bson.M {
	doc := bson.M{
		"_id":          a.ID,
		"user_id":      a.UserID,
		"session_id":   a.SessionID,
		"name":         a.Name,
		"mime_type":    a.MimeType,
		"size_bytes":   a.SizeBytes,
		"storage_path": a.StoragePath,
		"persistent":   a.Persistent,
		"created_at":   a.CreatedAt,
		"updated_at":   a.UpdatedAt,
	}
	if a.TaskID != "" {
		doc["task_id"] = a.TaskID
	}
	return doc
}

func docToArtifact(d bson.M) *artifact.Artifact {
	return &artifact.Artifact{
		ID:          getStr(d, "_id"),
		UserID:      getStr(d, "user_id"),
		SessionID:   getStr(d, "session_id"),
		TaskID:      getStr(d, "task_id"),
		Name:        getStr(d, "name"),
		MimeType:    getStr(d, "mime_type"),
		SizeBytes:   getInt64(d, "size_bytes"),
		StoragePath: getStr(d, "storage_path"),
		Persistent:  getBool(d, "persistent"),
		CreatedAt:   getTime(d, "created_at"),
		UpdatedAt:   getTime(d, "updated_at"),
	}
}

// ── KnowledgeDoc ────────────────────────────────────────────────────

func knowledgeDocToDoc(k *knowledge.KnowledgeDoc) bson.M {
	doc := bson.M{
		"_id":         k.ID,
		"user_id":     k.UserID,
		"title":       k.Title,
		"file_name":   k.FileName,
		"file_type":   k.FileType,
		"size_bytes":  k.SizeBytes,
		"status":      k.Status,
		"chunk_count": k.ChunkCount,
		"created_at":  k.CreatedAt,
		"updated_at":  k.UpdatedAt,
	}
	if k.GridFSFileID != "" {
		doc["gridfs_file_id"] = k.GridFSFileID
	}
	return doc
}

func docToKnowledgeDoc(d bson.M) *knowledge.KnowledgeDoc {
	return &knowledge.KnowledgeDoc{
		ID:           getStr(d, "_id"),
		UserID:       getStr(d, "user_id"),
		Title:        getStr(d, "title"),
		FileName:     getStr(d, "file_name"),
		FileType:     getStr(d, "file_type"),
		SizeBytes:    getInt64(d, "size_bytes"),
		Status:       knowledge.DocStatus(getStr(d, "status")),
		ChunkCount:   getInt(d, "chunk_count"),
		GridFSFileID: getStr(d, "gridfs_file_id"),
		CreatedAt:    getTime(d, "created_at"),
		UpdatedAt:    getTime(d, "updated_at"),
	}
}

// ── Chunk ───────────────────────────────────────────────────────────

func chunkToDoc(c *knowledge.Chunk) bson.M {
	doc := bson.M{
		"_id":        c.ID,
		"doc_id":     c.DocID,
		"content":    c.Content,
		"chunk_idx":  c.ChunkIdx,
		"char_count": c.CharCount,
		"created_at": c.CreatedAt,
	}
	if c.MilvusID != 0 {
		doc["milvus_id"] = c.MilvusID
	}
	return doc
}

func docToChunk(d bson.M) *knowledge.Chunk {
	return &knowledge.Chunk{
		ID:        getStr(d, "_id"),
		DocID:     getStr(d, "doc_id"),
		Content:   getStr(d, "content"),
		ChunkIdx:  getInt(d, "chunk_idx"),
		CharCount: getInt(d, "char_count"),
		MilvusID:  getInt64(d, "milvus_id"),
		CreatedAt: getTime(d, "created_at"),
	}
}

// ── APIReview ───────────────────────────────────────────────────────

func apiReviewToDoc(r *apireview.APIReview) bson.M {
	doc := bson.M{
		"_id":        r.ID,
		"name":       r.Name,
		"file_name":  r.FileName,
		"version":    r.Version,
		"endpoints":  r.Endpoints,
		"domain":     r.Domain,
		"rate_limit": r.RateLimit,
		"submitter":  r.Submitter,
		"status":     r.Status,
		"created_at": r.CreatedAt,
		"updated_at": r.UpdatedAt,
	}
	if r.Reviewer != "" {
		doc["reviewer"] = r.Reviewer
	}
	if r.RejectReason != "" {
		doc["reject_reason"] = r.RejectReason
	}
	if r.ReviewedAt != nil {
		doc["reviewed_at"] = *r.ReviewedAt
	}
	return doc
}

func docToAPIReview(d bson.M) *apireview.APIReview {
	return &apireview.APIReview{
		ID:           getStr(d, "_id"),
		Name:         getStr(d, "name"),
		FileName:     getStr(d, "file_name"),
		Version:      getStr(d, "version"),
		Endpoints:    getInt(d, "endpoints"),
		Domain:       getStr(d, "domain"),
		RateLimit:    getInt(d, "rate_limit"),
		Submitter:    getStr(d, "submitter"),
		Reviewer:     getStr(d, "reviewer"),
		RejectReason: getStr(d, "reject_reason"),
		Status:       apireview.Status(getStr(d, "status")),
		ReviewedAt:   getTimePtr(d, "reviewed_at"),
		CreatedAt:    getTime(d, "created_at"),
		UpdatedAt:    getTime(d, "updated_at"),
	}
}
