package mongo

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// NewDomainID generates a new domain entity ID as a 24-char hex string.
// It is used by infra repositories when creating new documents, so that
// ID generation stays inside the infra layer and is invisible to callers
// (domain/service/handler never import mongo-driver).
func NewDomainID() string {
	return primitive.NewObjectID().Hex()
}

// ObjectIDFromDomainID converts a domain string ID to a mongo ObjectID.
// Returns an error if id is empty or not a valid ObjectID hex.
//
// Reserved for SPEC-057: once domain structs lose their bson tags, infra
// repositories will build bson.M manually and need to turn the string ID
// back into an ObjectID for queries/updates. In SPEC-056 the _id is stored
// as a plain string (struct still carries bson:"_id" tag), so repos query
// by string directly and do not call this helper yet.
func ObjectIDFromDomainID(id string) (primitive.ObjectID, error) {
	if id == "" {
		return primitive.NilObjectID, fmt.Errorf("empty id")
	}
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return primitive.NilObjectID, fmt.Errorf("invalid object id %q: %w", id, err)
	}
	return oid, nil
}

// DomainIDFromObjectID converts a mongo ObjectID to a domain string ID.
//
// Reserved for SPEC-057 (see ObjectIDFromDomainID doc).
func DomainIDFromObjectID(oid primitive.ObjectID) string {
	return oid.Hex()
}
