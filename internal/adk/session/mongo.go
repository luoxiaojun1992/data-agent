package adksession

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

const CollectionName = "adk_sessions"

// SessionEventsCollection stores the append-only raw event stream — one event
// per document (SPEC-069 问题 4). It replaces the session document's
// raw_events array field for new writes; old sessions' arrays are still read
// as a fallback in DisplayEvents.
const SessionEventsCollection = "session_events"

type sessionDoc struct {
	ID        string           `bson:"_id"`
	AppName   string           `bson:"app_name"`
	UserID    string           `bson:"user_id"`
	State     map[string]any   `bson:"state"`
	Events    []*session.Event `bson:"events"`
	RawEvents []*session.Event `bson:"raw_events"` // legacy: fallback for pre-SPEC-069 sessions
	UpdatedAt time.Time        `bson:"updated_at"`
}

// sessionEventDoc is one raw (never-compacted) event in session_events.
type sessionEventDoc struct {
	ID        string         `bson:"_id"` // 纯 UUID，不承载业务语义
	SessionID string         `bson:"session_id"`
	AppName   string         `bson:"app_name"`
	UserID    string         `bson:"user_id"`
	Seq       int64          `bson:"seq"` // UnixNano 递增序号（排序 + 去重）
	Event     *session.Event `bson:"event"`
	CreatedAt time.Time      `bson:"created_at"`
}

// chunkBuffer accumulates streaming text for one session before flushing.
type chunkBuffer struct {
	author  string
	eventID string
	since   time.Time
	text    strings.Builder
	size    int // approximate bytes buffered, for the threshold backstop
}

// flushThresholdBytes is the maximum buffered streaming text before an early
// flush. It is a memory-safety backstop only: normal replies are far below
// this and are flushed once the stream completes (the next non-partial event).
const flushThresholdBytes = 128 * 1024

type Service struct {
	coll       *mongo.Collection
	evtColl    *mongo.Collection // session_events 独立 collection（raw 事件流）
	mu         sync.Mutex
	summarizer Summarizer
	compact    CompactionConfig
	// buf accumulates the in-progress streaming text for each session (one
	// complete message per session). Access is guarded by mu.
	buf map[string]*chunkBuffer
}

func NewService(db *mongo.Database) *Service {
	evtColl := db.Collection(SessionEventsCollection)
	// 幂等创建唯一索引（append-only 排序 + 去重）。CreateOne 重复执行安全。
	_, _ = evtColl.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys:    bson.D{{Key: "session_id", Value: 1}, {Key: "seq", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_session_seq"),
	})
	return &Service{
		coll:    db.Collection(CollectionName),
		evtColl: evtColl,
		buf:     make(map[string]*chunkBuffer),
	}
}

type Summarizer interface {
	Summarize(ctx context.Context, events []*session.Event) (string, error)
}

type CompactionConfig struct {
	MaxEvents  int
	MaxTokens  int
	KeepRecent int
	// MaxTokensFn optionally overrides MaxTokens dynamically (e.g. derived
	// from the compaction model's context length). nil → use MaxTokens.
	MaxTokensFn func(ctx context.Context) int
}

func (s *Service) WithCompaction(cfg CompactionConfig, summarizer Summarizer) *Service {
	s.compact = cfg
	s.summarizer = summarizer
	return s
}

func (s *Service) Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error) {
	id := req.SessionID
	if id == "" {
		id = "sess_" + uuid.New().String()
	}
	// Idempotent: if the session already exists, return it instead of erroring.
	// chat prepareRun invokes Create on every user message; for an existing
	// session the InsertOne below would otherwise fail with duplicate-key.
	if existing, err := s.find(ctx, req.AppName, req.UserID, id); err == nil && existing != nil {
		return &session.CreateResponse{Session: existing.toSession()}, nil
	}
	doc := &sessionDoc{
		ID:        id,
		AppName:   req.AppName,
		UserID:    req.UserID,
		State:     map[string]any{},
		Events:    []*session.Event{},
		RawEvents: []*session.Event{},
		UpdatedAt: time.Now(),
	}
	for k, v := range req.State {
		doc.State[k] = v
	}
	_, err := s.coll.InsertOne(ctx, doc)
	if err != nil {
		// Race: a concurrent Create from another goroutine may have inserted
		// between our find and InsertOne. Treat the duplicate as success.
		if mongo.IsDuplicateKeyError(err) {
			if existing, gErr := s.find(ctx, req.AppName, req.UserID, id); gErr == nil && existing != nil {
				return &session.CreateResponse{Session: existing.toSession()}, nil
			}
		}
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &session.CreateResponse{Session: doc.toSession()}, nil
}

func (s *Service) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	doc, err := s.find(ctx, req.AppName, req.UserID, req.SessionID)
	if err != nil {
		return nil, err
	}
	return &session.GetResponse{Session: doc.toSession()}, nil
}

func (s *Service) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	cursor, err := s.coll.Find(ctx, bson.M{"app_name": req.AppName, "user_id": req.UserID})
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer cursor.Close(ctx)
	resp := &session.ListResponse{Sessions: []session.Session{}}
	for cursor.Next(ctx) {
		var doc sessionDoc
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("decode session: %w", err)
		}
		resp.Sessions = append(resp.Sessions, doc.toSession())
	}
	return resp, cursor.Err()
}

func (s *Service) Delete(ctx context.Context, req *session.DeleteRequest) error {
	_, err := s.coll.DeleteOne(ctx, bson.M{
		"_id":      req.SessionID,
		"app_name": req.AppName,
		"user_id":  req.UserID,
	})
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	// SPEC-069 问题 4: cascade-delete the independent raw event stream. Best
	// effort — a leftover event doc only affects display, never correctness.
	if _, err := s.evtColl.DeleteMany(ctx, bson.M{"session_id": req.SessionID}); err != nil {
		log.Printf("[session] delete session events %s: %v", req.SessionID, err)
	}
	return nil
}

// AppendEvent appends an event. Streaming text chunks are buffered and flushed
// as one complete message to the independent session_events collection when
// the invocation changes or a non-text event arrives. This guarantees one LLM
// response = one record, making limit queries and compaction accurate.
func (s *Service) AppendEvent(ctx context.Context, sess session.Session, event *session.Event) error {
	if event.ID == "" {
		event.ID = "evt_" + uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	ms, _ := sess.(*mongoSession)
	isPartial := isStreamingChunk(event)
	isContinuation := isPartial && ms != nil &&
		len(ms.doc.Events) > 0 &&
		ms.doc.Events[len(ms.doc.Events)-1].Author == event.Author &&
		isStreamingChunk(ms.doc.Events[len(ms.doc.Events)-1])

	update := bson.M{"$set": bson.M{"updated_at": time.Now()}}
	if isContinuation {
		mergedEvent := mergeTextIntoEvent(ms.doc.Events[len(ms.doc.Events)-1], event)
		update["$set"].(bson.M)["events.$[elem]"] = mergedEvent
	} else {
		update["$push"] = bson.M{"events": event}
	}

	for k, v := range event.Actions.StateDelta {
		if strings.Contains(k, ".") || strings.HasPrefix(k, "$") {
			continue
		}
		update["$set"].(bson.M)["state."+k] = v
	}

	opts := options.Update()
	if isContinuation {
		opts.SetArrayFilters(options.ArrayFilters{Filters: []interface{}{
			bson.M{"elem.id": ms.doc.Events[len(ms.doc.Events)-1].ID},
		}})
	}

	// ---- raw_events (SPEC-069 问题 4): streaming partial chunks are buffered
	// and merged into a single message; everything else (non-streaming, and
	// the stream's final non-partial event) is inserted into the independent
	// session_events collection — one event per document. This keeps one LLM
	// turn = one record and removes the 16MB/整体读 problem of the legacy
	// array field. ----
	var rawEvent *session.Event
	if isPartial {
		s.bufferChunk(sess.ID(), event)
		s.flushBufferIfLarge(ctx, sess)
	} else {
		s.flushBuffer(ctx, sess)
		if event.Content != nil {
			rawEvent = event
		}
	}

	// SPEC-069 问题 2 并发安全：events 落库与 maybeCompact 的 $set events
	//（整体替换）共用同一把锁，保证「读 events → 算 cut → $set events」与
	// append 互斥，避免 compaction 覆盖并发 append 丢失事件。
	s.mu.Lock()
	res, err := s.coll.UpdateOne(ctx,
		bson.M{"_id": sess.ID(), "app_name": sess.AppName(), "user_id": sess.UserID()},
		update, opts)
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("session %q not found", sess.ID())
	}

	// Raw event goes to the independent collection after the events write
	// succeeds; a raw-write failure only degrades transcript display, never
	// the session's LLM context (events).
	if rawEvent != nil {
		if err := s.appendRawEvent(ctx, sess, rawEvent); err != nil {
			log.Printf("[session] append raw event %s: %v", rawEvent.ID, err)
		}
	}

	syncSnapshot(sess, event, isPartial)

	// Compaction is triggered only by user messages and tool outputs
	// (FunctionResponse). system and assistant text events are recorded and
	// compacted normally but never trigger compaction.
	if s.summarizer != nil && shouldCompact(event) {
		return s.maybeCompact(ctx, sess)
	}
	return nil
}

// shouldCompact reports whether appending this event should trigger a
// compaction check. Only user messages and tool outputs qualify. Tool calls
// (FunctionCall) are assistant (role=model) turns and do NOT trigger; the
// tool result (FunctionResponse) that follows them does.
func shouldCompact(event *session.Event) bool {
	if event == nil || event.Author == "compaction" || event.Author == "system" {
		return false
	}
	if event.Author == "user" {
		return true
	}
	if event.Content != nil {
		for _, p := range event.Content.Parts {
			if p != nil && p.FunctionResponse != nil {
				return true
			}
		}
	}
	return false
}

// appendRawEvent inserts one raw (never-compacted) event into the independent
// session_events collection with a monotonic seq (SPEC-069 问题 4). Seq is a
// UnixNano timestamp — monotonic across restarts, no counter coordination
// needed; a nanosecond collision (two concurrent appends) hits the unique
// index and retries once with seq+1.
func (s *Service) appendRawEvent(ctx context.Context, sess session.Session, event *session.Event) error {
	seq := time.Now().UnixNano()
	doc := &sessionEventDoc{
		ID:        "sessevt_" + uuid.New().String(),
		SessionID: sess.ID(),
		AppName:   sess.AppName(),
		UserID:    sess.UserID(),
		Seq:       seq,
		Event:     event,
		CreatedAt: time.Now(),
	}
	_, err := s.evtColl.InsertOne(ctx, doc)
	if err != nil && mongo.IsDuplicateKeyError(err) {
		doc.ID = "sessevt_" + uuid.New().String()
		doc.Seq = seq + 1
		_, err = s.evtColl.InsertOne(ctx, doc)
	}
	if err != nil {
		return fmt.Errorf("insert session event: %w", err)
	}
	return nil
}

// ensurePush was used to batch raw_events into the session document update;
// removed with SPEC-069 问题 4 (raw events now live in session_events).


func (s *Service) bufferChunk(sessionID string, event *session.Event) {
	if event.Content == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.buf[sessionID]
	if !ok {
		b = &chunkBuffer{
			author:  event.Author,
			eventID: event.ID,
			since:   event.Timestamp,
		}
		s.buf[sessionID] = b
	}
	for _, p := range event.Content.Parts {
		if p != nil && p.Text != "" {
			b.text.WriteString(p.Text)
			b.size += len(p.Text)
		}
	}
}

// flushBufferIfLarge flushes the session's buffered streaming text once it
// exceeds the threshold, as a memory-safety backstop. The normal flush path is
// the stream completing (next non-partial event) via flushBuffer.
func (s *Service) flushBufferIfLarge(ctx context.Context, sess session.Session) {
	s.mu.Lock()
	b, ok := s.buf[sess.ID()]
	over := ok && b.size >= flushThresholdBytes
	s.mu.Unlock()
	if over {
		s.flushBuffer(ctx, sess)
	}
}

func (s *Service) flushBuffer(ctx context.Context, sess session.Session) {
	s.mu.Lock()
	b, ok := s.buf[sess.ID()]
	if !ok || b.text.Len() == 0 {
		s.mu.Unlock()
		return
	}
	delete(s.buf, sess.ID())
	s.mu.Unlock()

	event := &session.Event{
		ID:        b.eventID,
		Timestamp: b.since,
		Author:    b.author,
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{
				Role:  "model",
				Parts: []*genai.Part{{Text: b.text.String()}},
			},
		},
	}
	// SPEC-069 问题 4: the merged stream event goes to session_events, not the
	// legacy raw_events array.
	if err := s.appendRawEvent(ctx, sess, event); err != nil {
		log.Printf("[session] flush buffered raw event: %v", err)
	}
}

func mergeTextIntoEvent(prev, next *session.Event) *session.Event {
	merged := *prev
	merged.Timestamp = next.Timestamp
	if prev.Content == nil {
		merged.Content = next.Content
		return &merged
	}
	var text string
	for _, p := range prev.Content.Parts {
		if p != nil {
			text += p.Text
		}
	}
	for _, p := range next.Content.Parts {
		if p != nil {
			text += p.Text
		}
	}
	role := prev.Content.Role
	if role == "" {
		role = "model"
	}
	merged.Content = &genai.Content{
		Role:  role,
		Parts: []*genai.Part{{Text: text}},
	}
	return &merged
}

func syncSnapshot(sess session.Session, event *session.Event, isPartial bool) {
	ms, ok := sess.(*mongoSession)
	if !ok {
		return
	}
	last := &session.Event{}
	if len(ms.doc.Events) > 0 {
		last = ms.doc.Events[len(ms.doc.Events)-1]
	}
	if last.Author == event.Author && isPartial && isStreamingChunk(last) {
		ms.doc.Events[len(ms.doc.Events)-1] = mergeTextIntoEvent(last, event)
	} else {
		ms.doc.Events = append(ms.doc.Events, event)
	}
	if !isPartial {
		ms.doc.RawEvents = append(ms.doc.RawEvents, event)
	}
	for k, v := range event.Actions.StateDelta {
		if strings.Contains(k, ".") || strings.HasPrefix(k, "$") {
			continue
		}
		ms.doc.State[k] = v
	}
	ms.doc.UpdatedAt = time.Now()
}

func (s *Service) maybeCompact(ctx context.Context, sess session.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc, err := s.find(ctx, sess.AppName(), sess.UserID(), sess.ID())
	if err != nil {
		return err
	}
	cfg := s.compact
	maxTokens := cfg.MaxTokens
	if cfg.MaxTokensFn != nil {
		if v := cfg.MaxTokensFn(ctx); v > 0 {
			maxTokens = v
		}
	}
	overCount := len(doc.Events) > cfg.MaxEvents
	overTokens := estimateEventTokens(doc.Events) > maxTokens
	if !overCount && !overTokens {
		return nil
	}
	if len(doc.Events) <= cfg.KeepRecent+1 {
		return nil
	}

	cut := len(doc.Events) - cfg.KeepRecent
	cut = adjustCutForDanglingCalls(doc.Events, cut)
	oldEvents := doc.Events[:cut]
	summary, err := s.summarizer.Summarize(ctx, oldEvents)
	if err != nil {
		return fmt.Errorf("compact session %q: %w", sess.ID(), err)
	}

	compactionEvent := &session.Event{
		ID:        "evt_" + uuid.New().String(),
		Timestamp: time.Now(),
		Author:    "compaction",
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{Role: "system", Parts: []*genai.Part{{Text: "[conversation summary] " + summary}}},
		},
	}
	newEvents := make([]*session.Event, 0, cfg.KeepRecent+1)
	newEvents = append(newEvents, compactionEvent)
	newEvents = append(newEvents, doc.Events[cut:]...)

	// SPEC-069 问题 3: summary 语义拆分。summary 事件（含摘要内容）只进
	// events（LLM 下一轮上下文）；raw_events 只收轻量压缩提示（无摘要内容），
	// 前端直接展示，不再需要 IsCompactionEvent 转轻提示。
	_, err = s.coll.UpdateOne(ctx,
		bson.M{"_id": sess.ID()},
		bson.M{
			"$set": bson.M{
				"events":     newEvents,
				"updated_at": time.Now(),
			},
		},
	)
	if err != nil {
		return fmt.Errorf("rewrite compacted events: %w", err)
	}
	notice := &session.Event{
		ID:        "evt_" + uuid.New().String(),
		Timestamp: time.Now(),
		Author:    "compaction",
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{Role: "system", Parts: []*genai.Part{{Text: "[compaction] 上下文已自动压缩"}}},
		},
	}
	if err := s.appendRawEvent(ctx, sess, notice); err != nil {
		log.Printf("[session] append compaction notice: %v", err)
	}

	if ms, ok := sess.(*mongoSession); ok {
		ms.doc.Events = newEvents
	}
	return nil
}

// adjustCutForDanglingCalls applies the 方案 C boundary rule: the compaction
// cut must not fall in the middle of an in-progress tool chain. A FunctionCall
// whose FunctionResponse has not arrived yet (dangling call, async tools) must
// stay in the retained range, otherwise the late response loses its pairing
// call (ADK reports "no function call event found", or worse, silently drops
// the tool result). Returns cut moved back to the latest dangling call event
// when necessary (SPEC-069 problem 2).
func adjustCutForDanglingCalls(events []*session.Event, cut int) int {
	if idx := latestDanglingCallIndex(events); idx >= 0 && idx < cut {
		return idx
	}
	return cut
}

// latestDanglingCallIndex returns the index of the latest event containing a
// FunctionCall whose FunctionResponse has not yet appeared in the event list
// (a "dangling call"), or -1 when every call is paired. The compaction cut
// must never fall between a call event and its response(s), so the boundary
// is moved back to this index when needed (SPEC-069 problem 2, 方案 C).
//
// Pairing is event-granularity (one call event may hold multiple
// FunctionCall parts whose responses arrive in separate events): a call ID is
// considered dangling when no response event with the same ID exists anywhere
// in the list.
func latestDanglingCallIndex(events []*session.Event) int {
	hasResponse := make(map[string]bool)
	for _, ev := range events {
		if ev == nil || ev.Content == nil {
			continue
		}
		for _, p := range ev.Content.Parts {
			if p != nil && p.FunctionResponse != nil && p.FunctionResponse.ID != "" {
				hasResponse[p.FunctionResponse.ID] = true
			}
		}
	}
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev == nil || ev.Content == nil {
			continue
		}
		for _, p := range ev.Content.Parts {
			if p != nil && p.FunctionCall != nil && p.FunctionCall.ID != "" && !hasResponse[p.FunctionCall.ID] {
				return i
			}
		}
	}
	return -1
}

func (s *Service) find(ctx context.Context, appName, userID, sessionID string) (*sessionDoc, error) {	var doc sessionDoc
	err := s.coll.FindOne(ctx, bson.M{
		"_id":      sessionID,
		"app_name": appName,
		"user_id":  userID,
	}).Decode(&doc)
	if err != nil {
		return nil, fmt.Errorf("session %q not found: %w", sessionID, err)
	}
	return &doc, nil
}

func (s *Service) DisplayEvents(ctx context.Context, appName, userID, sessionID string, limit int) ([]*session.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	// SPEC-069 问题 4: read from the independent session_events collection
	// with DB-level truncation (sort by seq desc, limit N), then reverse back
	// to chronological order. No whole-document read, no in-memory truncation.
	filter := bson.M{"session_id": sessionID, "app_name": appName, "user_id": userID}
	cur, err := s.evtColl.Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "seq", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("list session events: %w", err)
	}
	var docs []sessionEventDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("decode session events: %w", err)
	}
	if len(docs) > 0 {
		events := make([]*session.Event, 0, len(docs))
		for i := len(docs) - 1; i >= 0; i-- { // reverse back to chronological
			if docs[i].Event != nil {
				events = append(events, docs[i].Event)
			}
		}
		return events, nil
	}
	// Fallback for pre-SPEC-069 sessions: the legacy raw_events array, then
	// the compacted events array.
	doc, err := s.find(ctx, appName, userID, sessionID)
	if err != nil {
		return nil, err
	}
	events := doc.RawEvents
	if len(events) == 0 {
		events = doc.Events
	}
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events, nil
}

func (d *sessionDoc) toSession() session.Session {
	return &mongoSession{doc: d}
}

type mongoSession struct {
	doc *sessionDoc
}

func (s *mongoSession) ID() string               { return s.doc.ID }
func (s *mongoSession) AppName() string           { return s.doc.AppName }
func (s *mongoSession) UserID() string            { return s.doc.UserID }
func (s *mongoSession) Events() session.Events    { return eventsView(s.doc.Events) }
func (s *mongoSession) State() session.State      { return mapState(s.doc.State) }
func (s *mongoSession) SetStateDelta(k, v string) { s.doc.State[k] = v }
func (s *mongoSession) CreationTime() time.Time   { return s.doc.UpdatedAt }
func (s *mongoSession) LastUpdateTime() time.Time { return s.doc.UpdatedAt }

type mapState map[string]any

func (m mapState) Get(key string) (any, error) {
	v, ok := m[key]
	if !ok {
		return nil, fmt.Errorf("state key %q not found", key)
	}
	return v, nil
}
func (m mapState) Set(key string, value any) error { m[key] = value; return nil }
func (m mapState) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for k, v := range m {
			if !yield(k, v) {
				return
			}
		}
	}
}

type eventsView []*session.Event

func (e eventsView) All() iter.Seq[*session.Event] {
	return func(yield func(*session.Event) bool) {
		for _, ev := range e {
			if !yield(ev) {
				return
			}
		}
	}
}
func (e eventsView) Len() int              { return len(e) }
func (e eventsView) At(i int) *session.Event { return e[i] }

// estimateEventTokens estimates the token count of a set of events. It covers
// text parts plus tool content: FunctionCall (Name + Args) and
// FunctionResponse (Response), serialized to JSON and estimated at len/3 —
// the same lightweight heuristic as text, with no tokenizer dependency
// (SPEC-069 problem 1).
func estimateEventTokens(events []*session.Event) int {
	n := 0
	for _, ev := range events {
		if ev.Content == nil {
			continue
		}
		for _, p := range ev.Content.Parts {
			if p == nil {
				continue
			}
			switch {
			case p.Text != "":
				n += len(p.Text) / 3
			case p.FunctionCall != nil:
				raw, err := json.Marshal(map[string]any{
					"name": p.FunctionCall.Name,
					"args": p.FunctionCall.Args,
				})
				if err == nil {
					n += len(raw) / 3
				}
			case p.FunctionResponse != nil:
				raw, err := json.Marshal(p.FunctionResponse.Response)
				if err == nil {
					n += len(raw) / 3
				}
			}
		}
	}
	return n
}

// isStreamingChunk reports whether the event is an intermediate chunk of a
// streaming model turn. ADK sets LLMResponse.Partial=true on such chunks; the
// final aggregated event of a turn (and every non-streaming event) is
// Partial=false. The store keys its buffering decision solely on this flag —
// it must not interpret LLM-specific content such as function calls, finish
// reasons, code execution, or media parts (that would leak LLM semantics into
// the storage layer and break single responsibility).
func isStreamingChunk(ev *session.Event) bool {
	return ev != nil && ev.LLMResponse.Partial
}
