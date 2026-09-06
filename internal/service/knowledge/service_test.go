package knowledge

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func newTestSvc(t *testing.T) (*Service, *mockrepo.KBRepository) {
	t.Helper()
	kb := mockrepo.NewKBRepository(t)
	return NewService(kb), kb
}

// TestCreateTextDoc_Success verifies the full pipeline: redaction (no-op when
// no redactor) → UploadFile → CreateDoc (with correct args) → EnqueueRaw.
func TestCreateTextDoc_Success(t *testing.T) {
	svc, kb := newTestSvc(t)
	queue := mockrepo.NewQueueRepository(t)
	svc.WithQueue(queue)

	kb.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	var captured *knowledge.KnowledgeDoc
	kb.On("CreateDoc", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		captured = args.Get(1).(*knowledge.KnowledgeDoc)
	}).Return(nil)
	queue.On("EnqueueRaw", mock.Anything, "kb_index", mock.Anything).Return(nil)

	doc, err := svc.CreateTextDoc(t.Context(), "user-1", "我的总结", "今天的内容")
	if err != nil {
		t.Fatalf("CreateTextDoc: %v", err)
	}
	if doc == nil {
		t.Fatal("doc should not be nil")
	}
	if captured == nil {
		t.Fatal("CreateDoc was not called")
	}
	if captured.UserID != "user-1" {
		t.Errorf("CreateDoc UserID = %q, want user-1", captured.UserID)
	}
	if captured.Title != "我的总结" {
		t.Errorf("CreateDoc Title = %q, want 我的总结", captured.Title)
	}
	if captured.FileName != "我的总结.txt" {
		t.Errorf("CreateDoc FileName = %q, want 我的总结.txt", captured.FileName)
	}
	if captured.FileType != knowledge.FileTypeTxt {
		t.Errorf("CreateDoc FileType = %q, want %q", captured.FileType, knowledge.FileTypeTxt)
	}
	if captured.SizeBytes != int64(len("今天的内容")) {
		t.Errorf("CreateDoc SizeBytes = %d, want %d", captured.SizeBytes, len("今天的内容"))
	}
	if captured.GridFSFileID == "" {
		t.Error("CreateDoc GridFSFileID should be non-empty")
	}
	if captured.Status != knowledge.StatusUploaded {
		t.Errorf("CreateDoc Status = %q, want uploaded", captured.Status)
	}
	// queue enqueued with the doc's IDs.
	queue.AssertCalled(t, "EnqueueRaw", mock.Anything, "kb_index", mock.MatchedBy(func(p any) bool {
		return p != nil
	}))
}

// TestCreateTextDoc_TextTooLarge rejects content over MaxKBTextBytes before
// touching storage.
func TestCreateTextDoc_TextTooLarge(t *testing.T) {
	svc, kb := newTestSvc(t)
	big := strings.Repeat("a", knowledge.MaxKBTextBytes+1)
	_, err := svc.CreateTextDoc(t.Context(), "u", "t", big)
	if err == nil {
		t.Fatal("expected error for oversized content")
	}
	kb.AssertNotCalled(t, "UploadFile", mock.Anything, mock.Anything, mock.Anything)
}

// TestCreateTextDoc_TitleTruncated truncates an overlong title to MaxKBTitleRunes.
func TestCreateTextDoc_TitleTruncated(t *testing.T) {
	svc, kb := newTestSvc(t)
	kb.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	var captured *knowledge.KnowledgeDoc
	kb.On("CreateDoc", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		captured = args.Get(1).(*knowledge.KnowledgeDoc)
	}).Return(nil)

	longTitle := strings.Repeat("长", knowledge.MaxKBTitleRunes+50)
	_, err := svc.CreateTextDoc(t.Context(), "u", longTitle, "body")
	if err != nil {
		t.Fatalf("CreateTextDoc: %v", err)
	}
	if captured == nil {
		t.Fatal("CreateDoc not called")
	}
	if got := len([]rune(captured.Title)); got != knowledge.MaxKBTitleRunes {
		t.Errorf("title rune length = %d, want %d", got, knowledge.MaxKBTitleRunes)
	}
}

// TestCreateTextDoc_EmptyUserID refuses to create a doc without an owner.
func TestCreateTextDoc_EmptyUserID(t *testing.T) {
	svc, kb := newTestSvc(t)
	_, err := svc.CreateTextDoc(t.Context(), "", "t", "body")
	if err == nil {
		t.Fatal("expected error for empty userID")
	}
	kb.AssertNotCalled(t, "UploadFile", mock.Anything, mock.Anything, mock.Anything)
}

// TestCreateTextDoc_QueueNilDegrades creates the doc synchronously and skips
// enqueue when no queue is wired.
func TestCreateTextDoc_QueueNilDegrades(t *testing.T) {
	svc, kb := newTestSvc(t)
	// No WithQueue call → s.queue == nil.
	kb.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	kb.On("CreateDoc", mock.Anything, mock.Anything).Return(nil)

	doc, err := svc.CreateTextDoc(t.Context(), "u", "t", "body")
	if err != nil {
		t.Fatalf("CreateTextDoc (no queue): %v", err)
	}
	if doc == nil {
		t.Fatal("doc should not be nil when queue is nil")
	}
	if doc.Status != knowledge.StatusUploaded {
		t.Errorf("Status = %q, want uploaded", doc.Status)
	}
}

// TestTruncateRunes verifies the rune-safe truncation helper.
func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("abc", 5); got != "abc" {
		t.Errorf("truncateRunes(abc,5) = %q, want abc", got)
	}
	if got := truncateRunes("你好世界", 2); got != "你好" {
		t.Errorf("truncateRunes(你好世界,2) = %q, want 你好", got)
	}
}
