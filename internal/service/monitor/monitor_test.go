package monitor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
)

func TestSystemStats(t *testing.T) {
	stats := SystemStats()
	if stats == nil {
		t.Fatal("SystemStats() should not return nil")
	}

	requiredKeys := []string{"uptime_seconds", "go_version", "goroutines", "memory", "cpu_cores"}
	for _, key := range requiredKeys {
		if _, ok := stats[key]; !ok {
			t.Errorf("SystemStats() should contain key %q", key)
		}
	}

	if uptime, ok := stats["uptime_seconds"].(int); !ok || uptime < 0 {
		t.Errorf("uptime_seconds should be a non-negative int, got %v", stats["uptime_seconds"])
	}
	if gr, ok := stats["goroutines"].(int); !ok || gr < 0 {
		t.Errorf("goroutines should be a non-negative int, got %v", stats["goroutines"])
	}
	if cores, ok := stats["cpu_cores"].(int); !ok || cores <= 0 {
		t.Errorf("cpu_cores should be positive, got %v", stats["cpu_cores"])
	}
	if _, ok := stats["memory"].(map[string]interface{}); !ok {
		t.Fatal("memory should be a map")
	}
}

func TestSystemStats_UptimeIncreases(t *testing.T) {
	stats1 := SystemStats()
	time.Sleep(100 * time.Millisecond)
	stats2 := SystemStats()

	uptime1, _ := stats1["uptime_seconds"].(int)
	uptime2, _ := stats2["uptime_seconds"].(int)
	if uptime2 < uptime1 {
		t.Errorf("uptime should be monotonic: %d -> %d", uptime1, uptime2)
	}
}

func TestHandlerReturnsStats(t *testing.T) {
	mockStats := map[string]interface{}{
		"uptime_seconds": 42,
		"go_version":     "go1.25.0",
		"goroutines":     10,
		"memory": map[string]interface{}{
			"alloc_mb":       100,
			"total_alloc_mb": 200,
			"sys_mb":         300,
			"gc_cycles":      uint32(5),
		},
		"cpu_cores": 8,
	}
	patches := gomonkey.ApplyFuncReturn(SystemStats, mockStats)
	defer patches.Reset()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/stats", nil)

	Handler()(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
}

func TestHandlerResponseContainsAllKeys(t *testing.T) {
	mockStats := map[string]interface{}{
		"uptime_seconds": 0,
		"go_version":     "go1.25.0",
		"goroutines":     1,
		"memory": map[string]interface{}{
			"alloc_mb":       0,
			"total_alloc_mb": 0,
			"sys_mb":         0,
			"gc_cycles":      uint32(0),
		},
		"cpu_cores": 1,
	}
	patches := gomonkey.ApplyFuncReturn(SystemStats, mockStats)
	defer patches.Reset()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/stats", nil)

	Handler()(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, key := range []string{"uptime_seconds", "go_version", "goroutines", "memory", "cpu_cores"} {
		if !strings.Contains(body, key) {
			t.Errorf("response should contain key %q", key)
		}
	}
}

func TestSystemStats_ContainsGoVersion(t *testing.T) {
	goVer, ok := SystemStats()["go_version"].(string)
	if !ok || goVer == "" {
		t.Error("go_version should be a non-empty string")
	}
}

func TestSystemStats_ContainsGoroutines(t *testing.T) {
	gr, ok := SystemStats()["goroutines"].(int)
	if !ok || gr < 1 {
		t.Error("goroutines should be at least 1")
	}
}
