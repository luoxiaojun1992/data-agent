package monitor

import (
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
)

var startTime = time.Now()

// SystemStats returns system-level statistics.
func SystemStats() map[string]interface{} {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return map[string]interface{}{
		"uptime_seconds": int(time.Since(startTime).Seconds()),
		"go_version":     runtime.Version(),
		"goroutines":     runtime.NumGoroutine(),
		"memory": map[string]interface{}{
			"alloc_mb":       mem.Alloc / 1024 / 1024,
			"total_alloc_mb": mem.TotalAlloc / 1024 / 1024,
			"sys_mb":         mem.Sys / 1024 / 1024,
			"gc_cycles":      mem.NumGC,
		},
		"cpu_cores": runtime.NumCPU(),
	}
}

// Handler returns a Gin handler for the system stats endpoint.
func Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, SystemStats())
	}
}
