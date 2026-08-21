package mongo

import (
	"context"
	"fmt"
	"regexp"

	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// skillConfigCollection is the standalone collection for skill configs
// (migrated out of the legacy "system_config" collection).
const skillConfigCollection = "skill_configs"

// SkillConfigRepo implements SkillConfigRepository using MongoDB skill_configs.
type SkillConfigRepo struct {
	db *mongo.Database
}

// NewSkillConfigRepo creates a new MongoDB-backed skill config repository.
func NewSkillConfigRepo(db *mongo.Database) *SkillConfigRepo {
	return &SkillConfigRepo{db: db}
}

var _ repository.SkillConfigRepository = (*SkillConfigRepo)(nil)

func (r *SkillConfigRepo) coll() *mongo.Collection {
	return r.db.Collection(skillConfigCollection)
}

func (r *SkillConfigRepo) List(ctx context.Context, skip, limit int64) ([]skill.SkillConfig, error) {
	opts := options.Find().SetSkip(skip).SetLimit(limit).SetSort(bson.D{{Key: "name", Value: 1}})
	cur, err := r.coll().Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, fmt.Errorf("skill config list: %w", err)
	}
	defer cur.Close(ctx)

	var out []skill.SkillConfig
	for cur.Next(ctx) {
		var doc skillConfigDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, fmt.Errorf("skill config list decode: %w", err)
		}
		out = append(out, doc.toSkillConfig())
	}
	return out, nil
}

// Count returns the total number of skill config documents.
func (r *SkillConfigRepo) Count(ctx context.Context) (int64, error) {
	n, err := r.coll().CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("skill config count: %w", err)
	}
	return n, nil
}

func (r *SkillConfigRepo) Get(ctx context.Context, name string) (*skill.SkillConfig, error) {
	var doc skillConfigDoc
	err := r.coll().FindOne(ctx, bson.M{"name": name}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skill config get: %w", err)
	}
	cfg := doc.toSkillConfig()
	return &cfg, nil
}

// SearchByDescription returns up to `limit` enabled skills whose description
// matches the keyword (case-insensitive substring). Only the description
// field is matched — name and display_name are intentionally excluded.
func (r *SkillConfigRepo) SearchByDescription(ctx context.Context, keyword string, limit int) ([]skill.SkillConfig, error) {
	if limit <= 0 {
		limit = 5
	}
	filter := bson.M{
		"enabled":     true,
		"description": bson.M{"$regex": regexp.QuoteMeta(keyword), "$options": "i"},
	}
	opts := options.Find().SetLimit(int64(limit))
	cur, err := r.coll().Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("skill config search: %w", err)
	}
	defer cur.Close(ctx)

	var out []skill.SkillConfig
	for cur.Next(ctx) {
		var doc skillConfigDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, fmt.Errorf("skill config search decode: %w", err)
		}
		out = append(out, doc.toSkillConfig())
	}
	return out, nil
}

func (r *SkillConfigRepo) Upsert(ctx context.Context, cfg skill.SkillConfig) error {
	_, err := r.coll().UpdateOne(ctx,
		bson.M{"name": cfg.Name},
		bson.M{"$set": bson.M{"value": cfg.ConfigJSON, "display_name": cfg.DisplayName, "description": cfg.Description, "enabled": cfg.Enabled}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("skill config upsert: %w", err)
	}
	return nil
}

// skillConfigDoc is the persisted shape of a skill config document.
type skillConfigDoc struct {
	Name        string `bson:"name"`
	Value       string `bson:"value"`
	DisplayName string `bson:"display_name"`
	Description string `bson:"description"`
	Enabled     bool   `bson:"enabled"`
}

func (d skillConfigDoc) toSkillConfig() skill.SkillConfig {
	return skill.SkillConfig{
		Name:        d.Name,
		DisplayName: d.DisplayName,
		Description: d.Description,
		Enabled:     d.Enabled,
		ConfigJSON:  d.Value,
	}
}
