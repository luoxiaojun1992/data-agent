package knowledge

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
)

type fakeRedactor struct {
	out string
	err error
}

func (f fakeRedactor) Redact(_ context.Context, text string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if f.out != "" {
		return f.out, nil
	}
	return text, nil
}

var _ security.Redactor = fakeRedactor{}

func TestCreateFromText_EmptyFileNameDefaultsToTitle(t *testing.T) {
	svc, kb := newTestSvc(t)
	kb.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	var captured *knowledge.KnowledgeDoc
	kb.On("CreateDoc", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		captured = args.Get(1).(*knowledge.KnowledgeDoc)
	}).Return(nil)

	_, err := svc.CreateFromText(t.Context(), "u", "标题", "", "正文")
	if err != nil {
		t.Fatalf("CreateFromText: %v", err)
	}
	if captured == nil {
		t.Fatal("CreateDoc not called")
	}
	if captured.FileName != "标题.txt" {
		t.Errorf("FileName = %q, want 标题.txt", captured.FileName)
	}
}

func TestCreateFromText_UploadError(t *testing.T) {
	svc, kb := newTestSvc(t)
	kb.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("gridfs down"))

	if _, err := svc.CreateFromText(t.Context(), "u", "t", "f.txt", "正文"); err == nil {
		t.Fatal("expected upload error")
	}
	kb.AssertNotCalled(t, "CreateDoc", mock.Anything, mock.Anything)
}

func TestCreateFromText_CreateDocError(t *testing.T) {
	svc, kb := newTestSvc(t)
	kb.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	kb.On("CreateDoc", mock.Anything, mock.Anything).Return(fmt.Errorf("mongo down"))

	if _, err := svc.CreateFromText(t.Context(), "u", "t", "f.txt", "正文"); err == nil {
		t.Fatal("expected create doc error")
	}
}

func TestCreateFromText_RedactError(t *testing.T) {
	svc, kb := newTestSvc(t)
	svc.WithRedactor(fakeRedactor{err: errors.New("redact down")}, nil)

	if _, err := svc.CreateFromText(t.Context(), "u", "t", "f.txt", "正文"); err == nil {
		t.Fatal("expected pii redact error (fail-closed)")
	}
	kb.AssertNotCalled(t, "UploadFile", mock.Anything, mock.Anything, mock.Anything)
}

func TestCreateFromImage_EmptyUserID(t *testing.T) {
	svc, kb := newTestSvc(t)
	if _, err := svc.CreateFromImage(t.Context(), "", "t", "i.png", []byte("x"), "image/png"); err == nil {
		t.Fatal("expected empty userID error")
	}
	kb.AssertNotCalled(t, "UploadFile", mock.Anything, mock.Anything, mock.Anything)
}

func TestCreateFromImage_EmptyMimeDefaults(t *testing.T) {
	svc, kb := newTestSvc(t)
	kb.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	kb.On("CreateDoc", mock.Anything, mock.Anything).Return(nil)

	if _, err := svc.CreateFromImage(t.Context(), "u", "img", "i.png", []byte("x"), ""); err != nil {
		t.Fatalf("CreateFromImage empty mime: %v", err)
	}
}

func TestCreateFromImage_UploadError(t *testing.T) {
	svc, kb := newTestSvc(t)
	kb.On("UploadFile", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("gridfs down"))

	if _, err := svc.CreateFromImage(t.Context(), "u", "t", "i.png", []byte("x"), "image/png"); err == nil {
		t.Fatal("expected upload error")
	}
	kb.AssertNotCalled(t, "CreateDoc", mock.Anything, mock.Anything)
}
