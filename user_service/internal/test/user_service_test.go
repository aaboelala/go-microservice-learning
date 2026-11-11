package test

import (
	"context"
	"testing"
	"time"

	"github.com/ahmed/go-service/internal/domain"
	"github.com/ahmed/go-service/internal/dto"
	"github.com/ahmed/go-service/internal/helper"
	"github.com/ahmed/go-service/internal/repository"
	"github.com/ahmed/go-service/internal/service"
)

// MockUserRepository implements repository.UserRepository in-memory for unit testing.
type MockUserRepository struct {
	users map[uint]*domain.User
}

func NewMockUserRepo() *MockUserRepository {
	return &MockUserRepository{users: make(map[uint]*domain.User)}
}

func (m *MockUserRepository) Create(ctx context.Context, user *domain.User) error {
	user.ID = uint(len(m.users) + 1)
	m.users[user.ID] = user
	return nil
}

func (m *MockUserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, repository.ErrUserNotFound
}

func (m *MockUserRepository) FindByID(ctx context.Context, id uint) (*domain.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, repository.ErrUserNotFound
	}
	return u, nil
}

func (m *MockUserRepository) Update(ctx context.Context, user *domain.User) error {
	m.users[user.ID] = user
	return nil
}

func (m *MockUserRepository) Delete(ctx context.Context, id uint) error {
	delete(m.users, id)
	return nil
}

func TestUserServiceRegisterAndLogin(t *testing.T) {
	repo := NewMockUserRepo()
	h := helper.NewHelper("secret", 1*time.Hour)
	svc := service.NewUserService(repo, h)

	// Test Registration
	regRes, err := svc.Register(context.Background(), dto.RegisterInput{
		Email:    "test@example.com",
		Password: "password123",
		Name:     "Test User",
	})

	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if regRes.Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", regRes.Email)
	}

	// Test Login
	loginRes, err := svc.Login(context.Background(), dto.LoginInput{
		Email:    "test@example.com",
		Password: "password123",
	})

	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if loginRes.Token == "" {
		t.Error("expected token, got empty string")
	}
}
