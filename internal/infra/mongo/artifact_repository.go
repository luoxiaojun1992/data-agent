package mongo

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/artifact"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type ArtifactRepository struct {
	coll *mongo.Collection
}

func NewArtifactRepository(db *mongo.Database) *ArtifactRepository {
	return &ArtifactRepository{coll: db.Collection("artifacts")}
}

func (r *ArtifactRepository) Create(ctx context.Context, a *artifact.Artifact) error {
	_, err := r.coll.InsertOne(ctx, artifactToDoc(a))
	return err
}

func (r *ArtifactRepository) FindByID(ctx context.Context, id string) (*artifact.Artifact, error) {
	var d bson.M
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&d)
	if err != nil {
		return nil, err
	}
	return docToArtifact(d), nil
}

func (r *ArtifactRepository) Delete(ctx context.Context, id string) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *ArtifactRepository) ListBySession(ctx context.Context, sessionID string) ([]*artifact.Artifact, error) {
	cursor, err := r.coll.Find(ctx, bson.M{"session_id": sessionID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	list := make([]*artifact.Artifact, len(docs))
	for i, d := range docs {
		list[i] = docToArtifact(d)
	}
	return list, nil
}

func (r *ArtifactRepository) ListByTask(ctx context.Context, taskID string) ([]*artifact.Artifact, error) {
	cursor, err := r.coll.Find(ctx, bson.M{"task_id": taskID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	list := make([]*artifact.Artifact, len(docs))
	for i, d := range docs {
		list[i] = docToArtifact(d)
	}
	return list, nil
}

var _ repository.ArtifactRepository = (*ArtifactRepository)(nil)
