// Package adksession provides a MongoDB-backed implementation of the ADK
// session.Service, plus optional token/event-threshold compaction that
// summarizes old events via an LLM to keep context windows bounded.
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

// CollectionName is the MongoDB collection used to persist ADK sessions.
const CollectionName = "adk_sessions"

// sessionDoc is the MongoDB document layout for an ADK session.
type sessionDoc struct {
	ID        string           `bson:"_id"`
	AppName   string           `bson:"app_name"`
	UserID    string           `bson:"user_id"`
	State     map[string]any   `bson:"state"`
	Events    []*session.Event `bson:"events"`
	UpdatedAt time.Time        `bson:"updated_at"`
}

// Service implements session.Service on top of MongoDB.
type Service struct {
	coll *mongo.Collection

	mu         sync.Mutex // serializes compaction per process
	summarizer Summarizer
	compact    CompactionConfig
}

// NewService creates a MongoDB-backed session.Service.
func NewService(db *mongo.Database) *Service {
	return &Service{coll: db.Collection(CollectionName)}
}

// WithCompaction enables event compaction: after each AppendEvent, when the
// number of events exceeds cfg.MaxEvents (or their estimated tokens exceed
// cfg.MaxTokens), the oldest events are summarized into a single compaction
// event by the given Summarizer, keeping the most recent cfg.KeepRecent events.
func (s *Service) WithCompaction(cfg CompactionConfig, summarizer Summarizer) *Service {
	s.compact = cfg.withDefaults()
	s.summarizer = summarizer
	return s
}

// ---- session.Service implementation ----

// Create persists a new session with the given initial state.
func (s *Service) Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error) {
	id := req.SessionID
	if id == "" {
		id = "sess_" + uuid.New().String()
	}
	doc := &sessionDoc{
		ID:        id,
		AppName:   req.AppName,
		UserID:    req.UserID,
		State:     map[string]any{},
		Events:    []*session.Event{},
		UpdatedAt: time.Now(),
	}
	for k, v := range req.State {
		doc.State[k] = v
	}

	// Idempotent create: upsert with $setOnInsert so duplicate creates are no-ops.
	_, err := s.coll.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$setOnInsert": doc},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &session.CreateResponse{Session: doc.toSession()}, nil
}

// Get loads a session, applying optional event filters.
func (s *Service) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	doc, err := s.find(ctx, req.AppName, req.UserID, req.SessionID)
	if err != nil {
		return nil, err
	}

	events := doc.Events
	if !req.After.IsZero() {
		filtered := make([]*session.Event, 0, len(events))
		for _, e := range events {
			if !e.Timestamp.Before(req.After) {
				filtered = append(filtered, e)
			}
		}
		events = filtered
	}
	if req.NumRecentEvents > 0 && len(events) > req.NumRecentEvents {
		events = events[len(events)-req.NumRecentEvents:]
	}
	doc.Events = events
	return &session.GetResponse{Session: doc.toSession()}, nil
}

// List returns all sessions of a user within an app.
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

// Delete removes a session. Deleting a non-existent session is a no-op (idempotent).
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

// AppendEvent appends an event to the session, merges its state delta,
// and triggers compaction when configured thresholds are exceeded.
func (s *Service) AppendEvent(ctx context.Context, sess session.Session, event *session.Event) error {
	if event.ID == "" {
		event.ID = "evt_" + uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	update := bson.M{
		"$push": bson.M{"events": event},
		"$set":  bson.M{"updated_at": time.Now()},
	}
	setFields := update["$set"].(bson.M)
	for k, v := range event.Actions.StateDelta {
		if strings.Contains(k, ".") || strings.HasPrefix(k, "$") {
			continue // unsafe Mongo key — skip rather than corrupt the document
		}
		setFields["state."+k] = v
	}

	res, err := s.coll.UpdateOne(ctx,
		bson.M{"_id": sess.ID(), "app_name": sess.AppName(), "user_id": sess.UserID()},
		update,
	)
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("session %q not found", sess.ID())
	}

	// Keep the caller's in-memory snapshot in sync — the ADK runner builds
	// LLM request contents from session.Events() of the object it holds.
	if ms, ok := sess.(*mongoSession); ok {
		ms.doc.Events = append(ms.doc.Events, event)
		for k, v := range event.Actions.StateDelta {
			if strings.Contains(k, ".") || strings.HasPrefix(k, "$") {
				continue
			}
			ms.doc.State[k] = v
		}
		ms.doc.UpdatedAt = time.Now()
	}

	if s.summarizer != nil {
		return s.maybeCompact(ctx, sess)
	}
	return nil
}

// ---- compaction ----

// CompactionConfig controls when and how old events are summarized.
type CompactionConfig struct {
	// MaxEvents triggers compaction when total events exceed this count.
	MaxEvents int
	// MaxTokens triggers compaction when estimated total tokens exceed this budget.
	MaxTokens int
	// KeepRecent is the number of most recent events preserved verbatim.
	KeepRecent int
}

func (c CompactionConfig) withDefaults() CompactionConfig {
	if c.MaxEvents <= 0 {
		c.MaxEvents = 100
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 4000
	}
	if c.KeepRecent <= 0 {
		c.KeepRecent = 20
	}
	return c
}

// Summarizer compresses a list of old events into a single summary text.
type Summarizer interface {
	Summarize(ctx context.Context, events []*session.Event) (string, error)
}

// maybeCompact summarizes old events when thresholds are exceeded.
func (s *Service) maybeCompact(ctx context.Context, sess session.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc, err := s.find(ctx, sess.AppName(), sess.UserID(), sess.ID())
	if err != nil {
		return err
	}

	cfg := s.compact
	overCount := len(doc.Events) > cfg.MaxEvents
	overTokens := estimateEventTokens(doc.Events) > cfg.MaxTokens
	if !overCount && !overTokens {
		return nil
	}
	if len(doc.Events) <= cfg.KeepRecent+1 {
		return nil // not enough history to compact
	}

	cut := len(doc.Events) - cfg.KeepRecent
	oldEvents := doc.Events[:cut]
	// Skip leading compaction events from re-summarization input ordering;
	// they are included so the summary stays cumulative.
	summary, err := s.summarizer.Summarize(ctx, oldEvents)
	if err != nil {
		return fmt.Errorf("compact session %q: %w", sess.ID(), err)
	}

	compactionEvent := &session.Event{
		ID:        "evt_" + uuid.New().String(),
		Timestamp: time.Now(),
		Author:    "compaction",
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "[conversation summary] " + summary}}},
		},
	}
	newEvents := make([]*session.Event, 0, cfg.KeepRecent+1)
	newEvents = append(newEvents, compactionEvent)
	newEvents = append(newEvents, doc.Events[cut:]...)

	_, err = s.coll.UpdateOne(ctx,
		bson.M{"_id": sess.ID()},
		bson.M{"$set": bson.M{"events": newEvents, "updated_at": time.Now()}},
	)
	if err != nil {
		return fmt.Errorf("rewrite compacted events: %w", err)
	}
	return nil
}

// estimateEventTokens roughly estimates tokens as ~4 chars per token.
func estimateEventTokens(events []*session.Event) int {
	total := 0
	for _, e := range events {
		if e == nil || e.Content == nil {
			continue
		}
		for _, p := range e.Content.Parts {
			if p != nil {
				total += (len(p.Text) + 3) / 4
			}
		}
	}
	return total
}

// ---- helpers ----

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

// toSession converts a stored document into a session.Session view.
func (d *sessionDoc) toSession() session.Session {
	return &mongoSession{doc: d}
}

// mongoSession implements session.Session over a document snapshot.
type mongoSession struct {
	doc *sessionDoc
}

func (m *mongoSession) ID() string                { return m.doc.ID }
func (m *mongoSession) AppName() string           { return m.doc.AppName }
func (m *mongoSession) UserID() string            { return m.doc.UserID }
func (m *mongoSession) LastUpdateTime() time.Time { return m.doc.UpdatedAt }
func (m *mongoSession) State() session.State      { return &stateView{m: m.doc.State} }
func (m *mongoSession) Events() session.Events    { return eventsView(m.doc.Events) }

// stateView implements session.State over the in-memory snapshot.
// Set only mutates the snapshot; persistence happens on AppendEvent via StateDelta.
type stateView struct {
	m map[string]any
}

func (s *stateView) Get(key string) (any, error) {
	v, ok := s.m[key]
	if !ok {
		return nil, fmt.Errorf("state key %q: %w", key, session.ErrStateKeyNotExist)
	}
	return v, nil
}

func (s *stateView) Set(key string, value any) error {
	s.m[key] = value
	return nil
}

func (s *stateView) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for k, v := range s.m {
			if !yield(k, v) {
				return
			}
		}
	}
}

// eventsView implements session.Events over a slice snapshot.
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

func (e eventsView) Len() int { return len(e) }

func (e eventsView) At(i int) *session.Event { return e[i] }
