package knowledge

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/logic/webimport"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

// fakeImporter implements webimport.Importer for ImportURL unit tests.
type fakeImporter struct {
	res *webimport.Result
	err error
}

func (f fakeImporter) Import(_ context.Context, _ string) (*webimport.Result, error) {
	return f.res, f.err
}

func TestCreateFromText_Success(t *testing.T) {
	svc, kb := newTestSvc(t)
	queue := mockrepo.NewQueueRepository(t)
	svc.WithQueue(queue)

	kb.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	var captured *knowledge.KnowledgeDoc
	kb.On("CreateDoc", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		captured = args.Get(1).(*knowledge.KnowledgeDoc)
	}).Return(nil)
	queue.On("EnqueueRaw", mock.Anything, "kb_index", mock.Anything).Return(nil)

	doc, err := svc.CreateFromText(t.Context(), "u", "标题", "custom.txt", "正文")
	if err != nil {
		t.Fatalf("CreateFromText: %v", err)
	}
	if doc == nil || captured == nil {
		t.Fatal("doc/captured should not be nil")
	}
	if captured.FileName != "custom.txt" {
		t.Errorf("FileName = %q, want custom.txt", captured.FileName)
	}
	if captured.FileType != knowledge.FileTypeTxt {
		t.Errorf("FileType = %q, want %q", captured.FileType, knowledge.FileTypeTxt)
	}
	if captured.Title != "标题" {
		t.Errorf("Title = %q, want 标题", captured.Title)
	}
}

func TestCreateFromText_TooLarge(t *testing.T) {
	svc, kb := newTestSvc(t)
	big := strings.Repeat("a", knowledge.MaxKBTextBytes+1)
	_, err := svc.CreateFromText(t.Context(), "u", "t", "f.txt", big)
	if err == nil {
		t.Fatal("expected error for oversized text")
	}
	kb.AssertNotCalled(t, "UploadFile", mock.Anything, mock.Anything, mock.Anything)
}

func TestCreateFromImage_Success(t *testing.T) {
	svc, kb := newTestSvc(t)
	kb.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	var captured *knowledge.KnowledgeDoc
	kb.On("CreateDoc", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		captured = args.Get(1).(*knowledge.KnowledgeDoc)
	}).Return(nil)

	doc, err := svc.CreateFromImage(t.Context(), "u", "图片", "img.png", []byte("png-data"), "image/png")
	if err != nil {
		t.Fatalf("CreateFromImage: %v", err)
	}
	if doc == nil || captured == nil {
		t.Fatal("doc/captured should not be nil")
	}
	if captured.FileType != knowledge.FileTypeImage {
		t.Errorf("FileType = %q, want %q", captured.FileType, knowledge.FileTypeImage)
	}
	if captured.SizeBytes != int64(len("png-data")) {
		t.Errorf("SizeBytes = %d, want %d", captured.SizeBytes, len("png-data"))
	}
}

func TestCreateFromImage_TooLarge(t *testing.T) {
	svc, kb := newTestSvc(t)
	big := make([]byte, knowledge.MaxKBImageBytes+1)
	_, err := svc.CreateFromImage(t.Context(), "u", "t", "i.png", big, "image/png")
	if err == nil {
		t.Fatal("expected error for oversized image")
	}
	kb.AssertNotCalled(t, "UploadFile", mock.Anything, mock.Anything, mock.Anything)
}

func TestImportURL_Success(t *testing.T) {
	svc, kb := newTestSvc(t)
	svc.WithURLImporter(fakeImporter{res: &webimport.Result{
		Text:          "页面正文",
		Images:        []webimport.Image{{Data: []byte("img-bytes"), MimeType: "image/png"}},
		SkippedImages: 1,
	}})

	kb.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	var created []*knowledge.KnowledgeDoc
	kb.On("CreateDoc", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		created = append(created, args.Get(1).(*knowledge.KnowledgeDoc))
	}).Return(nil)

	res, err := svc.ImportURL(t.Context(), "u", "https://example.com/page")
	if err != nil {
		t.Fatalf("ImportURL: %v", err)
	}
	if res.SkippedImages != 1 {
		t.Errorf("SkippedImages = %d, want 1", res.SkippedImages)
	}
	if res.TextDocID == "" {
		t.Error("TextDocID should be non-empty")
	}
	if len(res.ImageDocIDs) != 1 {
		t.Errorf("ImageDocIDs len = %d, want 1", len(res.ImageDocIDs))
	}
	if len(res.DocIDs) != 2 {
		t.Errorf("DocIDs len = %d, want 2", len(res.DocIDs))
	}
	if len(created) != 2 {
		t.Fatalf("CreateDoc called %d times, want 2", len(created))
	}

	// Naming mirrors front-end PDF parse: {hash(url)}-{1-based counter}.
	base := hashOfURL("https://example.com/page")
	if created[0].Title != base+"-1" {
		t.Errorf("text doc title = %q, want %q", created[0].Title, base+"-1")
	}
	if created[0].FileType != knowledge.FileTypeTxt {
		t.Errorf("text doc FileType = %q", created[0].FileType)
	}
	if created[1].Title != base+"-2" {
		t.Errorf("image doc title = %q, want %q", created[1].Title, base+"-2")
	}
	if created[1].FileType != knowledge.FileTypeImage {
		t.Errorf("image doc FileType = %q", created[1].FileType)
	}
}

func TestImportURL_ImageOnly(t *testing.T) {
	svc, kb := newTestSvc(t)
	svc.WithURLImporter(fakeImporter{res: &webimport.Result{
		Images: []webimport.Image{{Data: []byte("i"), MimeType: "image/png"}},
	}})

	kb.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	var created []*knowledge.KnowledgeDoc
	kb.On("CreateDoc", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		created = append(created, args.Get(1).(*knowledge.KnowledgeDoc))
	}).Return(nil)

	res, err := svc.ImportURL(t.Context(), "u", "https://example.com/img")
	if err != nil {
		t.Fatalf("ImportURL: %v", err)
	}
	if res.TextDocID != "" {
		t.Errorf("TextDocID = %q, want empty (no text)", res.TextDocID)
	}
	if len(res.ImageDocIDs) != 1 {
		t.Errorf("ImageDocIDs len = %d, want 1", len(res.ImageDocIDs))
	}
	// No text → counter starts at 1 for the first image.
	base := hashOfURL("https://example.com/img")
	if created[0].Title != base+"-1" {
		t.Errorf("image doc title = %q, want %q", created[0].Title, base+"-1")
	}
}

func TestImportURL_NoImporter(t *testing.T) {
	svc, _ := newTestSvc(t)
	_, err := svc.ImportURL(t.Context(), "u", "https://example.com")
	if err == nil {
		t.Fatal("expected error when importer is not wired")
	}
}

func TestImportURL_EmptyUserID(t *testing.T) {
	svc, _ := newTestSvc(t)
	svc.WithURLImporter(fakeImporter{res: &webimport.Result{Text: "x"}})
	_, err := svc.ImportURL(t.Context(), "", "https://example.com")
	if err == nil {
		t.Fatal("expected error for empty userID")
	}
}

func TestImportURL_ImporterError(t *testing.T) {
	svc, kb := newTestSvc(t)
	svc.WithURLImporter(fakeImporter{err: webimport.ErrRenderFailed})
	_, err := svc.ImportURL(t.Context(), "u", "https://example.com")
	if err == nil {
		t.Fatal("expected error propagated from importer")
	}
	kb.AssertNotCalled(t, "CreateDoc", mock.Anything, mock.Anything)
}

func TestHashOfURL(t *testing.T) {
	a := hashOfURL("https://example.com/a")
	b := hashOfURL("https://example.com/b")
	if a == b {
		t.Errorf("hashOfURL should differ for different URLs")
	}
	if len(a) != 16 {
		t.Errorf("hash length = %d, want 16", len(a))
	}
	if hashOfURL("https://example.com/a") != a {
		t.Errorf("hashOfURL should be deterministic")
	}
}

func TestExtFromMime(t *testing.T) {
	cases := map[string]string{
		"image/png":  ".png",
		"image/jpeg": ".jpg",
		"image/gif":  ".gif",
		"image/webp": ".webp",
		"image/svg+xml": ".svg",
		"application/octet-stream": ".jpg",
	}
	for mime, want := range cases {
		if got := extFromMime(mime); got != want {
			t.Errorf("extFromMime(%q) = %q, want %q", mime, got, want)
		}
	}
}
