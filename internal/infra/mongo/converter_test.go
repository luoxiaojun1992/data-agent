package mongo

import (
	"testing"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/apireview"
	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── ID helpers (existing SPEC-056 tests) ────────────────────────────

func TestNewDomainID(t *testing.T) {
	id := NewDomainID()
	if id == "" {
		t.Fatal("NewDomainID should not return empty string")
	}
	if len(id) != 24 {
		t.Errorf("NewDomainID length: got %d, want 24 (ObjectID hex)", len(id))
	}
	if _, err := primitive.ObjectIDFromHex(id); err != nil {
		t.Errorf("NewDomainID %q is not a valid ObjectID hex: %v", id, err)
	}
	id2 := NewDomainID()
	if id == id2 {
		t.Error("NewDomainID should produce unique IDs")
	}
}

func TestObjectIDFromDomainID(t *testing.T) {
	t.Run("valid hex", func(t *testing.T) {
		orig := primitive.NewObjectID()
		got, err := ObjectIDFromDomainID(orig.Hex())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != orig {
			t.Errorf("ObjectIDFromDomainID roundtrip: got %v, want %v", got, orig)
		}
	})
	t.Run("empty id returns error", func(t *testing.T) {
		_, err := ObjectIDFromDomainID("")
		if err == nil {
			t.Fatal("expected error for empty id")
		}
	})
	t.Run("invalid hex returns error", func(t *testing.T) {
		_, err := ObjectIDFromDomainID("not-a-hex")
		if err == nil {
			t.Fatal("expected error for invalid hex")
		}
	})
}

func TestDomainIDFromObjectID(t *testing.T) {
	orig := primitive.NewObjectID()
	got := DomainIDFromObjectID(orig)
	if got != orig.Hex() {
		t.Errorf("DomainIDFromObjectID: got %q, want %q", got, orig.Hex())
	}
	nilHex := DomainIDFromObjectID(primitive.NilObjectID)
	if nilHex != "000000000000000000000000" {
		t.Errorf("DomainIDFromObjectID(NilObjectID): got %q, want zero hex", nilHex)
	}
}

// ── Helper function tests ───────────────────────────────────────────

func TestGetStr(t *testing.T) {
	d := bson.M{"key": "value"}
	if got := getStr(d, "key"); got != "value" {
		t.Errorf("getStr: got %q, want %q", got, "value")
	}
	if got := getStr(d, "missing"); got != "" {
		t.Errorf("getStr missing key: got %q, want empty", got)
	}
	if got := getStr(bson.M{"key": 123}, "key"); got != "" {
		t.Errorf("getStr non-string: got %q, want empty", got)
	}
}

func TestGetTime(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	d := bson.M{"ts": now}
	if got := getTime(d, "ts"); !got.Equal(now) {
		t.Errorf("getTime: got %v, want %v", got, now)
	}
	if got := getTime(d, "missing"); !got.IsZero() {
		t.Errorf("getTime missing: got %v, want zero", got)
	}
	dt := primitive.NewDateTimeFromTime(now)
	d2 := bson.M{"ts": dt}
	if got := getTime(d2, "ts"); !got.Equal(now) {
		t.Errorf("getTime DateTime: got %v, want %v", got, now)
	}
}

func TestGetTimePtr(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	d := bson.M{"ts": now}
	got := getTimePtr(d, "ts")
	if got == nil {
		t.Fatal("getTimePtr: got nil, want non-nil")
	}
	if !got.Equal(now) {
		t.Errorf("getTimePtr: got %v, want %v", *got, now)
	}
	if p := getTimePtr(d, "missing"); p != nil {
		t.Errorf("getTimePtr missing: got %v, want nil", p)
	}
	if p := getTimePtr(bson.M{"ts": time.Time{}}, "ts"); p != nil {
		t.Errorf("getTimePtr zero time: got %v, want nil", p)
	}
}

func TestGetInt(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
		want int
	}{
		{"int", 42, 42},
		{"int32", int32(42), 42},
		{"int64", int64(42), 42},
		{"missing", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := bson.M{}
			if tt.val != nil {
				d["n"] = tt.val
			}
			if got := getInt(d, "n"); got != tt.want {
				t.Errorf("getInt: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGetInt64(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
		want int64
	}{
		{"int64", int64(999), 999},
		{"int", 999, 999},
		{"int32", int32(999), 999},
		{"missing", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := bson.M{}
			if tt.val != nil {
				d["n"] = tt.val
			}
			if got := getInt64(d, "n"); got != tt.want {
				t.Errorf("getInt64: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGetBool(t *testing.T) {
	d := bson.M{"flag": true}
	if !getBool(d, "flag") {
		t.Error("getBool: got false, want true")
	}
	if getBool(d, "missing") {
		t.Error("getBool missing: got true, want false")
	}
}

func TestGetStrSlice(t *testing.T) {
	t.Run("[]string", func(t *testing.T) {
		d := bson.M{"items": []string{"a", "b"}}
		got := getStrSlice(d, "items")
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Errorf("getStrSlice []string: got %v", got)
		}
	})
	t.Run("[]interface{}", func(t *testing.T) {
		d := bson.M{"items": []interface{}{"a", "b"}}
		got := getStrSlice(d, "items")
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Errorf("getStrSlice []interface{}: got %v", got)
		}
	})
	t.Run("primitive.A", func(t *testing.T) {
		d := bson.M{"items": primitive.A{"a", "b"}}
		got := getStrSlice(d, "items")
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Errorf("getStrSlice primitive.A: got %v", got)
		}
	})
	t.Run("missing", func(t *testing.T) {
		if got := getStrSlice(bson.M{}, "items"); got != nil {
			t.Errorf("getStrSlice missing: got %v, want nil", got)
		}
	})
}

func TestGetMap(t *testing.T) {
	t.Run("map[string]interface{}", func(t *testing.T) {
		d := bson.M{"m": map[string]interface{}{"k": "v"}}
		got := getMap(d, "m")
		if got["k"] != "v" {
			t.Errorf("getMap: got %v", got)
		}
	})
	t.Run("bson.M", func(t *testing.T) {
		d := bson.M{"m": bson.M{"k": "v"}}
		got := getMap(d, "m")
		if got["k"] != "v" {
			t.Errorf("getMap bson.M: got %v", got)
		}
	})
	t.Run("missing", func(t *testing.T) {
		if got := getMap(bson.M{}, "m"); got != nil {
			t.Errorf("getMap missing: got %v, want nil", got)
		}
	})
}

func TestGetSubDoc(t *testing.T) {
	t.Run("bson.M", func(t *testing.T) {
		d := bson.M{"sub": bson.M{"k": "v"}}
		got := getSubDoc(d, "sub")
		if got["k"] != "v" {
			t.Errorf("getSubDoc: got %v", got)
		}
	})
	t.Run("map[string]interface{}", func(t *testing.T) {
		d := bson.M{"sub": map[string]interface{}{"k": "v"}}
		got := getSubDoc(d, "sub")
		if got["k"] != "v" {
			t.Errorf("getSubDoc map: got %v", got)
		}
	})
	t.Run("missing returns empty bson.M", func(t *testing.T) {
		got := getSubDoc(bson.M{}, "sub")
		if len(got) != 0 {
			t.Errorf("getSubDoc missing: got %v, want empty", got)
		}
	})
}

// ── Converter roundtrip tests ───────────────────────────────────────
// Each test constructs a fully-populated domain struct, converts to bson.M,
// converts back, and verifies all fields match.

func TestUserRoundtrip(t *testing.T) {
	acceptedAt := time.Now().UTC().Truncate(time.Millisecond)
	orig := &model.User{
		ID:              "user123",
		Username:        "alice",
		PasswordHash:    "$2a$10$hash",
		Role:            model.RoleAdmin,
		Status:          model.StatusEnabled,
		PasswordChanged: true,
		DisplayName:     "Alice",
		InvitedBy:       "admin1",
		InviteID:        "inv123",
		FeishuAppID:     "fs_app",
		FeishuAppSecret: "fs_secret",
		CreatedAt:       acceptedAt,
		UpdatedAt:       acceptedAt,
	}
	doc := userToDoc(orig)
	// Verify omitempty fields are present when non-zero
	if _, ok := doc["display_name"]; !ok {
		t.Error("display_name should be present when non-empty")
	}
	got := docToUser(doc)
	if got.ID != orig.ID || got.Username != orig.Username || got.PasswordHash != orig.PasswordHash {
		t.Errorf("ID/Username/PasswordHash mismatch: got %+v", got)
	}
	if got.Role != orig.Role || got.Status != orig.Status || got.PasswordChanged != orig.PasswordChanged {
		t.Errorf("Role/Status/PasswordChanged mismatch: got %+v", got)
	}
	if got.DisplayName != orig.DisplayName || got.InvitedBy != orig.InvitedBy || got.InviteID != orig.InviteID {
		t.Errorf("optional string fields mismatch: got %+v", got)
	}
	if got.FeishuAppID != orig.FeishuAppID || got.FeishuAppSecret != orig.FeishuAppSecret {
		t.Errorf("feishu fields mismatch: got %+v", got)
	}
	if !got.CreatedAt.Equal(orig.CreatedAt) || !got.UpdatedAt.Equal(orig.UpdatedAt) {
		t.Errorf("timestamps mismatch: got %v/%v", got.CreatedAt, got.UpdatedAt)
	}
}

func TestUserRoundtripEmptyOptionals(t *testing.T) {
	orig := &model.User{
		ID:           "user456",
		Username:     "bob",
		PasswordHash: "hash",
		Role:         model.RoleUser,
		Status:       model.StatusDisabled,
	}
	doc := userToDoc(orig)
	// omitempty fields must be absent when zero
	for _, key := range []string{"display_name", "invited_by", "invite_id", "feishu_app_id", "feishu_app_secret"} {
		if _, ok := doc[key]; ok {
			t.Errorf("%s should be absent when empty", key)
		}
	}
	got := docToUser(doc)
	if got.DisplayName != "" || got.InvitedBy != "" {
		t.Errorf("empty optionals should read as empty: got DisplayName=%q InvitedBy=%q", got.DisplayName, got.InvitedBy)
	}
}

func TestInviteRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	acceptedAt := now.Add(time.Hour)
	orig := &model.Invite{
		ID:         "inv1",
		InviteID:   "invite_token_abc",
		Email:      "user@example.com",
		Role:       "admin",
		Status:     model.InviteStatusAccepted,
		TokenHash:  "hash123",
		CreatedBy:  "admin1",
		CreatedAt:  now,
		ExpiresAt:  now.Add(24 * time.Hour),
		AcceptedAt: &acceptedAt,
		AcceptedBy: "user1",
	}
	doc := inviteToDoc(orig)
	got := docToInvite(doc)
	if got.ID != orig.ID || got.InviteID != orig.InviteID || got.Email != orig.Email {
		t.Errorf("basic fields mismatch: got %+v", got)
	}
	if got.Role != orig.Role || got.Status != orig.Status || got.TokenHash != orig.TokenHash {
		t.Errorf("role/status/token mismatch: got %+v", got)
	}
	if got.CreatedBy != orig.CreatedBy {
		t.Errorf("CreatedBy mismatch: got %q", got.CreatedBy)
	}
	if !got.CreatedAt.Equal(orig.CreatedAt) || !got.ExpiresAt.Equal(orig.ExpiresAt) {
		t.Errorf("timestamps mismatch")
	}
	if got.AcceptedAt == nil || !got.AcceptedAt.Equal(*orig.AcceptedAt) {
		t.Errorf("AcceptedAt mismatch: got %v", got.AcceptedAt)
	}
	if got.AcceptedBy != orig.AcceptedBy {
		t.Errorf("AcceptedBy mismatch: got %q", got.AcceptedBy)
	}
}

func TestInviteRoundtripNilAcceptedAt(t *testing.T) {
	orig := &model.Invite{
		ID:       "inv2",
		InviteID: "pending_invite",
		Role:     "user",
		Status:   model.InviteStatusPending,
	}
	doc := inviteToDoc(orig)
	if _, ok := doc["accepted_at"]; ok {
		t.Error("accepted_at should be absent when nil")
	}
	if _, ok := doc["accepted_by"]; ok {
		t.Error("accepted_by should be absent when empty")
	}
	got := docToInvite(doc)
	if got.AcceptedAt != nil {
		t.Errorf("AcceptedAt should be nil: got %v", got.AcceptedAt)
	}
}

func TestRoleRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	orig := &model.Role{
		ID:          "role1",
		Name:        "admin",
		DisplayName: "Administrator",
		Description: "Full access",
		Permissions: []string{"user:manage", "kb:manage_all"},
		Type:        "custom",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	doc := roleToDoc(orig)
	got := docToRole(doc)
	if got.ID != orig.ID || got.Name != orig.Name || got.DisplayName != orig.DisplayName {
		t.Errorf("basic fields mismatch: got %+v", got)
	}
	if got.Description != orig.Description || got.Type != orig.Type {
		t.Errorf("desc/type mismatch: got %+v", got)
	}
	if len(got.Permissions) != 2 || got.Permissions[0] != "user:manage" || got.Permissions[1] != "kb:manage_all" {
		t.Errorf("Permissions mismatch: got %v", got.Permissions)
	}
	if !got.CreatedAt.Equal(orig.CreatedAt) || !got.UpdatedAt.Equal(orig.UpdatedAt) {
		t.Errorf("timestamps mismatch")
	}
}

func TestAuditLogRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	orig := &model.AuditLog{
		ID:         "audit1",
		Action:     "login",
		UserID:     "user1",
		Resource:   "/api/login",
		Details:    "User logged in",
		IP:         "192.168.1.1",
		UserAgent:  "Mozilla/5.0",
		StatusCode: 200,
		CreatedAt:  now,
	}
	doc := auditLogToDoc(orig)
	got := docToAuditLog(doc)
	if got.ID != orig.ID || got.Action != orig.Action || got.UserID != orig.UserID {
		t.Errorf("basic fields mismatch: got %+v", got)
	}
	if got.Resource != orig.Resource || got.Details != orig.Details {
		t.Errorf("resource/details mismatch: got %+v", got)
	}
	if got.IP != orig.IP || got.UserAgent != orig.UserAgent {
		t.Errorf("ip/useragent mismatch: got %+v", got)
	}
	if got.StatusCode != orig.StatusCode {
		t.Errorf("StatusCode mismatch: got %d, want %d", got.StatusCode, orig.StatusCode)
	}
	if !got.CreatedAt.Equal(orig.CreatedAt) {
		t.Errorf("CreatedAt mismatch: got %v, want %v", got.CreatedAt, orig.CreatedAt)
	}
}

func TestNotificationRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	orig := &model.Notification{
		ID:        "notif1",
		Title:     "System Update",
		Content:   "Maintenance scheduled",
		Type:      "info",
		TargetAll: false,
		TargetIDs: []string{"user1", "user2"},
		ReadBy:    []string{"user1"},
		CreatedAt: now,
	}
	doc := notificationToDoc(orig)
	got := docToNotification(doc)
	if got.ID != orig.ID || got.Title != orig.Title || got.Content != orig.Content {
		t.Errorf("basic fields mismatch: got %+v", got)
	}
	if got.Type != orig.Type || got.TargetAll != orig.TargetAll {
		t.Errorf("type/target_all mismatch: got %+v", got)
	}
	if len(got.TargetIDs) != 2 || got.TargetIDs[0] != "user1" || got.TargetIDs[1] != "user2" {
		t.Errorf("TargetIDs mismatch: got %v", got.TargetIDs)
	}
	if len(got.ReadBy) != 1 || got.ReadBy[0] != "user1" {
		t.Errorf("ReadBy mismatch: got %v", got.ReadBy)
	}
	if !got.CreatedAt.Equal(orig.CreatedAt) {
		t.Errorf("CreatedAt mismatch")
	}
}

func TestNotificationEmptySlices(t *testing.T) {
	orig := &model.Notification{
		ID:        "notif2",
		Title:     "Broadcast",
		Content:   "Hello all",
		Type:      "warning",
		TargetAll: true,
	}
	doc := notificationToDoc(orig)
	if _, ok := doc["target_ids"]; ok {
		t.Error("target_ids should be absent when empty")
	}
	if _, ok := doc["read_by"]; ok {
		t.Error("read_by should be absent when empty")
	}
	got := docToNotification(doc)
	if got.TargetIDs != nil {
		t.Errorf("TargetIDs should be nil: got %v", got.TargetIDs)
	}
}

func TestSystemConfigRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	orig := &model.SystemConfig{
		ID:        "cfg1",
		Namespace: "feature_flags",
		Key:       "enable_beta",
		Value:     "true",
		UpdatedAt: now,
	}
	doc := systemConfigToDoc(orig)
	got := docToSystemConfig(doc)
	if got.ID != orig.ID || got.Namespace != orig.Namespace || got.Key != orig.Key {
		t.Errorf("basic fields mismatch: got %+v", got)
	}
	if got.Value != orig.Value {
		t.Errorf("Value mismatch: got %q", got.Value)
	}
	if !got.UpdatedAt.Equal(orig.UpdatedAt) {
		t.Errorf("UpdatedAt mismatch")
	}
}

func TestTaskProgressRoundtrip(t *testing.T) {
	orig := task.TaskProgress{
		CurrentStep: 2,
		TotalSteps:  5,
		Message:     "Processing",
		Percent:     40,
	}
	doc := taskProgressToDoc(orig)
	got := docToTaskProgress(doc)
	if got.CurrentStep != orig.CurrentStep || got.TotalSteps != orig.TotalSteps {
		t.Errorf("step fields mismatch: got %+v", got)
	}
	if got.Message != orig.Message || got.Percent != orig.Percent {
		t.Errorf("message/percent mismatch: got %+v", got)
	}
}

func TestTaskRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	completedAt := now.Add(time.Minute)
	orig := &task.Task{
		ID:         "task_abc",
		SessionID:  "sess1",
		UserID:     "user1",
		Type:       "agent_exec",
		Status:     task.StatusCompleted,
		SkillChain: []string{"sql-executor", "stats-engine"},
		Params:     map[string]interface{}{"query": "SELECT 1"},
		Result:     map[string]interface{}{"rows": 10},
		Error:      "",
		Progress: task.TaskProgress{
			CurrentStep: 2,
			TotalSteps:  2,
			Message:     "Done",
			Percent:     100,
		},
		RetryCount:  0,
		MaxRetries:  3,
		CreatedAt:   now,
		UpdatedAt:   now,
		CompletedAt: &completedAt,
		DurationMs:  1500,
	}
	doc := taskToDoc(orig)
	// Error is empty (omitempty) → should be absent
	if _, ok := doc["error"]; ok {
		t.Error("error should be absent when empty")
	}
	got := docToTask(doc)
	if got.ID != orig.ID || got.SessionID != orig.SessionID || got.UserID != orig.UserID {
		t.Errorf("basic fields mismatch: got %+v", got)
	}
	if got.Type != orig.Type || got.Status != orig.Status {
		t.Errorf("type/status mismatch: got %+v", got)
	}
	if len(got.SkillChain) != 2 || got.SkillChain[0] != "sql-executor" {
		t.Errorf("SkillChain mismatch: got %v", got.SkillChain)
	}
	if got.Params["query"] != "SELECT 1" {
		t.Errorf("Params mismatch: got %v", got.Params)
	}
	if got.Result["rows"] != 10 {
		t.Errorf("Result mismatch: got %v", got.Result)
	}
	if got.Progress.CurrentStep != 2 || got.Progress.TotalSteps != 2 || got.Progress.Percent != 100 {
		t.Errorf("Progress mismatch: got %+v", got.Progress)
	}
	if got.RetryCount != orig.RetryCount || got.MaxRetries != orig.MaxRetries {
		t.Errorf("retry fields mismatch: got %d/%d", got.RetryCount, got.MaxRetries)
	}
	if got.DurationMs != orig.DurationMs {
		t.Errorf("DurationMs mismatch: got %d", got.DurationMs)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(*orig.CompletedAt) {
		t.Errorf("CompletedAt mismatch: got %v", got.CompletedAt)
	}
}

func TestTaskRoundtripNilOptionals(t *testing.T) {
	orig := &task.Task{
		ID:         "task_pending",
		SessionID:  "sess1",
		UserID:     "user1",
		Type:       "agent_exec",
		Status:     task.StatusPending,
		SkillChain: []string{},
		Params:     nil,
	}
	doc := taskToDoc(orig)
	if _, ok := doc["result"]; ok {
		t.Error("result should be absent when nil")
	}
	if _, ok := doc["error"]; ok {
		t.Error("error should be absent when empty")
	}
	if _, ok := doc["completed_at"]; ok {
		t.Error("completed_at should be absent when nil")
	}
	got := docToTask(doc)
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt should be nil: got %v", got.CompletedAt)
	}
	if got.Result != nil {
		t.Errorf("Result should be nil: got %v", got.Result)
	}
}

func TestArtifactRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	orig := &artifact.Artifact{
		ID:          "art1",
		UserID:      "user1",
		SessionID:   "sess1",
		TaskID:      "task1",
		Name:        "report.pdf",
		MimeType:    "application/pdf",
		SizeBytes:   1024,
		StoragePath: "/data/reports/report.pdf",
		Persistent:  true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	doc := artifactToDoc(orig)
	got := docToArtifact(doc)
	if got.ID != orig.ID || got.UserID != orig.UserID || got.SessionID != orig.SessionID {
		t.Errorf("basic fields mismatch: got %+v", got)
	}
	if got.TaskID != orig.TaskID || got.Name != orig.Name || got.MimeType != orig.MimeType {
		t.Errorf("task/name/mime mismatch: got %+v", got)
	}
	if got.SizeBytes != orig.SizeBytes || got.StoragePath != orig.StoragePath || got.Persistent != orig.Persistent {
		t.Errorf("size/path/persistent mismatch: got %+v", got)
	}
	if !got.CreatedAt.Equal(orig.CreatedAt) || !got.UpdatedAt.Equal(orig.UpdatedAt) {
		t.Errorf("timestamps mismatch")
	}
}

func TestArtifactEmptyTaskID(t *testing.T) {
	orig := &artifact.Artifact{
		ID:        "art2",
		UserID:    "user1",
		SessionID: "sess1",
		Name:      "temp.txt",
		MimeType:  "text/plain",
	}
	doc := artifactToDoc(orig)
	if _, ok := doc["task_id"]; ok {
		t.Error("task_id should be absent when empty")
	}
	got := docToArtifact(doc)
	if got.TaskID != "" {
		t.Errorf("TaskID should be empty: got %q", got.TaskID)
	}
}

func TestKnowledgeDocRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	orig := &knowledge.KnowledgeDoc{
		ID:           "doc1",
		UserID:       "user1",
		Title:        "Manual",
		FileName:     "manual.pdf",
		FileType:     "pdf",
		SizeBytes:    2048,
		Status:       knowledge.StatusReady,
		ChunkCount:   10,
		GridFSFileID: "gridfs1",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	doc := knowledgeDocToDoc(orig)
	got := docToKnowledgeDoc(doc)
	if got.ID != orig.ID || got.UserID != orig.UserID || got.Title != orig.Title {
		t.Errorf("basic fields mismatch: got %+v", got)
	}
	if got.FileName != orig.FileName || got.FileType != orig.FileType {
		t.Errorf("file fields mismatch: got %+v", got)
	}
	if got.SizeBytes != orig.SizeBytes || got.ChunkCount != orig.ChunkCount {
		t.Errorf("size/chunk mismatch: got %d/%d", got.SizeBytes, got.ChunkCount)
	}
	if got.Status != orig.Status || got.GridFSFileID != orig.GridFSFileID {
		t.Errorf("status/gridfs mismatch: got %+v", got)
	}
	if !got.CreatedAt.Equal(orig.CreatedAt) || !got.UpdatedAt.Equal(orig.UpdatedAt) {
		t.Errorf("timestamps mismatch")
	}
}

func TestKnowledgeDocEmptyGridFS(t *testing.T) {
	orig := &knowledge.KnowledgeDoc{
		ID:     "doc2",
		UserID: "user1",
		Title:  "Doc",
	}
	doc := knowledgeDocToDoc(orig)
	if _, ok := doc["gridfs_file_id"]; ok {
		t.Error("gridfs_file_id should be absent when empty")
	}
	got := docToKnowledgeDoc(doc)
	if got.GridFSFileID != "" {
		t.Errorf("GridFSFileID should be empty: got %q", got.GridFSFileID)
	}
}

func TestChunkRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	orig := &knowledge.Chunk{
		ID:        "chunk1",
		DocID:     "doc1",
		Content:   "This is a test chunk",
		ChunkIdx:  3,
		CharCount: 20,
		MilvusID:  99,
		CreatedAt: now,
	}
	doc := chunkToDoc(orig)
	got := docToChunk(doc)
	if got.ID != orig.ID || got.DocID != orig.DocID || got.Content != orig.Content {
		t.Errorf("basic fields mismatch: got %+v", got)
	}
	if got.ChunkIdx != orig.ChunkIdx || got.CharCount != orig.CharCount {
		t.Errorf("idx/count mismatch: got %d/%d", got.ChunkIdx, got.CharCount)
	}
	if got.MilvusID != orig.MilvusID {
		t.Errorf("MilvusID mismatch: got %d, want %d", got.MilvusID, orig.MilvusID)
	}
	if !got.CreatedAt.Equal(orig.CreatedAt) {
		t.Errorf("CreatedAt mismatch")
	}
}

func TestChunkZeroMilvusID(t *testing.T) {
	orig := &knowledge.Chunk{
		ID:      "chunk2",
		DocID:   "doc1",
		Content: "No vector",
	}
	doc := chunkToDoc(orig)
	if _, ok := doc["milvus_id"]; ok {
		t.Error("milvus_id should be absent when zero")
	}
	got := docToChunk(doc)
	if got.MilvusID != 0 {
		t.Errorf("MilvusID should be 0: got %d", got.MilvusID)
	}
}

func TestAPIReviewRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	reviewedAt := now.Add(time.Hour)
	orig := &apireview.APIReview{
		ID:           "review1",
		Name:         "Payment API",
		FileName:     "payment.yaml",
		Version:      "3.0",
		Endpoints:    12,
		Domain:       "finance",
		RateLimit:    100,
		Submitter:    "user1",
		Reviewer:     "admin1",
		RejectReason: "",
		Status:       apireview.StatusApproved,
		ReviewedAt:   &reviewedAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	doc := apiReviewToDoc(orig)
	// RejectReason empty → absent
	if _, ok := doc["reject_reason"]; ok {
		t.Error("reject_reason should be absent when empty")
	}
	got := docToAPIReview(doc)
	if got.ID != orig.ID || got.Name != orig.Name || got.FileName != orig.FileName {
		t.Errorf("basic fields mismatch: got %+v", got)
	}
	if got.Version != orig.Version || got.Endpoints != orig.Endpoints || got.Domain != orig.Domain {
		t.Errorf("version/endpoints/domain mismatch: got %+v", got)
	}
	if got.RateLimit != orig.RateLimit || got.Submitter != orig.Submitter {
		t.Errorf("ratelimit/submitter mismatch: got %+v", got)
	}
	if got.Reviewer != orig.Reviewer {
		t.Errorf("Reviewer mismatch: got %q", got.Reviewer)
	}
	if got.Status != orig.Status {
		t.Errorf("Status mismatch: got %q", got.Status)
	}
	if got.ReviewedAt == nil || !got.ReviewedAt.Equal(*orig.ReviewedAt) {
		t.Errorf("ReviewedAt mismatch: got %v", got.ReviewedAt)
	}
	if !got.CreatedAt.Equal(orig.CreatedAt) || !got.UpdatedAt.Equal(orig.UpdatedAt) {
		t.Errorf("timestamps mismatch")
	}
}

func TestAPIReviewPendingNoReviewer(t *testing.T) {
	orig := &apireview.APIReview{
		ID:        "review2",
		Name:      "Pending API",
		FileName:  "pending.yaml",
		Version:   "3.0",
		Endpoints: 5,
		Domain:    "general",
		RateLimit: 50,
		Submitter: "user2",
		Status:    apireview.StatusPending,
	}
	doc := apiReviewToDoc(orig)
	for _, key := range []string{"reviewer", "reject_reason", "reviewed_at"} {
		if _, ok := doc[key]; ok {
			t.Errorf("%s should be absent when empty/nil", key)
		}
	}
	got := docToAPIReview(doc)
	if got.Reviewer != "" {
		t.Errorf("Reviewer should be empty: got %q", got.Reviewer)
	}
	if got.ReviewedAt != nil {
		t.Errorf("ReviewedAt should be nil: got %v", got.ReviewedAt)
	}
}

// ── Edge case tests for 100% coverage ───────────────────────────────

func TestGetTimeNonTime(t *testing.T) {
	// Non-time value → zero time
	d := bson.M{"ts": "not-a-time"}
	if got := getTime(d, "ts"); !got.IsZero() {
		t.Errorf("getTime non-time: got %v, want zero", got)
	}
}

func TestGetTimePtrDateTime(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	dt := primitive.NewDateTimeFromTime(now)
	d := bson.M{"ts": dt}
	got := getTimePtr(d, "ts")
	if got == nil || !got.Equal(now) {
		t.Errorf("getTimePtr DateTime: got %v, want %v", got, now)
	}
	// primitive.DateTime that is zero → nil
	d2 := bson.M{"ts": primitive.NewDateTimeFromTime(time.Time{})}
	if p := getTimePtr(d2, "ts"); p != nil {
		t.Errorf("getTimePtr zero DateTime: got %v, want nil", p)
	}
	// non-time type → nil
	d3 := bson.M{"ts": "string"}
	if p := getTimePtr(d3, "ts"); p != nil {
		t.Errorf("getTimePtr non-time: got %v, want nil", p)
	}
}

func TestGetIntNonNumeric(t *testing.T) {
	d := bson.M{"n": "string"}
	if got := getInt(d, "n"); got != 0 {
		t.Errorf("getInt non-numeric: got %d, want 0", got)
	}
}

func TestGetInt64NonNumeric(t *testing.T) {
	d := bson.M{"n": "string"}
	if got := getInt64(d, "n"); got != 0 {
		t.Errorf("getInt64 non-numeric: got %d, want 0", got)
	}
}

func TestGetStrSliceNonSlice(t *testing.T) {
	d := bson.M{"items": "not-a-slice"}
	if got := getStrSlice(d, "items"); got != nil {
		t.Errorf("getStrSlice non-slice: got %v, want nil", got)
	}
}

func TestGetMapNonMap(t *testing.T) {
	d := bson.M{"m": "string"}
	if got := getMap(d, "m"); got != nil {
		t.Errorf("getMap non-map: got %v, want nil", got)
	}
}

func TestGetSubDocNonDoc(t *testing.T) {
	d := bson.M{"sub": "string"}
	got := getSubDoc(d, "sub")
	if len(got) != 0 {
		t.Errorf("getSubDoc non-doc: got %v, want empty", got)
	}
}

func TestTaskToDocWithError(t *testing.T) {
	orig := &task.Task{
		ID:        "task_err",
		SessionID: "sess1",
		UserID:    "user1",
		Type:      "agent_exec",
		Status:    task.StatusFailed,
		Error:     "connection timeout",
	}
	doc := taskToDoc(orig)
	if v, ok := doc["error"]; !ok || v != "connection timeout" {
		t.Errorf("error field: got %v, want %q", v, "connection timeout")
	}
	got := docToTask(doc)
	if got.Error != orig.Error {
		t.Errorf("Error mismatch: got %q, want %q", got.Error, orig.Error)
	}
}

func TestAPIReviewToDocWithRejectReason(t *testing.T) {
	orig := &apireview.APIReview{
		ID:           "review3",
		Name:         "Bad API",
		FileName:     "bad.yaml",
		Version:      "3.0",
		Endpoints:    2,
		Domain:       "test",
		RateLimit:    10,
		Submitter:    "user3",
		Reviewer:     "admin2",
		RejectReason: "Invalid schema",
		Status:       apireview.StatusRejected,
	}
	doc := apiReviewToDoc(orig)
	if v, ok := doc["reject_reason"]; !ok || v != "Invalid schema" {
		t.Errorf("reject_reason: got %v, want %q", v, "Invalid schema")
	}
	got := docToAPIReview(doc)
	if got.RejectReason != orig.RejectReason {
		t.Errorf("RejectReason mismatch: got %q, want %q", got.RejectReason, orig.RejectReason)
	}
}
