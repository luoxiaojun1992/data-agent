package model

import "time"

// APICollectionStatus defines the approval status of an uploaded API collection.
type APICollectionStatus string

const (
	APICollectionPending  APICollectionStatus = "pending"
	APICollectionApproved APICollectionStatus = "approved"
	APICollectionRejected APICollectionStatus = "rejected"
)

// APICollection represents a user-uploaded OpenAPI 3.x specification.
type APICollection struct {
	ID          string               `json:"id" bson:"_id"`
	Name        string               `json:"name" bson:"name"`
	Description string               `json:"description" bson:"description"`
	Status      APICollectionStatus  `json:"status" bson:"status"`
	OpenAPISpec interface{}          `json:"openapi_spec" bson:"openapi_spec"`
	FileID      string               `json:"file_id" bson:"file_id"`     // SeaweedFS file ID
	UserID      string               `json:"user_id" bson:"user_id"`     // uploader
	APICount    int                  `json:"api_count" bson:"api_count"` // cached count of paths
	CreatedAt   time.Time            `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at" bson:"updated_at"`
}
