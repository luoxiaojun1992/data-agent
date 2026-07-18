package adksession

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// newServiceWithMockColl returns a Service whose collection methods are patchable.
func newServiceWithMockColl() (*Service, *mongo.Collection) {
	var coll mongo.Collection
	return &Service{coll: &coll}, &coll
}

func textEvent(author, text string) *session.Event {
	return &session.Event{
		ID:        "evt_" + author,
		Timestamp: time.Now(),
		Author:    author,
		LLMResponse: model.LLMResponse{
			Content: genai.NewContentFromText(text, "model"),
		},
	}
}

func TestCreate(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(coll, "UpdateOne", &mongo.UpdateResult{UpsertedCount: 1}, nil)

	resp, err := svc.Create(context.Background(), &session.CreateRequest{
		AppName: "app", UserID: "u1", SessionID: "s1",
		State: map[string]any{"user_id": "u1"},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if resp.Session.ID() != "s1" || resp.Session.AppName() != "app" || resp.Session.UserID() != "u1" {
		t.Errorf("unexpected session: %v %v %v", resp.Session.ID(), resp.Session.AppName(), resp.Session.UserID())
	}
	v, err := resp.Session.State().Get("user_id")
	if err != nil || v != "u1" {
		t.Errorf("state user_id = %v, %v", v, err)
	}
	if resp.Session.Events().Len() != 0 {
		t.Errorf("events should be empty")
	}
	if resp.Session.LastUpdateTime().IsZero() {
		t.Errorf("LastUpdateTime should be set")
	}
}

func TestCreate_AutoID(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(coll, "UpdateOne", &mongo.UpdateResult{}, nil)

	resp, err := svc.Create(context.Background(), &session.CreateRequest{AppName: "app", UserID: "u1"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if resp.Session.ID() == "" {
		t.Error("session id should be auto-generated")
	}
}

func TestCreate_Error(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(coll, "UpdateOne", (*mongo.UpdateResult)(nil), fmt.Errorf("db down"))

	_, err := svc.Create(context.Background(), &session.CreateRequest{AppName: "a", UserID: "u"})
	if err == nil {
		t.Error("expected error")
	}
}

func TestGet_Filters(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	now := time.Now()
	old := &session.Event{ID: "e1", Timestamp: now.Add(-time.Hour), Author: "user", LLMResponse: textEvent("user", "old").LLMResponse}
	recent := &session.Event{ID: "e2", Timestamp: now, Author: "model", LLMResponse: textEvent("model", "new").LLMResponse}
	doc := &sessionDoc{ID: "s1", AppName: "app", UserID: "u1", State: map[string]any{}, Events: []*session.Event{old, recent}, UpdatedAt: now}

	sr := &mongo.SingleResult{}
	patches.ApplyMethodReturn(coll, "FindOne", sr)
	patches.ApplyMethod(sr, "Decode", func(_ *mongo.SingleResult, v any) error {
		*v.(*sessionDoc) = *doc
		return nil
	})

	// No filter.
	resp, err := svc.Get(context.Background(), &session.GetRequest{AppName: "app", UserID: "u1", SessionID: "s1"})
	if err != nil || resp.Session.Events().Len() != 2 {
		t.Fatalf("Get failed: %v (events=%d)", err, resp.Session.Events().Len())
	}

	// NumRecentEvents filter.
	resp, err = svc.Get(context.Background(), &session.GetRequest{AppName: "app", UserID: "u1", SessionID: "s1", NumRecentEvents: 1})
	if err != nil || resp.Session.Events().Len() != 1 {
		t.Fatalf("NumRecentEvents filter failed: %v", err)
	}
	if resp.Session.Events().At(0).ID != "e2" {
		t.Errorf("expected most recent event e2, got %s", resp.Session.Events().At(0).ID)
	}

	// After filter.
	resp, err = svc.Get(context.Background(), &session.GetRequest{AppName: "app", UserID: "u1", SessionID: "s1", After: now.Add(-time.Minute)})
	if err != nil || resp.Session.Events().Len() != 1 {
		t.Fatalf("After filter failed: %v", err)
	}

	// Not found.
	patches.ApplyMethod(sr, "Decode", func(_ *mongo.SingleResult, v any) error {
		return mongo.ErrNoDocuments
	})
	_, err = svc.Get(context.Background(), &session.GetRequest{AppName: "app", UserID: "u1", SessionID: "missing"})
	if err == nil {
		t.Error("expected not found error")
	}
}

func TestList(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	cur := &mongo.Cursor{}
	docs := []*sessionDoc{
		{ID: "s1", AppName: "app", UserID: "u1", State: map[string]any{}, UpdatedAt: time.Now()},
		{ID: "s2", AppName: "app", UserID: "u1", State: map[string]any{}, UpdatedAt: time.Now()},
	}
	idx := 0
	patches.ApplyMethodReturn(coll, "Find", cur, nil)
	patches.ApplyMethod(cur, "Next", func(_ *mongo.Cursor, ctx context.Context) bool {
		return idx < len(docs)
	})
	patches.ApplyMethod(cur, "Decode", func(_ *mongo.Cursor, v any) error {
		*v.(*sessionDoc) = *docs[idx]
		idx++
		return nil
	})
	patches.ApplyMethodReturn(cur, "Close", nil)
	patches.ApplyMethodReturn(cur, "Err", nil)

	resp, err := svc.List(context.Background(), &session.ListRequest{AppName: "app", UserID: "u1"})
	if err != nil || len(resp.Sessions) != 2 {
		t.Fatalf("List failed: %v (n=%d)", err, len(resp.Sessions))
	}
}

func TestList_DecodeError(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	cur := &mongo.Cursor{}
	calls := 0
	patches.ApplyMethodReturn(coll, "Find", cur, nil)
	patches.ApplyMethod(cur, "Next", func(_ *mongo.Cursor, ctx context.Context) bool {
		calls++
		return calls == 1
	})
	patches.ApplyMethod(cur, "Decode", func(_ *mongo.Cursor, v any) error {
		return fmt.Errorf("bad doc")
	})
	patches.ApplyMethodReturn(cur, "Close", nil)
	patches.ApplyMethodReturn(cur, "Err", nil)

	_, err := svc.List(context.Background(), &session.ListRequest{AppName: "a", UserID: "u"})
	if err == nil {
		t.Error("expected decode error")
	}
}

func TestList_CursorError(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	cur := &mongo.Cursor{}
	patches.ApplyMethodReturn(coll, "Find", cur, nil)
	patches.ApplyMethod(cur, "Next", func(_ *mongo.Cursor, ctx context.Context) bool { return false })
	patches.ApplyMethodReturn(cur, "Close", nil)
	patches.ApplyMethodReturn(cur, "Err", fmt.Errorf("cursor broke"))

	_, err := svc.List(context.Background(), &session.ListRequest{AppName: "a", UserID: "u"})
	if err == nil {
		t.Error("expected cursor error")
	}
}

func TestCompaction_FindError(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	sum := &fakeSummarizer{text: "s"}
	svc.WithCompaction(CompactionConfig{MaxEvents: 1, KeepRecent: 1}, sum)

	sess := (&sessionDoc{ID: "s1", AppName: "app", UserID: "u1", State: map[string]any{}}).toSession()

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(coll, "UpdateOne", &mongo.UpdateResult{MatchedCount: 1}, nil)
	sr := &mongo.SingleResult{}
	patches.ApplyMethodReturn(coll, "FindOne", sr)
	patches.ApplyMethod(sr, "Decode", func(_ *mongo.SingleResult, v any) error {
		return mongo.ErrNoDocuments
	})

	err := svc.AppendEvent(context.Background(), sess, textEvent("user", "x"))
	if err == nil {
		t.Error("expected find error from compaction")
	}
}

func TestList_FindError(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(coll, "Find", (*mongo.Cursor)(nil), fmt.Errorf("db down"))

	_, err := svc.List(context.Background(), &session.ListRequest{AppName: "a", UserID: "u"})
	if err == nil {
		t.Error("expected error")
	}
}

func TestDelete_Idempotent(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(coll, "DeleteOne", &mongo.DeleteResult{DeletedCount: 0}, nil)

	// Deleting a non-existent session must NOT error (idempotent contract).
	if err := svc.Delete(context.Background(), &session.DeleteRequest{AppName: "a", UserID: "u", SessionID: "s"}); err != nil {
		t.Errorf("delete should be idempotent: %v", err)
	}

	patches.ApplyMethodReturn(coll, "DeleteOne", (*mongo.DeleteResult)(nil), fmt.Errorf("db down"))
	if err := svc.Delete(context.Background(), &session.DeleteRequest{AppName: "a", UserID: "u", SessionID: "s"}); err == nil {
		t.Error("expected db error")
	}
}

func TestAppendEvent(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var capturedUpdate bson.M
	patches.ApplyMethod(coll, "UpdateOne", func(_ *mongo.Collection, ctx context.Context, filter, update any, _ ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
		capturedUpdate = update.(bson.M)
		return &mongo.UpdateResult{MatchedCount: 1}, nil
	})

	sess := (&sessionDoc{ID: "s1", AppName: "app", UserID: "u1", State: map[string]any{}}).toSession()
	evt := &session.Event{Author: "user", LLMResponse: textEvent("user", "hello").LLMResponse}
	evt.Actions.StateDelta = map[string]any{"kb_id": "kb1", "bad.key": "skip", "$evil": "skip"}

	if err := svc.AppendEvent(context.Background(), sess, evt); err != nil {
		t.Fatalf("AppendEvent failed: %v", err)
	}
	if evt.ID == "" || evt.Timestamp.IsZero() {
		t.Error("event id/timestamp should be populated")
	}
	setFields := capturedUpdate["$set"].(bson.M)
	if setFields["state.kb_id"] != "kb1" {
		t.Errorf("state delta not merged: %v", setFields)
	}
	if _, bad := setFields["state.bad.key"]; bad {
		t.Error("unsafe key must be skipped")
	}
	if _, bad := setFields["state.$evil"]; bad {
		t.Error("dollar-prefixed key must be skipped")
	}
}

func TestAppendEvent_NotFound(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(coll, "UpdateOne", &mongo.UpdateResult{MatchedCount: 0}, nil)

	sess := (&sessionDoc{ID: "missing", AppName: "a", UserID: "u"}).toSession()
	err := svc.AppendEvent(context.Background(), sess, textEvent("user", "hi"))
	if err == nil {
		t.Error("expected not found error")
	}
}

func TestAppendEvent_DBError(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(coll, "UpdateOne", (*mongo.UpdateResult)(nil), fmt.Errorf("db down"))

	sess := (&sessionDoc{ID: "s", AppName: "a", UserID: "u"}).toSession()
	if err := svc.AppendEvent(context.Background(), sess, textEvent("user", "hi")); err == nil {
		t.Error("expected db error")
	}
}

// ---- compaction ----

type fakeSummarizer struct {
	calls int
	text  string
	err   error
}

func (f *fakeSummarizer) Summarize(ctx context.Context, events []*session.Event) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return f.text, nil
}

func TestCompaction_Triggers(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	sum := &fakeSummarizer{text: "summary of old"}
	svc.WithCompaction(CompactionConfig{MaxEvents: 3, MaxTokens: 1000000, KeepRecent: 1}, sum)

	sess := (&sessionDoc{ID: "s1", AppName: "app", UserID: "u1", State: map[string]any{}}).toSession()

	patches := gomonkey.NewPatches()
	defer patches.Reset()

	// Existing 5 events in store → exceeds MaxEvents=3.
	events := make([]*session.Event, 5)
	for i := range events {
		events[i] = textEvent("user", fmt.Sprintf("m%d", i))
	}
	doc := &sessionDoc{ID: "s1", AppName: "app", UserID: "u1", State: map[string]any{}, Events: events, UpdatedAt: time.Now()}

	sr := &mongo.SingleResult{}
	patches.ApplyMethodReturn(coll, "FindOne", sr)
	patches.ApplyMethod(sr, "Decode", func(_ *mongo.SingleResult, v any) error {
		*v.(*sessionDoc) = *doc
		return nil
	})

	var rewritten []*session.Event
	patches.ApplyMethod(coll, "UpdateOne", func(_ *mongo.Collection, ctx context.Context, filter, update any, _ ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
		u := update.(bson.M)
		if set, ok := u["$set"].(bson.M); ok {
			if evs, ok := set["events"].([]*session.Event); ok {
				rewritten = evs
			}
		}
		return &mongo.UpdateResult{MatchedCount: 1}, nil
	})

	if err := svc.AppendEvent(context.Background(), sess, textEvent("user", "trigger")); err != nil {
		t.Fatalf("AppendEvent failed: %v", err)
	}
	if sum.calls != 1 {
		t.Fatalf("summarizer should be called once, got %d", sum.calls)
	}
	// KeepRecent=1 → rewritten = 1 compaction event + 1 recent = 2.
	if len(rewritten) != 2 {
		t.Fatalf("expected 2 events after compaction, got %d", len(rewritten))
	}
	if rewritten[0].Author != "compaction" {
		t.Errorf("first event should be compaction, got %q", rewritten[0].Author)
	}
	if rewritten[1].Author != "user" {
		t.Errorf("recent event should be preserved, got %q", rewritten[1].Author)
	}
}

func TestCompaction_TokenThreshold(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	sum := &fakeSummarizer{text: "s"}
	svc.WithCompaction(CompactionConfig{MaxEvents: 1000000, MaxTokens: 5, KeepRecent: 1}, sum)

	sess := (&sessionDoc{ID: "s1", AppName: "app", UserID: "u1", State: map[string]any{}}).toSession()

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	events := make([]*session.Event, 4)
	for i := range events {
		events[i] = textEvent("user", "this is a fairly long message body")
	}
	doc := &sessionDoc{ID: "s1", AppName: "app", UserID: "u1", State: map[string]any{}, Events: events, UpdatedAt: time.Now()}
	sr := &mongo.SingleResult{}
	patches.ApplyMethodReturn(coll, "FindOne", sr)
	patches.ApplyMethod(sr, "Decode", func(_ *mongo.SingleResult, v any) error {
		*v.(*sessionDoc) = *doc
		return nil
	})
	patches.ApplyMethodReturn(coll, "UpdateOne", &mongo.UpdateResult{MatchedCount: 1}, nil)

	if err := svc.AppendEvent(context.Background(), sess, textEvent("user", "x")); err != nil {
		t.Fatalf("AppendEvent failed: %v", err)
	}
	if sum.calls != 1 {
		t.Errorf("token threshold should trigger compaction, calls=%d", sum.calls)
	}
}

func TestCompaction_BelowThreshold(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	sum := &fakeSummarizer{text: "s"}
	svc.WithCompaction(CompactionConfig{MaxEvents: 100, MaxTokens: 100000, KeepRecent: 20}, sum)

	sess := (&sessionDoc{ID: "s1", AppName: "app", UserID: "u1", State: map[string]any{}}).toSession()

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	doc := &sessionDoc{ID: "s1", AppName: "app", UserID: "u1", State: map[string]any{}, Events: []*session.Event{textEvent("user", "hi")}, UpdatedAt: time.Now()}
	sr := &mongo.SingleResult{}
	patches.ApplyMethodReturn(coll, "FindOne", sr)
	patches.ApplyMethod(sr, "Decode", func(_ *mongo.SingleResult, v any) error {
		*v.(*sessionDoc) = *doc
		return nil
	})
	patches.ApplyMethodReturn(coll, "UpdateOne", &mongo.UpdateResult{MatchedCount: 1}, nil)

	if err := svc.AppendEvent(context.Background(), sess, textEvent("user", "x")); err != nil {
		t.Fatalf("AppendEvent failed: %v", err)
	}
	if sum.calls != 0 {
		t.Errorf("below threshold must not compact, calls=%d", sum.calls)
	}
}

func TestCompaction_TooFewEvents(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	sum := &fakeSummarizer{text: "s"}
	// MaxEvents=2 forces threshold, but KeepRecent=20 > event count.
	svc.WithCompaction(CompactionConfig{MaxEvents: 2, MaxTokens: 1, KeepRecent: 20}, sum)

	sess := (&sessionDoc{ID: "s1", AppName: "app", UserID: "u1", State: map[string]any{}}).toSession()

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	doc := &sessionDoc{ID: "s1", AppName: "app", UserID: "u1", State: map[string]any{}, Events: []*session.Event{textEvent("user", "a"), textEvent("user", "b"), textEvent("user", "c")}, UpdatedAt: time.Now()}
	sr := &mongo.SingleResult{}
	patches.ApplyMethodReturn(coll, "FindOne", sr)
	patches.ApplyMethod(sr, "Decode", func(_ *mongo.SingleResult, v any) error {
		*v.(*sessionDoc) = *doc
		return nil
	})
	patches.ApplyMethodReturn(coll, "UpdateOne", &mongo.UpdateResult{MatchedCount: 1}, nil)

	if err := svc.AppendEvent(context.Background(), sess, textEvent("user", "x")); err != nil {
		t.Fatalf("AppendEvent failed: %v", err)
	}
	if sum.calls != 0 {
		t.Errorf("too few events must not compact, calls=%d", sum.calls)
	}
}

func TestCompaction_SummarizerError(t *testing.T) {
	svc, coll := newServiceWithMockColl()
	sum := &fakeSummarizer{err: fmt.Errorf("llm down")}
	svc.WithCompaction(CompactionConfig{MaxEvents: 2, MaxTokens: 1000000, KeepRecent: 1}, sum)

	sess := (&sessionDoc{ID: "s1", AppName: "app", UserID: "u1", State: map[string]any{}}).toSession()

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	doc := &sessionDoc{ID: "s1", AppName: "app", UserID: "u1", State: map[string]any{}, Events: []*session.Event{textEvent("user", "a"), textEvent("user", "b"), textEvent("user", "c")}, UpdatedAt: time.Now()}
	sr := &mongo.SingleResult{}
	patches.ApplyMethodReturn(coll, "FindOne", sr)
	patches.ApplyMethod(sr, "Decode", func(_ *mongo.SingleResult, v any) error {
		*v.(*sessionDoc) = *doc
		return nil
	})
	patches.ApplyMethodReturn(coll, "UpdateOne", &mongo.UpdateResult{MatchedCount: 1}, nil)

	err := svc.AppendEvent(context.Background(), sess, textEvent("user", "x"))
	if err == nil {
		t.Error("summarizer error should propagate")
	}
}

func TestCompactionConfig_Defaults(t *testing.T) {
	cfg := CompactionConfig{}.withDefaults()
	if cfg.MaxEvents != 100 || cfg.MaxTokens != 4000 || cfg.KeepRecent != 20 {
		t.Errorf("defaults = %+v", cfg)
	}
	cfg = CompactionConfig{MaxEvents: 5, MaxTokens: 6, KeepRecent: 7}.withDefaults()
	if cfg.MaxEvents != 5 || cfg.MaxTokens != 6 || cfg.KeepRecent != 7 {
		t.Errorf("explicit values must be preserved: %+v", cfg)
	}
}

// ---- state/events views ----

func TestStateView(t *testing.T) {
	m := map[string]any{"a": 1}
	s := &stateView{m: m}

	v, err := s.Get("a")
	if err != nil || v != 1 {
		t.Errorf("Get = %v, %v", v, err)
	}
	if _, err := s.Get("missing"); err == nil {
		t.Error("missing key should error")
	}
	if err := s.Set("b", 2); err != nil {
		t.Errorf("Set failed: %v", err)
	}
	seen := map[string]any{}
	for k, v := range s.All() {
		seen[k] = v
		break // exercise early-break path
	}
	if len(seen) != 1 {
		t.Errorf("All early break = %v", seen)
	}
	all := map[string]any{}
	for k, v := range s.All() {
		all[k] = v
	}
	if len(all) != 2 || all["b"] != 2 {
		t.Errorf("All = %v", all)
	}
}

func TestEventsView(t *testing.T) {
	events := eventsView([]*session.Event{textEvent("user", "a"), textEvent("model", "b")})
	if events.Len() != 2 {
		t.Errorf("Len = %d", events.Len())
	}
	if events.At(1).Author != "model" {
		t.Errorf("At(1) = %v", events.At(1).Author)
	}
	count := 0
	for range events.All() {
		count++
		break
	}
	if count != 1 {
		t.Errorf("All early break = %d", count)
	}
}

func TestEstimateEventTokens(t *testing.T) {
	if got := estimateEventTokens(nil); got != 0 {
		t.Errorf("nil events = %d", got)
	}
	events := []*session.Event{
		nil,
		{},
		textEvent("user", "12345678"), // 8 chars → 2 tokens
	}
	if got := estimateEventTokens(events); got != 2 {
		t.Errorf("tokens = %d, want 2", got)
	}
}

func TestNewService(t *testing.T) {
	db := &mongo.Database{}
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(db, "Collection", &coll)
	defer patches.Reset()

	svc := NewService(db)
	if svc.coll != &coll {
		t.Error("collection should come from db.Collection")
	}
}
