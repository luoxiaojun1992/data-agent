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

const skillConfigNS = "skill"

// SkillConfigRepo implements SkillConfigRepository using MongoDB system_config.
type SkillConfigRepo struct {
	db *mongo.Database
}

// NewSkillConfigRepo creates a new MongoDB-backed skill config repository.
func NewSkillConfigRepo(db *mongo.Database) *SkillConfigRepo {
	return &SkillConfigRepo{db: db}
}

var _ repository.SkillConfigRepository = (*SkillConfigRepo)(nil)

func (r *SkillConfigRepo) List(ctx context.Context, skip, limit int64) ([]skill.SkillConfig, error) {
	opts := options.Find().SetSkip(skip).SetLimit(limit).SetSort(bson.D{{Key: "key", Value: 1}})
	cur, err := r.db.Collection("system_config").Find(ctx, bson.M{"ns": skillConfigNS}, opts)
	if err != nil {
		return nil, fmt.Errorf("skill config list: %w", err)
	}
	defer cur.Close(ctx)

	var out []skill.SkillConfig
	for cur.Next(ctx) {
		var doc struct {
			Key         string `bson:"key"`
			Value       string `bson:"value"`
			DisplayName string `bson:"display_name"`
			Description string `bson:"description"`
			Enabled     bool   `bson:"enabled"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, fmt.Errorf("skill config list decode: %w", err)
		}
		out = append(out, skill.SkillConfig{
			Name:        doc.Key,
			DisplayName: doc.DisplayName,
			Description: doc.Description,
			Enabled:     doc.Enabled,
			ConfigJSON:  doc.Value,
		})
	}
	return out, nil
}

// Count returns the total number of skill config documents.
func (r *SkillConfigRepo) Count(ctx context.Context) (int64, error) {
	n, err := r.db.Collection("system_config").CountDocuments(ctx, bson.M{"ns": skillConfigNS})
	if err != nil {
		return 0, fmt.Errorf("skill config count: %w", err)
	}
	return n, nil
}

func (r *SkillConfigRepo) Get(ctx context.Context, name string) (*skill.SkillConfig, error) {
	var doc struct {
		Key         string `bson:"key"`
		Value       string `bson:"value"`
		DisplayName string `bson:"display_name"`
		Description string `bson:"description"`
		Enabled     bool   `bson:"enabled"`
	}
	err := r.db.Collection("system_config").FindOne(ctx, bson.M{"ns": skillConfigNS, "key": name}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skill config get: %w", err)
	}
	return &skill.SkillConfig{
		Name:        doc.Key,
		DisplayName: doc.DisplayName,
		Description: doc.Description,
		Enabled:     doc.Enabled,
		ConfigJSON:  doc.Value,
	}, nil
}

// SearchByDescription returns up to `limit` enabled skills whose description
// matches the keyword (case-insensitive substring). Only the description
// field is matched — name and display_name are intentionally excluded.
func (r *SkillConfigRepo) SearchByDescription(ctx context.Context, keyword string, limit int) ([]skill.SkillConfig, error) {
	if limit <= 0 {
		limit = 5
	}
	filter := bson.M{
		"ns":          skillConfigNS,
		"enabled":     true,
		"description": bson.M{"$regex": regexp.QuoteMeta(keyword), "$options": "i"},
	}
	opts := options.Find().SetLimit(int64(limit))
	cur, err := r.db.Collection("system_config").Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("skill config search: %w", err)
	}
	defer cur.Close(ctx)

	var out []skill.SkillConfig
	for cur.Next(ctx) {
		var doc struct {
			Key         string `bson:"key"`
			Value       string `bson:"value"`
			DisplayName string `bson:"display_name"`
			Description string `bson:"description"`
			Enabled     bool   `bson:"enabled"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, fmt.Errorf("skill config search decode: %w", err)
		}
		out = append(out, skill.SkillConfig{
			Name:        doc.Key,
			DisplayName: doc.DisplayName,
			Description: doc.Description,
			Enabled:     doc.Enabled,
			ConfigJSON:  doc.Value,
		})
	}
	return out, nil
}

func (r *SkillConfigRepo) Upsert(ctx context.Context, cfg skill.SkillConfig) error {
	_, err := r.db.Collection("system_config").UpdateOne(ctx,
		bson.M{"ns": skillConfigNS, "key": cfg.Name},
		bson.M{"$set": bson.M{"value": cfg.ConfigJSON, "display_name": cfg.DisplayName, "description": cfg.Description, "enabled": cfg.Enabled}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("skill config upsert: %w", err)
	}
	return nil
}
