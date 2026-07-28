package sql

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	_ "github.com/go-sql-driver/mysql"
)

// ExecConfig holds the MySQL connection and execution parameters from skill config.
type ExecConfig struct {
	DSN          string        `json:"dsn"`
	MaxRows      int           `json:"max_rows"`
	QueryTimeout time.Duration `json:"query_timeout"`
}

// ExecResult is the outcome of a SQL execution.
type ExecResult struct {
	Status     string   `json:"status"`
	Query      string   `json:"query"`
	Columns    []string `json:"columns"`
	Rows       [][]any  `json:"rows"`
	RowCount   int      `json:"row_count"`
	DurationMs int64    `json:"duration_ms"`
	Message    string   `json:"message,omitempty"`
}

// Execute validates a SQL statement against safety rules, then executes it
// against the configured MySQL database using GORM.
func Execute(config ExecConfig, query string, params []any) (ExecResult, error) {
	// 1. Validate
	vr := Validate(query, params)
	if !vr.Allowed {
		return ExecResult{
			Status:  "rejected",
			Query:   query,
			Message: vr.Reason,
		}, fmt.Errorf("sql_executor: query rejected: %s", vr.Reason)
	}

	// 2. Apply defaults
	if config.MaxRows <= 0 {
		config.MaxRows = 100
	}
	if config.QueryTimeout <= 0 {
		config.QueryTimeout = 30 * time.Second
	}

	// 3. Open GORM connection
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       config.DSN,
		DefaultStringSize:         256,
		SkipInitializeWithVersion: false,
	}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return ExecResult{}, fmt.Errorf("sql_executor: connect: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return ExecResult{}, fmt.Errorf("sql_executor: get db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetConnMaxLifetime(config.QueryTimeout)
	defer sqlDB.Close()

	start := time.Now()

	// 4. Execute with raw SQL
	var results []map[string]interface{}
	tx := db.Raw(query, params...)
	if config.MaxRows > 0 {
		tx = tx.Limit(config.MaxRows)
	}
	if err := tx.Scan(&results).Error; err != nil {
		return ExecResult{
			Status:     "error",
			Query:      query,
			DurationMs: time.Since(start).Milliseconds(),
		}, fmt.Errorf("sql_executor: query error: %w", err)
	}

	// 5. Extract columns from first row
	var cols []string
	if len(results) > 0 {
		for k := range results[0] {
			cols = append(cols, k)
		}
	}

	// 6. Convert to 2D any slice
	rows := make([][]any, len(results))
	for i, r := range results {
		row := make([]any, len(cols))
		for j, c := range cols {
			row[j] = r[c]
		}
		rows[i] = row
	}

	return ExecResult{
		Status:     "ok",
		Query:      query,
		Columns:    cols,
		Rows:       rows,
		RowCount:   len(rows),
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}
