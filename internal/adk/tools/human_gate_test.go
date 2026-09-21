package adktools

import (
	"context"
	"errors"
	"iter"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/session"

	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
	chatsvc "github.com/luoxiaojun1992/data-agent/internal/service/chat"
	skillsvc "github.com/luoxiaojun1992/data-agent/internal/service/skill"
)

// ---- test doubles ----

// fakeHumanGate is a minimal HumanGate double for testing the confirm/ask
// wiring in file_delete / dir_delete / ask_user.
type fakeHumanGate struct {
	confirmResult bool
	confirmErr    error
	askResult     string
	askErr        error
	confirmCalls  int
	askCalls      int
}

func (f *fakeHumanGate) Confirm(ctx context.Context, sessionID, hint string) (bool, error) {
	f.confirmCalls++
	return f.confirmResult, f.confirmErr
}

func (f *fakeHumanGate) Ask(ctx context.Context, sessionID, question string, options []string) (string, error) {
	f.askCalls++
	return f.askResult, f.askErr
}

// fakeSkillConfigRepo is a minimal SkillConfigRepository for testing the
// approval switch (SPEC-101). Get returns a preset config (or error).
type fakeSkillConfigRepo struct {
	getResult *skill.SkillConfig
	getErr    error
}

func (f *fakeSkillConfigRepo) List(ctx context.Context, skip, limit int64) ([]skill.SkillConfig, error) {
	return nil, nil
}
func (f *fakeSkillConfigRepo) Count(ctx context.Context) (int64, error) { return 0, nil }
func (f *fakeSkillConfigRepo) Get(ctx context.Context, name string) (*skill.SkillConfig, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.getResult, nil
}
func (f *fakeSkillConfigRepo) SearchByDescription(ctx context.Context, keyword string, limit int) ([]skill.SkillConfig, error) {
	return nil, nil
}
func (f *fakeSkillConfigRepo) Upsert(ctx context.Context, cfg skill.SkillConfig) error { return nil }

// approvalConfig builds a ConfigService that answers RequiresApproval with the
// given flag for any tool name.
func approvalConfig(approval bool) *skillsvc.ConfigService {
	return skillsvc.NewConfigService(&fakeSkillConfigRepo{
		getResult: &skill.SkillConfig{Name: "file_delete", RequiresApproval: approval},
	})
}

// fakeState is a minimal session.State backed by a map.
type fakeState struct {
	vals map[string]any
}

func (s *fakeState) Get(key string) (any, error) {
	if v, ok := s.vals[key]; ok {
		return v, nil
	}
	return nil, session.ErrStateKeyNotExist
}

func (s *fakeState) Set(key string, v any) error {
	s.vals[key] = v
	return nil
}

func (s *fakeState) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for k, v := range s.vals {
			if !yield(k, v) {
				return
			}
		}
	}
}

// fakeToolContext embeds the ADK StrictContextMock (which supplies the
// context.Context methods from Ctx) and overrides State().
type fakeToolContext struct {
	agent.StrictContextMock
	state session.State
}

func (f *fakeToolContext) State() session.State { return f.state }

func newToolContext(sessionID string) *fakeToolContext {
	return &fakeToolContext{
		StrictContextMock: agent.StrictContextMock{Ctx: context.Background()},
		state:             &fakeState{vals: map[string]any{"session_id": sessionID}},
	}
}

// setupWorkspaceFile creates a session workspace with a single file and returns
// the workspace root and file path. The caller is responsible for cleanup.
func setupWorkspaceFile(t *testing.T, sessionID, name string) (string, string) {
	t.Helper()
	ws := chatsvc.SessionWorkspace(sessionID)
	if err := os.MkdirAll(ws, 0o700); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	filePath := filepath.Join(ws, name)
	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return ws, filePath
}

// ---- file_delete ----

func TestFileDeleteConfirmApproved(t *testing.T) {
	sessionID := "hc-file-del-ok"
	ws, filePath := setupWorkspaceFile(t, sessionID, "a.txt")
	defer os.RemoveAll(ws)

	gate := &fakeHumanGate{confirmResult: true}
	fn := fileDelete(&Deps{HumanGate: gate, SkillConfig: approvalConfig(true)})
	res, err := fn(newToolContext(sessionID), FileDeleteArgs{Path: "a.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Path != "a.txt" {
		t.Fatalf("expected path a.txt, got %q", res.Path)
	}
	if gate.confirmCalls != 1 {
		t.Fatalf("expected 1 confirm call, got %d", gate.confirmCalls)
	}
	if _, statErr := os.Stat(filePath); !os.IsNotExist(statErr) {
		t.Fatal("expected file to be deleted after approval")
	}
}

func TestFileDeleteConfirmDenied(t *testing.T) {
	sessionID := "hc-file-del-denied"
	ws, filePath := setupWorkspaceFile(t, sessionID, "a.txt")
	defer os.RemoveAll(ws)

	gate := &fakeHumanGate{confirmResult: false}
	fn := fileDelete(&Deps{HumanGate: gate, SkillConfig: approvalConfig(true)})
	_, err := fn(newToolContext(sessionID), FileDeleteArgs{Path: "a.txt"})
	if err == nil {
		t.Fatal("expected error on deny")
	}
	if _, statErr := os.Stat(filePath); statErr != nil {
		t.Fatal("expected file to remain after deny")
	}
}

func TestFileDeleteConfirmError(t *testing.T) {
	sessionID := "hc-file-del-err"
	ws, filePath := setupWorkspaceFile(t, sessionID, "a.txt")
	defer os.RemoveAll(ws)

	gate := &fakeHumanGate{confirmErr: context.Canceled}
	fn := fileDelete(&Deps{HumanGate: gate, SkillConfig: approvalConfig(true)})
	if _, err := fn(newToolContext(sessionID), FileDeleteArgs{Path: "a.txt"}); err == nil {
		t.Fatal("expected error from confirm failure")
	}
	if _, statErr := os.Stat(filePath); statErr != nil {
		t.Fatal("expected file to remain after confirm error")
	}
}

func TestFileDeleteApprovalTrueNilGateRefuses(t *testing.T) {
	sessionID := "hc-file-del-nogate"
	ws, filePath := setupWorkspaceFile(t, sessionID, "a.txt")
	defer os.RemoveAll(ws)

	// requires_approval=true + HumanGate nil → refuse (fail-closed, SPEC-101).
	fn := fileDelete(&Deps{HumanGate: nil, SkillConfig: approvalConfig(true)})
	if _, err := fn(newToolContext(sessionID), FileDeleteArgs{Path: "a.txt"}); err == nil {
		t.Fatal("expected error when approval required but HumanGate is nil")
	}
	if _, statErr := os.Stat(filePath); statErr != nil {
		t.Fatal("expected file to remain when gate is missing")
	}
}

func TestFileDeleteApprovalFalseRunsDirectly(t *testing.T) {
	sessionID := "hc-file-del-noapproval"
	ws, filePath := setupWorkspaceFile(t, sessionID, "a.txt")
	defer os.RemoveAll(ws)

	// requires_approval=false → delete without any Confirm call.
	gate := &fakeHumanGate{confirmResult: true}
	fn := fileDelete(&Deps{HumanGate: gate, SkillConfig: approvalConfig(false)})
	res, err := fn(newToolContext(sessionID), FileDeleteArgs{Path: "a.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Path != "a.txt" {
		t.Fatalf("expected path a.txt, got %q", res.Path)
	}
	if gate.confirmCalls != 0 {
		t.Fatalf("expected 0 confirm calls when approval=false, got %d", gate.confirmCalls)
	}
	if _, statErr := os.Stat(filePath); !os.IsNotExist(statErr) {
		t.Fatal("expected file deleted when no approval is required")
	}
}

func TestFileDeleteApprovalLookupErrorRefuses(t *testing.T) {
	sessionID := "hc-file-del-lookuperr"
	ws, filePath := setupWorkspaceFile(t, sessionID, "a.txt")
	defer os.RemoveAll(ws)

	// Approval lookup failure → fail-closed refuse (SPEC-101 决策 1).
	svc := skillsvc.NewConfigService(&fakeSkillConfigRepo{getErr: errors.New("db down")})
	fn := fileDelete(&Deps{HumanGate: &fakeHumanGate{}, SkillConfig: svc})
	if _, err := fn(newToolContext(sessionID), FileDeleteArgs{Path: "a.txt"}); err == nil {
		t.Fatal("expected error when approval lookup fails")
	}
	if _, statErr := os.Stat(filePath); statErr != nil {
		t.Fatal("expected file to remain on lookup failure")
	}
}

func TestFileDeleteSkillConfigNilRefuses(t *testing.T) {
	sessionID := "hc-file-del-nilcfg"
	ws, filePath := setupWorkspaceFile(t, sessionID, "a.txt")
	defer os.RemoveAll(ws)

	// SkillConfig nil → fail-closed refuse (cannot determine approval).
	fn := fileDelete(&Deps{HumanGate: &fakeHumanGate{}, SkillConfig: nil})
	if _, err := fn(newToolContext(sessionID), FileDeleteArgs{Path: "a.txt"}); err == nil {
		t.Fatal("expected error when SkillConfig is nil")
	}
	if _, statErr := os.Stat(filePath); statErr != nil {
		t.Fatal("expected file to remain when SkillConfig is nil")
	}
}

// ---- dir_delete ----

func TestDirDeleteConfirmApproved(t *testing.T) {
	sessionID := "hc-dir-del-ok"
	ws := chatsvc.SessionWorkspace(sessionID)
	dirPath := filepath.Join(ws, "sub")
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer os.RemoveAll(ws)

	gate := &fakeHumanGate{confirmResult: true}
	fn := dirDelete(&Deps{HumanGate: gate, SkillConfig: approvalConfig(true)})
	if _, err := fn(newToolContext(sessionID), DirDeleteArgs{Path: "sub"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(dirPath); !os.IsNotExist(statErr) {
		t.Fatal("expected dir deleted after approval")
	}
}

func TestDirDeleteConfirmDenied(t *testing.T) {
	sessionID := "hc-dir-del-denied"
	ws := chatsvc.SessionWorkspace(sessionID)
	dirPath := filepath.Join(ws, "sub")
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer os.RemoveAll(ws)

	gate := &fakeHumanGate{confirmResult: false}
	fn := dirDelete(&Deps{HumanGate: gate, SkillConfig: approvalConfig(true)})
	if _, err := fn(newToolContext(sessionID), DirDeleteArgs{Path: "sub"}); err == nil {
		t.Fatal("expected error on deny")
	}
	if _, statErr := os.Stat(dirPath); statErr != nil {
		t.Fatal("expected dir to remain after deny")
	}
}

func TestDirDeleteApprovalFalseRunsDirectly(t *testing.T) {
	sessionID := "hc-dir-del-noapproval"
	ws := chatsvc.SessionWorkspace(sessionID)
	dirPath := filepath.Join(ws, "sub")
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer os.RemoveAll(ws)

	gate := &fakeHumanGate{confirmResult: true}
	fn := dirDelete(&Deps{HumanGate: gate, SkillConfig: approvalConfig(false)})
	if _, err := fn(newToolContext(sessionID), DirDeleteArgs{Path: "sub"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gate.confirmCalls != 0 {
		t.Fatalf("expected 0 confirm calls when approval=false, got %d", gate.confirmCalls)
	}
	if _, statErr := os.Stat(dirPath); !os.IsNotExist(statErr) {
		t.Fatal("expected dir deleted when no approval is required")
	}
}

func TestDirDeleteApprovalTrueNilGateRefuses(t *testing.T) {
	sessionID := "hc-dir-del-nogate"
	ws := chatsvc.SessionWorkspace(sessionID)
	dirPath := filepath.Join(ws, "sub")
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer os.RemoveAll(ws)

	fn := dirDelete(&Deps{HumanGate: nil, SkillConfig: approvalConfig(true)})
	if _, err := fn(newToolContext(sessionID), DirDeleteArgs{Path: "sub"}); err == nil {
		t.Fatal("expected error when approval required but HumanGate is nil")
	}
	if _, statErr := os.Stat(dirPath); statErr != nil {
		t.Fatal("expected dir to remain when gate is missing")
	}
}

func TestDirDeleteApprovalLookupErrorRefuses(t *testing.T) {
	sessionID := "hc-dir-del-lookuperr"
	ws := chatsvc.SessionWorkspace(sessionID)
	dirPath := filepath.Join(ws, "sub")
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer os.RemoveAll(ws)

	svc := skillsvc.NewConfigService(&fakeSkillConfigRepo{getErr: errors.New("db down")})
	fn := dirDelete(&Deps{HumanGate: &fakeHumanGate{}, SkillConfig: svc})
	if _, err := fn(newToolContext(sessionID), DirDeleteArgs{Path: "sub"}); err == nil {
		t.Fatal("expected error when approval lookup fails")
	}
	if _, statErr := os.Stat(dirPath); statErr != nil {
		t.Fatal("expected dir to remain on lookup failure")
	}
}

// ---- ask_user ----

func TestAskUserReturnsAnswer(t *testing.T) {
	gate := &fakeHumanGate{askResult: "按地区"}
	fn := askUser(&Deps{HumanGate: gate})
	res, err := fn(newToolContext("hc-ask-ok"), AskUserArgs{Question: "维度?", Options: []string{"按地区", "按产品"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Answer != "按地区" {
		t.Fatalf("expected answer 按地区, got %q", res.Answer)
	}
	if gate.askCalls != 1 {
		t.Fatalf("expected 1 ask call, got %d", gate.askCalls)
	}
}

func TestAskUserMissingQuestion(t *testing.T) {
	fn := askUser(&Deps{HumanGate: &fakeHumanGate{}})
	if _, err := fn(newToolContext("hc-ask-empty"), AskUserArgs{Question: "  "}); err == nil {
		t.Fatal("expected error for empty question")
	}
}

func TestAskUserNoGate(t *testing.T) {
	fn := askUser(&Deps{HumanGate: nil})
	if _, err := fn(newToolContext("hc-ask-nogate"), AskUserArgs{Question: "q"}); err == nil {
		t.Fatal("expected error when HumanGate is nil")
	}
}

func TestAskUserTooManyOptions(t *testing.T) {
	opts := make([]string, 11)
	for i := range opts {
		opts[i] = "o"
	}
	fn := askUser(&Deps{HumanGate: &fakeHumanGate{}})
	if _, err := fn(newToolContext("hc-ask-many"), AskUserArgs{Question: "q", Options: opts}); err == nil {
		t.Fatal("expected error for >10 options")
	}
}

func TestAskUserNoSession(t *testing.T) {
	// A tool context without session_id must be rejected.
	tc := &fakeToolContext{
		StrictContextMock: agent.StrictContextMock{Ctx: context.Background()},
		state:             &fakeState{vals: map[string]any{}},
	}
	fn := askUser(&Deps{HumanGate: &fakeHumanGate{}})
	if _, err := fn(tc, AskUserArgs{Question: "q"}); err == nil {
		t.Fatal("expected error for missing session context")
	}
}
