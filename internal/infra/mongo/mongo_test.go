package mongo

import (
	"context"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestUpsertOne(t *testing.T) {
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(&coll, "UpdateOne", &mongo.UpdateResult{MatchedCount: 1}, nil)
	defer patches.Reset()

	err := UpsertOne(context.Background(), &coll, bson.M{"k": "v"}, struct{}{})
	if err != nil {
		t.Fatalf("UpsertOne error: %v", err)
	}
}

func TestDeleteOne(t *testing.T) {
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(&coll, "DeleteOne", &mongo.DeleteResult{DeletedCount: 1}, nil)
	defer patches.Reset()

	err := DeleteOne(context.Background(), &coll, bson.M{"k": "v"})
	if err != nil {
		t.Fatalf("DeleteOne error: %v", err)
	}
}

func TestDeleteOne_Idempotent(t *testing.T) {
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(&coll, "DeleteOne", &mongo.DeleteResult{DeletedCount: 0}, nil)
	defer patches.Reset()

	err := DeleteOne(context.Background(), &coll, bson.M{"k": "v"})
	if err != nil {
		t.Fatalf("DeleteOne should be idempotent: %v", err)
	}
}

func TestNewUserRepository(t *testing.T) {
	db := &mongo.Database{}
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(db, "Collection", &coll)
	defer patches.Reset()

	repo := NewUserRepository(db)
	if repo == nil {
		t.Error("NewUserRepository should not return nil")
	}
}

func TestNewInviteRepository(t *testing.T) {
	db := &mongo.Database{}
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(db, "Collection", &coll)
	defer patches.Reset()

	repo := NewInviteRepository(db)
	if repo == nil {
		t.Error("NewInviteRepository should not return nil")
	}
}
