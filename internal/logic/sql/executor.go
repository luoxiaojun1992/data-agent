package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

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
// against the configured MySQL database. Validation happens first — unsafe
// queries are rejected before any database interaction.
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

	// 3. Connect & execute
	db, err := sql.Open("mysql", config.DSN)
	if err != nil {
		return ExecResult{}, fmt.Errorf("sql_executor: connect: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), config.QueryTimeout)
	defer cancel()

	// Ping to verify connectivity + DNS before running the actual query.
	t0 := time.Now()
	if err := db.PingContext(ctx); err != nil {
		return ExecResult{
			Status:     "error",
			Query:      query,
			DurationMs: time.Since(t0).Milliseconds(),
		}, fmt.Errorf("sql_executor: mysql ping: %w", err)
	}

	start := time.Now()
	rows, err := db.QueryContext(ctx, query, params...)
	duration := time.Since(start).Milliseconds()
	if err != nil {
		return ExecResult{
			Status:     "error",
			Query:      query,
			DurationMs: duration,
		}, fmt.Errorf("sql_executor: query error: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return ExecResult{}, fmt.Errorf("sql_executor: columns: %w", err)
	}

	var resultRows [][]any
	count := 0
	for rows.Next() {
		if count >= config.MaxRows {
			break
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return ExecResult{}, fmt.Errorf("sql_executor: scan row %d: %w", count, err)
		}
		// Convert byte slices to strings for JSON readability
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		resultRows = append(resultRows, vals)
		count++
	}

	return ExecResult{
		Status:     "ok",
		Query:      query,
		Columns:    cols,
		Rows:       resultRows,
		RowCount:   count,
		DurationMs: duration,
	}, nil
}
