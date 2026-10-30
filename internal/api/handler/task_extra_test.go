package handler

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	tasksvc "github.com/luoxiaojun1992/data-agent/internal/service/task"
	mocktasksvc "github.com/luoxiaojun1992/data-agent/internal/service/task/mocks"
	"github.com/stretchr/testify/mock"
)

// ── ListRuns ──

func TestListRuns_Success(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	runSvc := mocktasksvc.NewTaskRunService(t)
	h := NewTaskHandler(svc, runSvc)

	runs := []*task.TaskRun{{ID: "run_1"}, {ID: "run_2"}}
	runSvc.On("ListRuns", "task_1", "", false, "running", int64(0), int64(20)).Return(runs, int64(2), nil)

	c, w := newGinContext("GET", "/tasks/task_1/runs?status=running&page=1&page_size=20", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.ListRuns(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "run_1") {
		t.Errorf("body should contain run_1: %s", w.Body.String())
	}
}

func TestListRuns_Pagination(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	runSvc := mocktasksvc.NewTaskRunService(t)
	h := NewTaskHandler(svc, runSvc)

	// page=2, page_size=5 → skip=5
	runSvc.On("ListRuns", "task_1", "", false, "", int64(5), int64(5)).Return([]*task.TaskRun{}, int64(0), nil)

	c, w := newGinContext("GET", "/tasks/task_1/runs?page=2&page_size=5", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.ListRuns(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListRuns_NotFound(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	runSvc := mocktasksvc.NewTaskRunService(t)
	h := NewTaskHandler(svc, runSvc)

	runSvc.On("ListRuns", "missing", "", false, "", int64(0), int64(20)).Return(([]*task.TaskRun)(nil), int64(0), tasksvc.ErrNotFound)

	c, w := newGinContext("GET", "/tasks/missing/runs", "")
	c.Params = gin.Params{{Key: "task_id", Value: "missing"}}
	h.ListRuns(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestListRuns_InternalError(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	runSvc := mocktasksvc.NewTaskRunService(t)
	h := NewTaskHandler(svc, runSvc)

	runSvc.On("ListRuns", "task_1", "", false, "", int64(0), int64(20)).Return(([]*task.TaskRun)(nil), int64(0), fmt.Errorf("db down"))

	c, w := newGinContext("GET", "/tasks/task_1/runs", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.ListRuns(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── CreateRun ──

func TestCreateRun_Success(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc, nil)

	svc.On("CreateRun", "task_1", "", false).Return(&task.TaskRun{ID: "run_new"}, nil)

	c, w := newGinContext("POST", "/tasks/task_1/run", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.CreateRun(c)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateRun_NotFound(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc, nil)

	svc.On("CreateRun", "missing", "", false).Return((*task.TaskRun)(nil), tasksvc.ErrNotFound)

	c, w := newGinContext("POST", "/tasks/missing/run", "")
	c.Params = gin.Params{{Key: "task_id", Value: "missing"}}
	h.CreateRun(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestCreateRun_InternalError(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc, nil)

	svc.On("CreateRun", "task_1", "", false).Return((*task.TaskRun)(nil), fmt.Errorf("queue full"))

	c, w := newGinContext("POST", "/tasks/task_1/run", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.CreateRun(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── SetEnabled ──

func TestSetEnabled_BadJSON(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc, nil)

	c, w := newGinContext("PATCH", "/tasks/task_1/enabled", "not-json")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.SetEnabled(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	svc.AssertNotCalled(t, "SetScheduledEnabled", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestSetEnabled_NotFound(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc, nil)

	svc.On("SetScheduledEnabled", "missing", "", false, true).Return(tasksvc.ErrNotFound)

	c, w := newGinContext("PATCH", "/tasks/missing/enabled", `{"enabled":true}`)
	c.Params = gin.Params{{Key: "task_id", Value: "missing"}}
	h.SetEnabled(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSetEnabled_InternalError(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc, nil)

	svc.On("SetScheduledEnabled", "task_1", "", false, true).Return(fmt.Errorf("db down"))

	c, w := newGinContext("PATCH", "/tasks/task_1/enabled", `{"enabled":true}`)
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.SetEnabled(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── GetRun ──

func TestGetRun_NotFound(t *testing.T) {
	runSvc := mocktasksvc.NewTaskRunService(t)
	h := NewTaskHandler(nil, runSvc)

	runSvc.On("GetRun", "missing", "", false).Return((*task.TaskRun)(nil), fmt.Errorf("not found"))

	c, w := newGinContext("GET", "/tasks/t1/runs/missing", "")
	c.Params = gin.Params{{Key: "run_id", Value: "missing"}}
	h.GetRun(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── prependPDFExcelText (SPEC-096 D1) ──

func TestPrependPDFExcelText_EmptyAttachments(t *testing.T) {
	if got := prependPDFExcelText("desc", nil, nil); got != "desc" {
		t.Errorf("got %q, want desc", got)
	}
}

func TestPrependPDFExcelText_PDFOnly(t *testing.T) {
	pdfs := []domainchat.PdfAttachment{
		{Name: "a.pdf", Text: "pdf text"},
		{Name: "empty.pdf", Text: "   "}, // blank → skipped
	}
	got := prependPDFExcelText("desc", pdfs, nil)
	if !strings.Contains(got, "[PDF:a.pdf]") {
		t.Errorf("missing pdf tag: %q", got)
	}
	if strings.Contains(got, "empty.pdf") {
		t.Errorf("blank pdf should be skipped: %q", got)
	}
	if !strings.HasSuffix(got, "desc") {
		t.Errorf("description should be appended last: %q", got)
	}
}

func TestPrependPDFExcelText_ExcelOnly(t *testing.T) {
	excels := []domainchat.ExcelAttachment{
		{Name: "b.xlsx", Text: "excel text"},
		{Name: "blank.xlsx", Text: ""}, // blank → skipped
	}
	got := prependPDFExcelText("desc", nil, excels)
	if !strings.Contains(got, "[Excel:b.xlsx]") {
		t.Errorf("missing excel tag: %q", got)
	}
	if strings.Contains(got, "blank.xlsx") {
		t.Errorf("blank excel should be skipped: %q", got)
	}
}

func TestPrependPDFExcelText_BothOrdered(t *testing.T) {
	pdfs := []domainchat.PdfAttachment{{Name: "a.pdf", Text: "pdf"}}
	excels := []domainchat.ExcelAttachment{{Name: "b.xlsx", Text: "excel"}}
	got := prependPDFExcelText("desc", pdfs, excels)
	pdfIdx := strings.Index(got, "[PDF:a.pdf]")
	excelIdx := strings.Index(got, "[Excel:b.xlsx]")
	if pdfIdx < 0 || excelIdx < 0 {
		t.Fatalf("missing tags: %q", got)
	}
	if pdfIdx > excelIdx {
		t.Errorf("pdf should precede excel: %q", got)
	}
}

