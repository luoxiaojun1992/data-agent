package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/luoxiaojun1992/data-agent/internal/infra/vault"
)

// IMBindRepository implements repository.IMBindRepository backed by MongoDB.
type IMBindRepository struct {
	coll  *mongo.Collection
	vault *vault.Client
}

// NewIMBindRepository creates a new IMBindRepository.
func NewIMBindRepository(db *mongo.Database, vc *vault.Client) *IMBindRepository {
	return &IMBindRepository{coll: db.Collection("im_binds"), vault: vc}
}

// Get returns the IM binding record for the given user, or nil if none exists.
func (r *IMBindRepository) Get(ctx context.Context, userID string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := r.coll.FindOne(ctx, bson.M{"user_id": userID}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("get im bind: %w", err)
	}
	// Decrypt app_secret from Vault if exists
	if result != nil && r.vault != nil {
		if vaultKey, ok := result["vault_secret_path"].(string); ok && vaultKey != "" {
			if secret, err := r.vault.Retrieve(ctx, vaultKey); err == nil {
				result["app_secret"] = secret
			} else {
				result["app_secret"] = "••••••••••"
			}
			delete(result, "vault_secret_path")
		}
	}
	return result, nil
}

// Upsert creates or replaces the IM binding record for the given user.
func (r *IMBindRepository) Upsert(ctx context.Context, userID string, data map[string]interface{}) error {
	if data == nil {
		data = map[string]interface{}{}
	}
	// Encrypt app_secret to Vault
	if r.vault != nil {
		if secret, ok := data["app_secret"].(string); ok && secret != "" && secret != "••••••••••" {
			vaultPath := fmt.Sprintf("imbind/%s/app_secret", userID)
			if err := r.vault.Store(ctx, vaultPath, secret); err == nil {
				data["vault_secret_path"] = vaultPath
			}
		}
		delete(data, "app_secret")
	}
	data["user_id"] = userID
	_, err := r.coll.UpdateOne(ctx,
		bson.M{"user_id": userID},
		bson.M{"$set": data},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("upsert im bind: %w", err)
	}
	return nil
}

// Delete removes the IM binding record for the given user (idempotent).
func (r *IMBindRepository) Delete(ctx context.Context, userID string) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"user_id": userID})
	if err != nil {
		return fmt.Errorf("delete im bind: %w", err)
	}
	return nil
}
