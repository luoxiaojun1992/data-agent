package mongo

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/feishu"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type FeishuConfigRepository struct {
	coll *mongo.Collection
}

func NewFeishuConfigRepository(db *mongo.Database) *FeishuConfigRepository {
	return &FeishuConfigRepository{coll: db.Collection("feishu_configs")}
}

func (r *FeishuConfigRepository) Create(ctx context.Context, cfg *feishu.Config) error {
	_, err := r.coll.InsertOne(ctx, feishuCfgToDoc(cfg))
	return err
}

func (r *FeishuConfigRepository) Get(ctx context.Context, id string) (*feishu.Config, error) {
	var d bson.M
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&d)
	if err != nil {
		return nil, err
	}
	return docToFeishuCfg(d), nil
}

func (r *FeishuConfigRepository) Update(ctx context.Context, cfg *feishu.Config) error {
	_, err := r.coll.ReplaceOne(ctx, bson.M{"_id": cfg.ID}, feishuCfgToDoc(cfg))
	return err
}

func (r *FeishuConfigRepository) Delete(ctx context.Context, id string) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *FeishuConfigRepository) ListByUser(ctx context.Context, userID string, skip, limit int64) ([]*feishu.Config, int64, error) {
	filter := bson.M{"user_id": userID}
	total, _ := r.coll.CountDocuments(ctx, filter)
	opts := options.Find().SetSkip(skip).SetLimit(limit).SetSort(bson.M{"created_at": -1})
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, 0, err
	}
	list := make([]*feishu.Config, len(docs))
	for i, d := range docs {
		list[i] = docToFeishuCfg(d)
	}
	return list, total, nil
}

func (r *FeishuConfigRepository) FindBySession(ctx context.Context, sessionID string) (*feishu.Config, error) {
	var d bson.M
	err := r.coll.FindOne(ctx, bson.M{"session_id": sessionID}).Decode(&d)
	if err != nil {
		return nil, err
	}
	return docToFeishuCfg(d), nil
}

func (r *FeishuConfigRepository) AllEnabled(ctx context.Context) ([]*feishu.Config, error) {
	cursor, err := r.coll.Find(ctx, bson.M{"enabled": true})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	list := make([]*feishu.Config, len(docs))
	for i, d := range docs {
		list[i] = docToFeishuCfg(d)
	}
	return list, nil
}

var _ repository.FeishuConfigRepository = (*FeishuConfigRepository)(nil)
