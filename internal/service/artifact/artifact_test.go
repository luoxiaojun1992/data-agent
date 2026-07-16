package artifact

import (
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestNewStorage(t *testing.T) {
	var db mongo.Database
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(&db, "Collection", &coll)
	defer patches.Reset()

	s := NewStorage(nil, &db)
	if s == nil {
		t.Error("NewStorage should not return nil")
	}
}
