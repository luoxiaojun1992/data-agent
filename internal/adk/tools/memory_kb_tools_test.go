package adktools

import (
	"context"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/ieshan/adk-go-memory/adapter"
	"github.com/ieshan/idx"
	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	knowledgepkg "github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
	"google.golang.org/adk/agent"
)

// ---- test doubles ----

// fakeMemoryLister is a minimal MemoryLister double that records its args.
type fakeMemoryLister struct {
	obs       []adapter.Observation
	total     int64
	err       error
	gotUserID string
	gotLimit  int
	gotOffset int
}

func (f *fakeMemoryLister) ListRecent(ctx context.Context, userID string, limit, offset int) ([]adapter.Observation, int64, error) {
	f.gotUserID = userID
	f.gotLimit = limit
	f.gotOffset = offset
	return f.obs, f.total, f.err
}

// newToolContextWithUser builds a tool context whose state carries a user_id
// (used by memory_list / kb_create_doc, which force-bind identity to the
// session state).
func newToolContextWithUser(userID string) *fakeToolContext {
	vals := map[string]any{"session_id": "s1"}
	if userID != "" {
		vals["user_id"] = userID
	}
	return &fakeToolContext{
		StrictContextMock: agent.StrictContextMock{Ctx: context.Background()},
		state:             &fakeState{vals: vals},
	}
}

// ---- memory_list ----

func TestMemoryList_Success(t *testing.T) {
	now := time.Now()
	lister := &fakeMemoryLister{
		obs: []adapter.Observation{
			{ID: idx.NewID(), Content: "记忆A", CreatedAt: now},
			{ID: idx.NewID(), Content: "记忆B", CreatedAt: now.Add(-time.Hour)},
		},
		total: 10,
	}
	fn := memoryList(&Deps{MemoryLister: lister})
	res, err := fn(newToolContextWithUser("u1"), MemoryListArgs{Limit: 5, Offset: 0})
	if err != nil {
		t.Fatalf("memoryList: %v", err)
	}
	if res.Count != 2 {
		t.Errorf("Count = %d, want 2", res.Count)
	}
	if !res.HasMore {
		t.Error("HasMore should be true (0+2 < 10)")
	}
	if lister.gotUserID != "u1" {
		t.Errorf("ListRecent userID = %q, want u1", lister.gotUserID)
	}
	if res.Memories[0].Content != "记忆A" || res.Memories[0].CreatedAt == "" {
		t.Errorf("first memory = %+v", res.Memories[0])
	}
	if res.Memories[0].ID == "" {
		t.Error("memory id should be non-empty")
	}
}

func TestMemoryList_LimitClampedTo50(t *testing.T) {
	lister := &fakeMemoryLister{obs: []adapter.Observation{}, total: 0}
	fn := memoryList(&Deps{MemoryLister: lister})
	_, err := fn(newToolContextWithUser("u1"), MemoryListArgs{Limit: 100, Offset: -5})
	if err != nil {
		t.Fatalf("memoryList: %v", err)
	}
	if lister.gotLimit != 50 {
		t.Errorf("gotLimit = %d, want 50", lister.gotLimit)
	}
	if lister.gotOffset != 0 {
		t.Errorf("gotOffset = %d, want 0 (negative clamped)", lister.gotOffset)
	}
}

func TestMemoryList_EmptyUserID(t *testing.T) {
	lister := &fakeMemoryLister{}
	fn := memoryList(&Deps{MemoryLister: lister})
	// state has session_id but no user_id → refuse (never read all).
	_, err := fn(newToolContext("s1"), MemoryListArgs{})
	if err == nil {
		t.Fatal("expected error when session has no user_id")
	}
}

func TestMemoryList_NilLister(t *testing.T) {
	fn := memoryList(&Deps{MemoryLister: nil})
	_, err := fn(newToolContextWithUser("u1"), MemoryListArgs{})
	if err == nil {
		t.Fatal("expected error when MemoryLister is nil")
	}
}

// ---- kb_create_doc ----

func TestKBCreateDoc_Success(t *testing.T) {
	svc := knowledgepkg.NewService(nil)
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(svc, "CreateTextDoc", func(ctx context.Context, userID, title, text string) (*knowledge.KnowledgeDoc, error) {
		if userID != "u1" {
			t.Errorf("CreateTextDoc userID = %q, want u1", userID)
		}
		return &knowledge.KnowledgeDoc{ID: "kbdoc_test", Title: title}, nil
	})

	fn := kbCreateDoc(&Deps{KBService: svc})
	res, err := fn(newToolContextWithUser("u1"), KBCreateDocArgs{Title: "日常总结", Content: "内容"})
	if err != nil {
		t.Fatalf("kbCreateDoc: %v", err)
	}
	if res.DocID != "kbdoc_test" {
		t.Errorf("DocID = %q, want kbdoc_test", res.DocID)
	}
	if res.Status != "created" {
		t.Errorf("Status = %q, want created", res.Status)
	}
}

func TestKBCreateDoc_EmptyTitle(t *testing.T) {
	svc := knowledgepkg.NewService(nil)
	fn := kbCreateDoc(&Deps{KBService: svc})
	_, err := fn(newToolContextWithUser("u1"), KBCreateDocArgs{Title: "  ", Content: "内容"})
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestKBCreateDoc_EmptyContent(t *testing.T) {
	svc := knowledgepkg.NewService(nil)
	fn := kbCreateDoc(&Deps{KBService: svc})
	_, err := fn(newToolContextWithUser("u1"), KBCreateDocArgs{Title: "t", Content: "  "})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestKBCreateDoc_EmptyUserID(t *testing.T) {
	svc := knowledgepkg.NewService(nil)
	fn := kbCreateDoc(&Deps{KBService: svc})
	_, err := fn(newToolContext("s1"), KBCreateDocArgs{Title: "t", Content: "c"})
	if err == nil {
		t.Fatal("expected error when session has no user_id")
	}
}

func TestKBCreateDoc_NilKBService(t *testing.T) {
	fn := kbCreateDoc(&Deps{KBService: nil})
	_, err := fn(newToolContextWithUser("u1"), KBCreateDocArgs{Title: "t", Content: "c"})
	if err == nil {
		t.Fatal("expected error when KBService is nil")
	}
}
