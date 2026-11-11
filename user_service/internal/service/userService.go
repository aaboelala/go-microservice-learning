package service

import (
	"context"
	"errors"

	"github.com/ahmed/go-service/internal/domain"
	"github.com/ahmed/go-service/internal/dto"
	"github.com/ahmed/go-service/internal/helper"
	"github.com/ahmed/go-service/internal/repository"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserAlreadyExists  = errors.New("user with this email already exists")
)

// UserService defines business operations.
type UserService interface {
	Register(ctx context.Context, input dto.RegisterInput) (*dto.UserResponse, error)
	Login(ctx context.Context, input dto.LoginInput) (*dto.LoginResponse, error)
	GetByID(ctx context.Context, id uint) (*dto.UserResponse, error)
	Update(ctx context.Context, id uint, input dto.UpdateUserInput) (*dto.UserResponse, error)
	Delete(ctx context.Context, id uint) error
}

type userService struct {
	repo   repository.UserRepository
	helper *helper.Helper
}

// NewUserService creates a new UserService implementation.
func NewUserService(repo repository.UserRepository, h *helper.Helper) UserService {
	return &userService{
		repo:   repo,
		helper: h,
	}
}

func (s *userService) Register(ctx context.Context, input dto.RegisterInput) (*dto.UserResponse, error) {
	hashedPassword, err := s.helper.HashPassword(input.Password)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		Email:        input.Email,
		PasswordHash: hashedPassword,
		Name:         input.Name,
		Role:         "user",
	}

	if err := s.repo.Create(ctx, user); err != nil {
		if errors.Is(err, repository.ErrDuplicateEmail) {
			return nil, ErrUserAlreadyExists
		}
		return nil, err
	}

	res := mapToUserResponse(user)
	return &res, nil
}

func (s *userService) Login(ctx context.Context, input dto.LoginInput) (*dto.LoginResponse, error) {
	user, err := s.repo.FindByEmail(ctx, input.Email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if err := s.helper.CheckPassword(input.Password, user.PasswordHash); err != nil {
		return nil, ErrInvalidCredentials
	}

	token, err := s.helper.GenerateToken(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	return &dto.LoginResponse{
		Token: token,
		User:  mapToUserResponse(user),
	}, nil
}

func (s *userService) GetByID(ctx context.Context, id uint) (*dto.UserResponse, error) {
	user, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	res := mapToUserResponse(user)
	return &res, nil
}

func (s *userService) Update(ctx context.Context, id uint, input dto.UpdateUserInput) (*dto.UserResponse, error) {
	user, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Name != "" {
		user.Name = input.Name
	}
	if input.Email != "" {
		user.Email = input.Email
	}

	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}

	res := mapToUserResponse(user)
	return &res, nil
}

func (s *userService) Delete(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}

func mapToUserResponse(user *domain.User) dto.UserResponse {
	return dto.UserResponse{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}
