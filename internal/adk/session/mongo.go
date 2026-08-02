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

type Service struct {
	coll *mongo.Collection
	mu         sync.Mutex
	summarizer Summarizer
	compact    CompactionConfig
}

func NewService(db *mongo.Database) *Service {
	return &Service{coll: db.Collection(CollectionName)}
}

type Summarizer interface {
	Summarize(ctx context.Context, events []*session.Event) (string, error)
}

type CompactionConfig struct {
	MaxEvents  int
	MaxTokens  int
	KeepRecent int
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

// AppendEvent appends an event. Streaming text chunks from the same invocation
// are merged into the last event in both events and raw_events — one LLM
// response = one DB record in both arrays.
func (s *Service) AppendEvent(ctx context.Context, sess session.Session, event *session.Event) error {
	if event.ID == "" {
		event.ID = "evt_" + uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	ms, _ := sess.(*mongoSession)
	isStreamContinuation := ms != nil &&
		len(ms.doc.Events) > 0 &&
		ms.doc.Events[len(ms.doc.Events)-1].InvocationID == event.InvocationID &&
		ms.doc.Events[len(ms.doc.Events)-1].Author == event.Author &&
		isStreamingTextChunk(event) &&
		isStreamingTextChunk(ms.doc.Events[len(ms.doc.Events)-1])

	update := bson.M{"$set": bson.M{"updated_at": time.Now()}}
	filter := bson.M{"_id": sess.ID(), "app_name": sess.AppName(), "user_id": sess.UserID()}

	if isStreamContinuation {
		// Merge text into last events and raw_events elements in-place.
		last := ms.doc.Events[len(ms.doc.Events)-1]
		for _, p := range event.Content.Parts {
			if p.Text != "" {
				for _, lp := range last.Content.Parts {
					if lp.Text != "" {
						lp.Text += p.Text
						break
					}
				}
			}
		}
		last.Timestamp = event.Timestamp
		update["$set"].(bson.M)["events.$[elem]"] = last

		// Also update raw_events if the previous event is there
		if len(ms.doc.RawEvents) > 0 &&
			ms.doc.RawEvents[len(ms.doc.RawEvents)-1].InvocationID == event.InvocationID {
			lastRaw := ms.doc.RawEvents[len(ms.doc.RawEvents)-1]
			for _, p := range event.Content.Parts {
				if p.Text != "" {
					for _, lp := range lastRaw.Content.Parts {
						if lp.Text != "" {
							lp.Text += p.Text
							break
						}
					}
				}
			}
			lastRaw.Timestamp = event.Timestamp
			update["$set"].(bson.M)["raw_events.$[elem2]"] = lastRaw
		}
	} else {
		update["$push"] = bson.M{"events": event}
		if event.Author == "user" || event.Content == nil || !isStreamingTextChunk(event) {
			update["$push"].(bson.M)["raw_events"] = event
		}
	}

	for k, v := range event.Actions.StateDelta {
		if strings.Contains(k, ".") || strings.HasPrefix(k, "$") {
			continue
		}
		update["$set"].(bson.M)["state."+k] = v
	}

	opts := options.Update()
	if _, ok := update["$set"].(bson.M)["events.$[elem]"]; ok {
		af := options.ArrayFilters{Filters: []interface{}{
			bson.M{"elem.id": ms.doc.Events[len(ms.doc.Events)-1].ID},
		}}
		if _, ok2 := update["$set"].(bson.M)["raw_events.$[elem2]"]; ok2 && len(ms.doc.RawEvents) > 0 {
			af.Filters = append(af.Filters, bson.M{"elem2.id": ms.doc.RawEvents[len(ms.doc.RawEvents)-1].ID})
		}
		opts.SetArrayFilters(af)
	}

	res, err := s.coll.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("session %q not found", sess.ID())
	}

	syncSnapshot(sess, event)

	if s.summarizer != nil {
		return s.maybeCompact(ctx, sess)
	}
	return nil
}

func syncSnapshot(sess session.Session, event *session.Event) {
	ms, ok := sess.(*mongoSession)
	if !ok {
		return
	}
	// If streaming continuation, text was merged in-place above — don't append.
	last := &session.Event{}
	if len(ms.doc.Events) > 0 {
		last = ms.doc.Events[len(ms.doc.Events)-1]
	}
	if last.InvocationID == event.InvocationID && last.Author == event.Author &&
		isStreamingTextChunk(event) && isStreamingTextChunk(last) {
		// already merged
	} else {
		ms.doc.Events = append(ms.doc.Events, event)
	}
	// RawEvents: append non-streaming events
	if event.Author == "user" || event.Content == nil || !isStreamingTextChunk(event) {
		lastRaw := &session.Event{}
		if len(ms.doc.RawEvents) > 0 {
			lastRaw = ms.doc.RawEvents[len(ms.doc.RawEvents)-1]
		}
		if lastRaw.InvocationID == event.InvocationID && lastRaw.Author == event.Author &&
			isStreamingTextChunk(event) && isStreamingTextChunk(lastRaw) {
			// merged
		} else {
			ms.doc.RawEvents = append(ms.doc.RawEvents, event)
		}
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
	overCount := len(doc.Events) > cfg.MaxEvents
	overTokens := estimateEventTokens(doc.Events) > cfg.MaxTokens
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
			Content: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "[conversation summary] " + summary}}},
		},
	}
	newEvents := make([]*session.Event, 0, cfg.KeepRecent+1)
	newEvents = append(newEvents, compactionEvent)
	newEvents = append(newEvents, doc.Events[cut:]...)

	_, err = s.coll.UpdateOne(ctx,
		bson.M{"_id": sess.ID()},
		bson.M{"$set": bson.M{
			"events":     newEvents,
			"updated_at": time.Now(),
		}},
	)
	if err != nil {
		return fmt.Errorf("rewrite compacted events: %w", err)
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
