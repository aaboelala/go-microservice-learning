package helper

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// Claims represents JWT payload data.
type Claims struct {
	UserID uint   `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// Helper struct holds security dependencies like JWT secrets.
type Helper struct {
	jwtSecret     []byte
	jwtExpiration time.Duration
}

// NewHelper constructs a helper instance.
func NewHelper(jwtSecret string, jwtExpiration time.Duration) *Helper {
	return &Helper{
		jwtSecret:     []byte(jwtSecret),
		jwtExpiration: jwtExpiration,
	}
}

// HashPassword hashes a plaintext password using bcrypt.
func (h *Helper) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// CheckPassword checks if a plaintext password matches a bcrypt hash.
func (h *Helper) CheckPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// GenerateToken creates a signed JWT string.
func (h *Helper) GenerateToken(userID uint, email, role string) (string, error) {
	if userID == 0 || email == "" || role == "" {
		return "", errors.New("missing value to generate token")
	}
	now := time.Now()
	claims := &Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   email,
			ExpiresAt: jwt.NewNumericDate(now.Add(h.jwtExpiration)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.jwtSecret)
}

// ValidateToken verifies and parses a JWT token string.
func (h *Helper) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return h.jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}
