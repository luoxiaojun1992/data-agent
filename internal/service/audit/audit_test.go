package audit

import (
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestNewService(t *testing.T) {
	db := &mongo.Database{}
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(db, "Collection", &coll)
	defer patches.Reset()

	s := NewService(db)
	if s == nil {
		t.Error("NewService should not return nil")
	}
}

func TestListParamsDefaults(t *testing.T) {
	p := ListParams{Limit: 20}
	if p.Limit != 20 {
		t.Errorf("Limit: got %d", p.Limit)
	}
}

func TestListResultType(t *testing.T) {
	r := &ListResult{Logs: nil, Total: 0}
	if r.Total != 0 {
		t.Errorf("Total: got %d", r.Total)
	}
}
