package role

import (
	"context"
	"fmt"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// service implements Service.
type service struct {
	repo repository.RoleRepository
}

// NewService creates a role management service.
func NewService(repo repository.RoleRepository) Service {
	return &service{repo: repo}
}

var _ Service = (*service)(nil)

func (s *service) List(ctx context.Context) ([]model.Role, error) {
	return s.repo.List(ctx)
}

func (s *service) ListPermissions() []model.PermissionInfo {
	return model.GetAllPermissions()
}

func (s *service) Create(ctx context.Context, name string, permissions []string) (*model.Role, error) {
	r := &model.Role{Name: name, Permissions: permissions}
	if err := s.repo.Create(ctx, r); err != nil {
		return nil, fmt.Errorf("创建角色失败: %w", err)
	}
	return r, nil
}

func (s *service) Update(ctx context.Context, id string, permissions []string) error {
	return s.repo.Update(ctx, id, permissions)
}

func (s *service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
