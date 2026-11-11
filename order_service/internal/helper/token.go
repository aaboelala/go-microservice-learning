package helper

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrEmptyToken        = errors.New("token cannot be empty")
	ErrEmptySecret       = errors.New("secret key cannot be empty")
	ErrInvalidToken      = errors.New("invalid token")
	ErrTokenExpired      = errors.New("token has expired")
	ErrUnexpectedSigning = errors.New("unexpected signing method")
	ErrInvalidClaims     = errors.New("invalid token claims")
	ErrMissingAuthHeader = errors.New("missing authorization header")
	ErrInvalidAuthFormat = errors.New("invalid authorization header format, must be Bearer <token>")
)

// UserClaims defines the JWT claims structure holding user ID and role
// as issued by the user_service.
type UserClaims struct {
	UserID uint   `json:"user_id"`
	ID     uint   `json:"id,omitempty"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// TokenHelper provides helper methods for token validation.
type TokenHelper struct {
	SecretKey string
}

// NewTokenHelper creates a new TokenHelper instance with the given shared secret key.
func NewTokenHelper(secretKey string) *TokenHelper {
	return &TokenHelper{SecretKey: secretKey}
}

// ValidateToken validates a JWT token string using the helper's shared secret key.
func (h *TokenHelper) ValidateToken(tokenString string) (*UserClaims, error) {
	return ValidateToken(tokenString, h.SecretKey)
}

// ValidateToken parses and validates a JWT token, verifying the HMAC signature with secretKey.
func ValidateToken(tokenString string, secretKey string) (*UserClaims, error) {
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return nil, ErrEmptyToken
	}
	if secretKey == "" {
		return nil, ErrEmptySecret
	}

	claims := &UserClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: %v", ErrUnexpectedSigning, token.Header["alg"])
		}
		return []byte(secretKey), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	// Normalize user ID from various possible claim formats
	if claims.UserID == 0 && claims.ID != 0 {
		claims.UserID = claims.ID
	}

	// Fallback to JWT Subject claim (sub) if UserID is not explicitly populated
	if claims.UserID == 0 && claims.Subject != "" {
		if id, parseErr := strconv.ParseUint(claims.Subject, 10, 64); parseErr == nil {
			claims.UserID = uint(id)
		}
	}

	if claims.UserID == 0 {
		return nil, fmt.Errorf("%w: user ID is missing or zero", ErrInvalidClaims)
	}

	return claims, nil
}

// ExtractTokenFromHeader extracts the Bearer token string from an Authorization header.
func ExtractTokenFromHeader(authHeader string) (string, error) {
	trimmed := strings.TrimSpace(authHeader)
	if trimmed == "" {
		return "", ErrMissingAuthHeader
	}

	parts := strings.SplitN(trimmed, " ", 2)
	if len(parts) == 1 && strings.EqualFold(parts[0], "bearer") {
		return "", ErrEmptyToken
	}
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", ErrInvalidAuthFormat
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", ErrEmptyToken
	}

	return token, nil
}
