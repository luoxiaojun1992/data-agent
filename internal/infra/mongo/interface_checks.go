// Package mongo implements repository interfaces for MongoDB storage.
package mongo

import (
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// Compile-time interface satisfaction checks.
var (
	_ repository.UserRepository       = (*UserRepository)(nil)
	_ repository.InviteRepository     = (*InviteRepository)(nil)
	_ repository.RoleRepository       = (*RoleRepository)(nil)
	_ repository.SysConfigRepository  = (*SystemConfigRepository)(nil)
)
