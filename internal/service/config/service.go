package config

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// service implements Service.
type service struct {
	sysConfig repository.SysConfigRepository
}

// NewService creates a system configuration service.
func NewService(sysConfig repository.SysConfigRepository) Service {
	return &service{sysConfig: sysConfig}
}

var _ Service = (*service)(nil)

func (s *service) GetAll(ctx context.Context, namespace string) ([]model.SystemConfig, error) {
	cfgs, err := s.sysConfig.GetAll(ctx, namespace)
	if err != nil {
		return nil, err
	}
	if cfgs == nil {
		return []model.SystemConfig{}, nil
	}
	return cfgs, nil
}

func (s *service) Upsert(ctx context.Context, namespace, key, value string) error {
	return s.sysConfig.Upsert(ctx, namespace, key, value)
}
