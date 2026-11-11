package test

import (
	"testing"
	"time"

	"github.com/ahmed/go-service/internal/helper"
)

func TestPasswordHashing(t *testing.T) {
	h := helper.NewHelper("secret", 1*time.Hour)
	password := "mypassword123"

	hash, err := h.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if err := h.CheckPassword(password, hash); err != nil {
		t.Errorf("CheckPassword failed on valid password: %v", err)
	}

	if err := h.CheckPassword("wrongpass", hash); err == nil {
		t.Error("CheckPassword expected error on wrong password, got nil")
	}
}

func TestJWTGenerationAndValidation(t *testing.T) {
	h := helper.NewHelper("secret-jwt-key", 1*time.Hour)

	token, err := h.GenerateToken(10, "user@example.com", "user")
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	claims, err := h.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.UserID != 10 {
		t.Errorf("expected UserID 10, got %d", claims.UserID)
	}
	if claims.Role != "user" {
		t.Errorf("expected Role 'user', got %s", claims.Role)
	}
}
