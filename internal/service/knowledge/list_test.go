package knowledge

import (
	"context"
	"errors"
	"testing"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// fakeKBRepo embeds the KBRepository interface and overrides only the list
// methods under test, keeping the other 11 methods satisfied via the embedded
// (nil) interface — they panic only if accidentally called.
type fakeKBRepo struct {
	repository.KBRepository
	listDocsFn        func(ctx context.Context, userID string, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error)
	listByVisFn       func(ctx context.Context, userID string, isSystemAdmin bool, q string, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error)
	listAllFn         func(ctx context.Context, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error)
}

func (f *fakeKBRepo) ListDocs(ctx context.Context, userID string, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error) {
	return f.listDocsFn(ctx, userID, skip, limit)
}

func (f *fakeKBRepo) ListDocsByVisibility(ctx context.Context, userID string, isSystemAdmin bool, q string, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error) {
	return f.listByVisFn(ctx, userID, isSystemAdmin, q, skip, limit)
}

func (f *fakeKBRepo) ListAllDocs(ctx context.Context, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error) {
	return f.listAllFn(ctx, skip, limit)
}

func TestService_ListDocs_ClampAndSkip(t *testing.T) {
	var gotSkip, gotLimit int64
	repo := &fakeKBRepo{listDocsFn: func(_ context.Context, _ string, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error) {
		gotSkip, gotLimit = skip, limit
		return []*knowledge.KnowledgeDoc{{ID: "d1"}}, int64(1), nil
	}}
	s := NewService(repo)

	// page=3, pageSize=10 → skip=20, limit=10
	if _, total, err := s.ListDocs("u1", 3, 10); err != nil || total != 1 {
		t.Fatalf("ListDocs: total=%d err=%v", total, err)
	}
	if gotSkip != 20 || gotLimit != 10 {
		t.Errorf("skip/limit = %d/%d, want 20/10", gotSkip, gotLimit)
	}
}

func TestService_ListDocs_ClampInvalid(t *testing.T) {
	var gotSkip, gotLimit int64
	repo := &fakeKBRepo{listDocsFn: func(_ context.Context, _ string, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error) {
		gotSkip, gotLimit = skip, limit
		return nil, 0, nil
	}}
	s := NewService(repo)
	// page=0, pageSize=500 → page=1, pageSize=20 → skip=0, limit=20
	if _, _, err := s.ListDocs("u1", 0, 500); err != nil {
		t.Fatalf("ListDocs: %v", err)
	}
	if gotSkip != 0 || gotLimit != 20 {
		t.Errorf("skip/limit = %d/%d, want 0/20", gotSkip, gotLimit)
	}
}

func TestService_ListDocs_Error(t *testing.T) {
	repo := &fakeKBRepo{listDocsFn: func(_ context.Context, _ string, _, _ int64) ([]*knowledge.KnowledgeDoc, int64, error) {
		return nil, 0, errors.New("db down")
	}}
	s := NewService(repo)
	if _, _, err := s.ListDocs("u1", 1, 20); err == nil {
		t.Error("expected error propagated from repo")
	}
}

func TestService_ListAllDocs_ClampAndSkip(t *testing.T) {
	var gotSkip, gotLimit int64
	repo := &fakeKBRepo{listAllFn: func(_ context.Context, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error) {
		gotSkip, gotLimit = skip, limit
		return []*knowledge.KnowledgeDoc{{ID: "d1"}}, int64(1), nil
	}}
	s := NewService(repo)
	if _, total, err := s.ListAllDocs(2, 5); err != nil || total != 1 {
		t.Fatalf("ListAllDocs: total=%d err=%v", total, err)
	}
	if gotSkip != 5 || gotLimit != 5 {
		t.Errorf("skip/limit = %d/%d, want 5/5", gotSkip, gotLimit)
	}
}

func TestService_ListAllDocs_ClampInvalid(t *testing.T) {
	var gotLimit int64
	repo := &fakeKBRepo{listAllFn: func(_ context.Context, _, limit int64) ([]*knowledge.KnowledgeDoc, int64, error) {
		gotLimit = limit
		return nil, 0, nil
	}}
	s := NewService(repo)
	if _, _, err := s.ListAllDocs(-1, 999); err != nil {
		t.Fatalf("ListAllDocs: %v", err)
	}
	if gotLimit != 20 {
		t.Errorf("limit = %d, want 20 (clamped)", gotLimit)
	}
}

func TestService_ListDocsByVisibility_Args(t *testing.T) {
	var gotUser string
	var gotAdmin bool
	var gotQ string
	var gotSkip, gotLimit int64
	repo := &fakeKBRepo{listByVisFn: func(_ context.Context, userID string, isSystemAdmin bool, q string, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error) {
		gotUser, gotAdmin, gotQ, gotSkip, gotLimit = userID, isSystemAdmin, q, skip, limit
		return []*knowledge.KnowledgeDoc{{ID: "d1"}}, int64(1), nil
	}}
	s := NewService(repo)
	if _, total, err := s.ListDocsByVisibility("u1", true, "销售", 2, 50); err != nil || total != 1 {
		t.Fatalf("ListDocsByVisibility: total=%d err=%v", total, err)
	}
	if gotUser != "u1" || !gotAdmin || gotQ != "销售" || gotSkip != 50 || gotLimit != 50 {
		t.Errorf("args = (%q,%v,%q,%d,%d), want (u1,true,销售,50,50)", gotUser, gotAdmin, gotQ, gotSkip, gotLimit)
	}
}

func TestService_ListDocsByVisibility_Clamp(t *testing.T) {
	var gotSkip, gotLimit int64
	repo := &fakeKBRepo{listByVisFn: func(_ context.Context, _ string, _ bool, _ string, skip, limit int64) ([]*knowledge.KnowledgeDoc, int64, error) {
		gotSkip, gotLimit = skip, limit
		return nil, 0, nil
	}}
	s := NewService(repo)
	// page=0 → 1; pageSize=999 → 20 → skip=0, limit=20
	if _, _, err := s.ListDocsByVisibility("u1", false, "", 0, 999); err != nil {
		t.Fatalf("ListDocsByVisibility: %v", err)
	}
	if gotSkip != 0 || gotLimit != 20 {
		t.Errorf("skip/limit = %d/%d, want 0/20 (clamped)", gotSkip, gotLimit)
	}
}

func TestService_ListDocsByVisibility_Error(t *testing.T) {
	repo := &fakeKBRepo{listByVisFn: func(_ context.Context, _ string, _ bool, _ string, _, _ int64) ([]*knowledge.KnowledgeDoc, int64, error) {
		return nil, 0, errors.New("db down")
	}}
	s := NewService(repo)
	if _, _, err := s.ListDocsByVisibility("u1", false, "", 1, 20); err == nil {
		t.Error("expected error propagated from repo")
	}
}
