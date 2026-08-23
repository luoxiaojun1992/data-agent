package adksession

import (
	"context"
	"fmt"
	"iter"
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

type sessionDoc struct {
	ID        string           `bson:"_id"`
	AppName   string           `bson:"app_name"`
	UserID    string           `bson:"user_id"`
	State     map[string]any   `bson:"state"`
	Events    []*session.Event `bson:"events"`
	RawEvents []*session.Event `bson:"raw_events"`
	UpdatedAt time.Time        `bson:"updated_at"`
}

// chunkBuffer accumulates streaming text for one invocation before flushing.
type chunkBuffer struct {
	invokeID string
	author   string
	eventID  string
	since    time.Time
	text     strings.Builder
}

type Service struct {
	coll       *mongo.Collection
	mu         sync.Mutex
	summarizer Summarizer
	compact    CompactionConfig
	// buf accumulates the in-progress streaming text for each session (one
	// complete message per session). Access is guarded by mu.
	buf map[string]*chunkBuffer
}

func NewService(db *mongo.Database) *Service {
	return &Service{coll: db.Collection(CollectionName), buf: make(map[string]*chunkBuffer)}
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
	return nil
}

// AppendEvent appends an event. Streaming text chunks are buffered and flushed
// as one complete message to raw_events when the invocation changes or a
// non-text event arrives. This guarantees one LLM response = one DB record,
// making limit queries and compaction accurate.
func (s *Service) AppendEvent(ctx context.Context, sess session.Session, event *session.Event) error {
	if event.ID == "" {
		event.ID = "evt_" + uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	ms, _ := sess.(*mongoSession)
	isTextChunk := isStreamingTextChunk(event)
	isContinuation := isTextChunk && ms != nil &&
		len(ms.doc.Events) > 0 &&
		ms.doc.Events[len(ms.doc.Events)-1].InvocationID == event.InvocationID &&
		ms.doc.Events[len(ms.doc.Events)-1].Author == event.Author &&
		isStreamingTextChunk(ms.doc.Events[len(ms.doc.Events)-1])

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

	// ---- raw_events: user/system/tool/non-text go directly; only assistant
	// streaming text chunks are buffered. system events (guard [intent] etc.)
	// must NOT be buffered — they are discrete messages, and buffering them
	// would either interleave or clobber the in-progress assistant reply. ----
	if event.Author == "user" || event.Author == "system" || event.Content == nil || !isTextChunk {
		s.flushBuffer(ctx, sess)
		update["$push"] = ensurePush(update, "raw_events", event)
	} else {
		s.bufferChunk(sess.ID(), event)
	}

	res, err := s.coll.UpdateOne(ctx,
		bson.M{"_id": sess.ID(), "app_name": sess.AppName(), "user_id": sess.UserID()},
		update, opts)
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("session %q not found", sess.ID())
	}

	syncSnapshot(sess, event, isTextChunk)

	// Compaction triggered only by user messages and tool messages
	// (FunctionCall / FunctionResponse). system and assistant text events are
	// recorded and compacted normally but never trigger compaction.
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

func ensurePush(update bson.M, key string, event *session.Event) bson.M {
	if _, ok := update["$push"]; !ok {
		update["$push"] = bson.M{}
	}
	update["$push"].(bson.M)[key] = event
	return update["$push"].(bson.M)
}

func (s *Service) bufferChunk(sessionID string, event *session.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.buf[sessionID]
	if !ok {
		b = &chunkBuffer{
			invokeID: event.InvocationID,
			author:   event.Author,
			eventID:  event.ID,
			since:    event.Timestamp,
		}
		s.buf[sessionID] = b
	}
	for _, p := range event.Content.Parts {
		if p.Text != "" {
			b.text.WriteString(p.Text)
		}
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
		ID:           b.eventID,
		Timestamp:    b.since,
		InvocationID: b.invokeID,
		Author:       b.author,
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{
				Role:  "model",
				Parts: []*genai.Part{{Text: b.text.String()}},
			},
		},
	}
	_, _ = s.coll.UpdateOne(ctx,
		bson.M{"_id": sess.ID(), "app_name": sess.AppName(), "user_id": sess.UserID()},
		bson.M{
			"$push": bson.M{"raw_events": event},
			"$set":  bson.M{"updated_at": time.Now()},
		},
	)
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

func syncSnapshot(sess session.Session, event *session.Event, isTextChunk bool) {
	ms, ok := sess.(*mongoSession)
	if !ok {
		return
	}
	last := &session.Event{}
	if len(ms.doc.Events) > 0 {
		last = ms.doc.Events[len(ms.doc.Events)-1]
	}
	if last.InvocationID == event.InvocationID && last.Author == event.Author &&
		isStreamingTextChunk(event) && isStreamingTextChunk(last) {
		ms.doc.Events[len(ms.doc.Events)-1] = mergeTextIntoEvent(last, event)
	} else {
		ms.doc.Events = append(ms.doc.Events, event)
	}
	if !isTextChunk {
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

	_, err = s.coll.UpdateOne(ctx,
		bson.M{"_id": sess.ID()},
		bson.M{
			"$set": bson.M{
				"events":     newEvents,
				"updated_at": time.Now(),
			},
			"$push": bson.M{
				"raw_events": compactionEvent,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("rewrite compacted events: %w", err)
	}

	if ms, ok := sess.(*mongoSession); ok {
		ms.doc.Events = newEvents
		ms.doc.RawEvents = append(ms.doc.RawEvents, compactionEvent)
	}
	return nil
}

func (s *Service) find(ctx context.Context, appName, userID, sessionID string) (*sessionDoc, error) {
	var doc sessionDoc
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

func estimateEventTokens(events []*session.Event) int {
	n := 0
	for _, ev := range events {
		if ev.Content == nil {
			continue
		}
		for _, p := range ev.Content.Parts {
			if p.Text != "" {
				n += len(p.Text) / 3
			}
		}
	}
	return n
}

func isStreamingTextChunk(ev *session.Event) bool {
	if ev.Content == nil {
		return false
	}
	for _, p := range ev.Content.Parts {
		if p == nil {
			continue
		}
		if p.FunctionCall != nil || p.FunctionResponse != nil ||
			p.ExecutableCode != nil || p.CodeExecutionResult != nil ||
			p.InlineData != nil || p.FileData != nil {
			return false
		}
	}
	return true
}
