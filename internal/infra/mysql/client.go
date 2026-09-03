// Package mysql provides a health-check-only MySQL client singleton (SPEC-079).
// It deliberately does NOT build a connection pool and does NOT serve business
// queries — sql_executor opens its own on-demand gorm connection from the skill
// config DSN. This client exists solely so the global health check can ping
// MySQL and reflect its liveness in the online indicator.
package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Client is a minimal MySQL handle used exclusively for health pings.
type Client struct {
	db *sql.DB
}

// NewClient opens a MySQL handle from a DSN (e.g. the compose-injected
// MYSQL_DSN). It does not verify connectivity eagerly — the first Ping does.
// MaxOpenConns is pinned to 1 and idle conns disabled so no pool is ever built.
func NewClient(dsn string) (*Client, error) {
	if dsn == "" {
		return nil, fmt.Errorf("empty MySQL DSN")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	db.SetConnMaxLifetime(30 * time.Second)
	return &Client{db: db}, nil
}

// Ping verifies MySQL connectivity (aligned with the docker-compose healthcheck
// `mysqladmin ping -h localhost`).
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.db == nil {
		return fmt.Errorf("mysql client not initialized")
	}
	return c.db.PingContext(ctx)
}

// Close releases the underlying handle.
func (c *Client) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}
