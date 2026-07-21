package mongo

import (
	"context"
	"fmt"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// InviteRepository handles invite data access.
type InviteRepository struct {
	coll *mongo.Collection
}

// NewInviteRepository creates a new InviteRepository.
func NewInviteRepository(db *mongo.Database) *InviteRepository {
	return &InviteRepository{coll: db.Collection(model.CollInvites)}
}

// Create inserts a new invite.
func (r *InviteRepository) Create(ctx context.Context, invite *model.Invite) error {
	invite.ID = NewDomainID()
	invite.CreatedAt = time.Now()

	_, err := r.coll.InsertOne(ctx, inviteToDoc(invite))
	if err != nil {
		return fmt.Errorf("create invite: %w", err)
	}
	return nil
}

// FindByInviteID looks up an invite by its public invite_id.
func (r *InviteRepository) FindByInviteID(ctx context.Context, inviteID string) (*model.Invite, error) {
	var d bson.M
	err := r.coll.FindOne(ctx, bson.M{"invite_id": inviteID}).Decode(&d)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("find invite by invite_id: %w", err)
	}
	return docToInvite(d), nil
}

// FindByTokenHash looks up an invite by its token hash for fast verification.
func (r *InviteRepository) FindByTokenHash(ctx context.Context, hash string) (*model.Invite, error) {
	var d bson.M
	err := r.coll.FindOne(ctx, bson.M{"token_hash": hash}).Decode(&d)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("find invite by token_hash: %w", err)
	}
	return docToInvite(d), nil
}

// MarkAccepted updates an invite to accepted status and records who accepted it.
func (r *InviteRepository) MarkAccepted(ctx context.Context, inviteID string, userID string) error {
	now := time.Now()
	result, err := r.coll.UpdateOne(ctx,
		bson.M{"invite_id": inviteID, "status": model.InviteStatusPending},
		bson.M{"$set": bson.M{
			"status":      model.InviteStatusAccepted,
			"accepted_at": now,
			"accepted_by": userID,
		}},
	)
	if err != nil {
		return fmt.Errorf("mark invite accepted: %w", err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("invite not found or not pending")
	}
	return nil
}

// Revoke updates an invite status from pending to revoked.
func (r *InviteRepository) Revoke(ctx context.Context, inviteID string) error {
	result, err := r.coll.UpdateOne(ctx,
		bson.M{"invite_id": inviteID, "status": model.InviteStatusPending},
		bson.M{"$set": bson.M{"status": model.InviteStatusRevoked}},
	)
	if err != nil {
		return fmt.Errorf("revoke invite: %w", err)
	}
	if result.MatchedCount == 0 {
		return nil // Idempotent: not found or already revoked/expired/accepted
	}
	return nil
}

// List returns paginated invites, optionally filtered by creator.
func (r *InviteRepository) List(ctx context.Context, createdBy string, skip, limit int64) ([]model.Invite, int64, error) {
	filter := bson.M{}
	if createdBy != "" {
		filter["created_by"] = createdBy
	}

	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count invites: %w", err)
	}

	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("list invites: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, 0, fmt.Errorf("decode invites: %w", err)
	}

	invites := make([]model.Invite, len(docs))
	for i, d := range docs {
		invites[i] = *docToInvite(d)
	}

	return invites, total, nil
}
