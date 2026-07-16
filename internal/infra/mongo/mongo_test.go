package mongo

import (
	"testing"
)

func TestPackageCompiles(t *testing.T) {
	// Verify package types exist and compile
	var _ *Client
	var _ *UserRepository
	var _ *InviteRepository
	t.Log("mongo package compiled successfully")
}
