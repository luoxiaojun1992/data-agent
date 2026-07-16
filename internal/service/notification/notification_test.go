package notification

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
