package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

// ErrDuplicate indicates a username already exists.
var ErrDuplicate = errors.New("用户名已存在")

// PasswordHasher abstracts password hashing.
type PasswordHasher interface {
	Hash(password string) (string, error)
}

// bcryptHasher implements PasswordHasher using bcrypt.
type bcryptHasher struct{}

func (bcryptHasher) Hash(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(bytes), nil
}

// NewBcryptHasher returns a PasswordHasher backed by bcrypt.
func NewBcryptHasher() PasswordHasher {
	return bcryptHasher{}
}

// service implements Service.
type service struct {
	repo   repository.UserRepository
	hasher PasswordHasher
}

// NewService creates a user management service.
func NewService(repo repository.UserRepository, hasher PasswordHasher) Service {
	return &service{repo: repo, hasher: hasher}
}

var _ Service = (*service)(nil)

func (s *service) List(ctx context.Context, role string, skip, limit int64, sortBy, sortOrder string) ([]model.User, int64, error) {
	return s.repo.ListSorted(ctx, role, skip, limit, sortBy, sortOrder)
}

func (s *service) Get(ctx context.Context, id string) (*model.User, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *service) Create(ctx context.Context, username, password, role string) (*model.User, error) {
	existing, _ := s.repo.FindByUsername(ctx, username)
	if existing != nil {
		return nil, ErrDuplicate
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}
	user := &model.User{
		Username:     username,
		PasswordHash: hash,
		Role:         model.UserRole(role),
		Status:       model.StatusEnabled,
	}
	if err := s.repo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}
	return user, nil
}

func (s *service) UpdateRole(ctx context.Context, id string, role model.UserRole) error {
	return s.repo.UpdateRole(ctx, id, role)
}

func (s *service) ToggleStatus(ctx context.Context, id string) error {
	user, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("用户不存在")
	}
	newStatus := model.StatusEnabled
	if user.Status == model.StatusEnabled {
		newStatus = model.StatusDisabled
	}
	return s.repo.UpdateStatus(ctx, id, newStatus)
}

func (s *service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
