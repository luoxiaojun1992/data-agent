package artifact

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func TestNewStorage(t *testing.T) {
	meta := mockrepo.NewArtifactRepository(t)
	fs := mockrepo.NewFileRepository(t)
	s := NewStorage(fs, meta)
	if s == nil {
		t.Fatal("NewStorage should not return nil")
	}
}

func TestUpload_Success(t *testing.T) {
	meta := mockrepo.NewArtifactRepository(t)
	fs := mockrepo.NewFileRepository(t)
	meta.On("Create", mock.Anything, mock.Anything).Return(nil)
	fs.On("Upload", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	s := NewStorage(fs, meta)
	a, err := s.Upload("u1", "s1", "t1", "test.txt", "text/plain", strings.NewReader("hello"), true)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if a == nil || a.ID == "" {
		t.Fatal("expected artifact with ID")
	}
}

func TestFindByID_Success(t *testing.T) {
	meta := mockrepo.NewArtifactRepository(t)
	fs := mockrepo.NewFileRepository(t)
	meta.On("FindByID", mock.Anything, "art_1").Return(&artifact.Artifact{ID: "art_1", Name: "test.txt"}, nil)

	a, err := NewStorage(fs, meta).FindByID("art_1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if a.Name != "test.txt" {
		t.Errorf("Name: got %q, want test.txt", a.Name)
	}
}

func TestListBySession_Success(t *testing.T) {
	meta := mockrepo.NewArtifactRepository(t)
	fs := mockrepo.NewFileRepository(t)
	meta.On("ListBySession", mock.Anything, "s1").Return([]*artifact.Artifact{
		{ID: "a1", Name: "f1.txt"},
		{ID: "a2", Name: "f2.txt"},
	}, nil)

	arts, err := NewStorage(fs, meta).ListBySession("s1")
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("got %d, want 2", len(arts))
	}
}

func TestDelete_Success(t *testing.T) {
	meta := mockrepo.NewArtifactRepository(t)
	fs := mockrepo.NewFileRepository(t)
	meta.On("FindByID", mock.Anything, "art_1").Return(&artifact.Artifact{ID: "art_1", StoragePath: "/tmp/x"}, nil)
	meta.On("Delete", mock.Anything, "art_1").Return(nil)
	fs.On("Delete", mock.Anything, mock.Anything).Return(nil)

	if err := NewStorage(fs, meta).Delete("art_1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
