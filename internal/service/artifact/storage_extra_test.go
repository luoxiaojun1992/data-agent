package artifact

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
	"github.com/luoxiaojun1992/data-agent/internal/infra/metrics"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

type fakeCounter struct {
	incrCalls int
}

func (f *fakeCounter) Incr(ctx context.Context, m metrics.Metric, at time.Time, delta int64) error {
	f.incrCalls++
	return nil
}
func (f *fakeCounter) Stop() {}

// ── WithCounter ──────────────────────────────────────────────────────

func TestWithCounter(t *testing.T) {
	fs := mockrepo.NewFileRepository(t)
	meta := mockrepo.NewArtifactRepository(t)
	fc := &fakeCounter{}

	s := NewStorage(fs, meta).WithCounter(fc)
	if s.counter != fc {
		t.Error("WithCounter did not attach the counter")
	}
}

// ── Upload error branches ────────────────────────────────────────────

func TestUpload_FileUploadError(t *testing.T) {
	fs := mockrepo.NewFileRepository(t)
	meta := mockrepo.NewArtifactRepository(t)
	fs.On("Upload", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("disk full"))

	s := NewStorage(fs, meta)
	if _, err := s.Upload("u1", "s1", "", "f.txt", "text/plain", strings.NewReader("x"), false); err == nil {
		t.Fatal("expected file upload error")
	}
}

func TestUpload_MetaCreateErrorRollsBackFile(t *testing.T) {
	fs := mockrepo.NewFileRepository(t)
	meta := mockrepo.NewArtifactRepository(t)
	fs.On("Upload", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	meta.On("Create", mock.Anything, mock.Anything).Return(errors.New("meta insert failed"))
	fs.On("Delete", mock.Anything, mock.Anything).Return(nil)

	s := NewStorage(fs, meta)
	if _, err := s.Upload("u1", "s1", "", "f.txt", "text/plain", strings.NewReader("x"), false); err == nil {
		t.Fatal("expected meta create error")
	}
	// Rollback should have deleted the uploaded file.
	fs.AssertCalled(t, "Delete", mock.Anything, mock.Anything)
}

func TestUpload_WithCounterIncrements(t *testing.T) {
	fs := mockrepo.NewFileRepository(t)
	meta := mockrepo.NewArtifactRepository(t)
	fs.On("Upload", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	meta.On("Create", mock.Anything, mock.Anything).Return(nil)
	fc := &fakeCounter{}

	s := NewStorage(fs, meta).WithCounter(fc)
	if _, err := s.Upload("u1", "s1", "", "f.txt", "text/plain", strings.NewReader("x"), false); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if fc.incrCalls != 1 {
		t.Errorf("Incr calls = %d, want 1", fc.incrCalls)
	}
}

// ── Download / DownloadURL ───────────────────────────────────────────

func TestDownload_Success(t *testing.T) {
	fs := mockrepo.NewFileRepository(t)
	meta := mockrepo.NewArtifactRepository(t)
	meta.On("FindByID", mock.Anything, "art_1").Return(&artifact.Artifact{ID: "art_1", StoragePath: "u/s/f.txt"}, nil)
	fs.On("Download", mock.Anything, "u/s/f.txt").Return([]byte("data"), nil)

	data, err := NewStorage(fs, meta).Download("art_1")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if string(data) != "data" {
		t.Errorf("data = %q", data)
	}
}

func TestDownload_NotFound(t *testing.T) {
	fs := mockrepo.NewFileRepository(t)
	meta := mockrepo.NewArtifactRepository(t)
	meta.On("FindByID", mock.Anything, "missing").Return((*artifact.Artifact)(nil), errors.New("nf"))

	if _, err := NewStorage(fs, meta).Download("missing"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestDownloadURL_Success(t *testing.T) {
	fs := mockrepo.NewFileRepository(t)
	meta := mockrepo.NewArtifactRepository(t)
	meta.On("FindByID", mock.Anything, "art_1").Return(&artifact.Artifact{ID: "art_1", StoragePath: "artifacts/u/s/f.txt"}, nil)

	url, err := NewStorage(fs, meta).DownloadURL("art_1")
	if err != nil {
		t.Fatalf("DownloadURL: %v", err)
	}
	if url != "/files/artifacts/u/s/f.txt" {
		t.Errorf("url = %q, want /files/artifacts/u/s/f.txt", url)
	}
}

func TestDownloadURL_NotFound(t *testing.T) {
	fs := mockrepo.NewFileRepository(t)
	meta := mockrepo.NewArtifactRepository(t)
	meta.On("FindByID", mock.Anything, "art_1").Return(nil, errors.New("nf"))

	if _, err := NewStorage(fs, meta).DownloadURL("art_1"); err == nil {
		t.Fatal("expected not-found error")
	}
}

// ── ListByUser pagination ────────────────────────────────────────────

func TestListByUser_ClampDefaults(t *testing.T) {
	fs := mockrepo.NewFileRepository(t)
	meta := mockrepo.NewArtifactRepository(t)
	var gotSkip, gotLimit int64
	meta.On("ListByUser", mock.Anything, "u1", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			gotSkip = args.Get(2).(int64)
			gotLimit = args.Get(3).(int64)
		}).
		Return([]*artifact.Artifact{}, int64(0), nil)

	_, _, err := NewStorage(fs, meta).ListByUser("u1", 0, 0)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	// page 0 → 1, pageSize 0 → 20, skip = 0
	if gotSkip != 0 || gotLimit != 20 {
		t.Errorf("skip/limit = %d/%d, want 0/20", gotSkip, gotLimit)
	}
}

func TestListByUser_SkipCalculation(t *testing.T) {
	fs := mockrepo.NewFileRepository(t)
	meta := mockrepo.NewArtifactRepository(t)
	var gotSkip, gotLimit int64
	meta.On("ListByUser", mock.Anything, "u1", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			gotSkip = args.Get(2).(int64)
			gotLimit = args.Get(3).(int64)
		}).
		Return([]*artifact.Artifact{}, int64(0), nil)

	_, _, err := NewStorage(fs, meta).ListByUser("u1", 3, 10)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	// page 3, pageSize 10 → skip = 20
	if gotSkip != 20 || gotLimit != 10 {
		t.Errorf("skip/limit = %d/%d, want 20/10", gotSkip, gotLimit)
	}
}

func TestListByUser_PageSizeClampedHigh(t *testing.T) {
	fs := mockrepo.NewFileRepository(t)
	meta := mockrepo.NewArtifactRepository(t)
	var gotLimit int64
	meta.On("ListByUser", mock.Anything, "u1", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { gotLimit = args.Get(3).(int64) }).
		Return([]*artifact.Artifact{}, int64(0), nil)

	_, _, err := NewStorage(fs, meta).ListByUser("u1", 1, 500)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if gotLimit != 20 {
		t.Errorf("limit = %d, want 20 (clamped)", gotLimit)
	}
}

// ── ListByTask ───────────────────────────────────────────────────────

func TestListByTask_Success(t *testing.T) {
	fs := mockrepo.NewFileRepository(t)
	meta := mockrepo.NewArtifactRepository(t)
	meta.On("ListByTask", mock.Anything, "t1").Return([]*artifact.Artifact{{ID: "a1"}}, nil)

	arts, err := NewStorage(fs, meta).ListByTask("t1")
	if err != nil {
		t.Fatalf("ListByTask: %v", err)
	}
	if len(arts) != 1 {
		t.Errorf("got %d artifacts, want 1", len(arts))
	}
}
