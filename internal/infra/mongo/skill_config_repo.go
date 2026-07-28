package mongo

import (
	"context"
	"fmt"

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

func (r *SkillConfigRepo) List(ctx context.Context) ([]skill.SkillConfig, error) {
	cur, err := r.db.Collection("system_config").Find(ctx, bson.M{"ns": skillConfigNS})
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
