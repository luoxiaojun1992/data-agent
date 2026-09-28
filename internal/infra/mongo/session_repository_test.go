package mongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// TestChatSourceFilter asserts the shared chat-session list filter scopes to
// the user and excludes task/feishu sessions via $ne:true (which also matches
// legacy documents where the omitempty field was never written) — SPEC-104 D3.
func TestChatSourceFilter(t *testing.T) {
	f := chatSourceFilter("u1")

	if f["user_id"] != "u1" {
		t.Errorf("user_id = %v, want u1", f["user_id"])
	}

	isTask, ok := f["is_task"].(bson.M)
	if !ok {
		t.Fatalf("is_task filter missing or wrong type: %v", f["is_task"])
	}
	if isTask["$ne"] != true {
		t.Errorf("is_task filter = %v, want {$ne: true}", isTask)
	}

	isFeishu, ok := f["is_feishu"].(bson.M)
	if !ok {
		t.Fatalf("is_feishu filter missing or wrong type: %v", f["is_feishu"])
	}
	if isFeishu["$ne"] != true {
		t.Errorf("is_feishu filter = %v, want {$ne: true}", isFeishu)
	}
}
