package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Service handles audit log queries.
type Service struct {
	coll *mongo.Collection
}

// NewService creates an audit log service.
func NewService(db *mongo.Database) *Service {
	return &Service{coll: db.Collection(model.CollAuditLogs)}
}

// ListParams are the filter parameters for listing audit logs.
type ListParams struct {
	Action string
	UserID string
	Start  string // ISO date
	End    string // ISO date
	Skip   int64
	Limit  int64
}

// ListResult contains the audit log list and total count.
type ListResult struct {
	Logs  []model.AuditLog `json:"logs"`
	Total int64            `json:"total"`
}

// List returns audit logs matching the filter params.
func (s *Service) List(p ListParams) (*ListResult, error) {
	filter, err := buildAuditFilter(p)
	if err != nil {
		return nil, err
	}
	p.Limit = normalizeAuditLimit(p.Limit)

	total, err := s.coll.CountDocuments(context.Background(), filter)
	if err != nil {
		return nil, err
	}

	opts := options.Find().SetSort(bson.M{"created_at": -1}).SetSkip(p.Skip).SetLimit(p.Limit)
	cursor, err := s.coll.Find(context.Background(), filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var logs []model.AuditLog
	if err := cursor.All(context.Background(), &logs); err != nil {
		return nil, err
	}
	if logs == nil {
		logs = []model.AuditLog{}
	}

	return &ListResult{Logs: logs, Total: total}, nil
}

// buildAuditFilter builds the MongoDB filter from ListParams.
func buildAuditFilter(p ListParams) (bson.M, error) {
	filter := bson.M{}
	if p.Action != "" {
		filter["action"] = p.Action
	}
	if p.UserID != "" {
		filter["user_id"] = p.UserID
	}
	dateFilter, err := buildDateFilter(p.Start, p.End)
	if err != nil {
		return nil, err
	}
	if len(dateFilter) > 0 {
		filter["created_at"] = dateFilter
	}
	return filter, nil
}

// buildDateFilter builds a date range filter from start/end strings.
func buildDateFilter(start, end string) (bson.M, error) {
	if start == "" && end == "" {
		return nil, nil
	}
	dateFilter := bson.M{}
	if start != "" {
		t, err := time.Parse("2006-01-02", start)
		if err != nil {
			return nil, fmt.Errorf("invalid start date %q: must be YYYY-MM-DD", start)
		}
		dateFilter["$gte"] = t
	}
	if end != "" {
		t, err := time.Parse("2006-01-02", end)
		if err != nil {
			return nil, fmt.Errorf("invalid end date %q: must be YYYY-MM-DD", end)
		}
		dateFilter["$lt"] = t.Add(24 * time.Hour)
	}
	return dateFilter, nil
}

// normalizeAuditLimit clamps the limit parameter to valid bounds.
func normalizeAuditLimit(limit int64) int64 {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

// ExportParams defines the export request.
type ExportParams struct {
	Action string `json:"action"`
	UserID string `json:"user_id"`
	Start  string `json:"start"`
	End    string `json:"end"`
	Limit  int64  `json:"limit"`
	Format string `json:"format"`
}
