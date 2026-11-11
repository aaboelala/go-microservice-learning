package help

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
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

// UserClaims defines the claims structure extracted from the token issued by user_service.
type UserClaims struct {
	UserID uint   `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// HelpMethod provides helper methods for token validation.
type HelpMethod struct {
	SecretKey string
}

// NewHelpMethod creates a new HelpMethod instance with the given shared secret key.
func NewHelpMethod(secretKey string) *HelpMethod {
	return &HelpMethod{SecretKey: secretKey}
}

// ValidateToken validates a JWT token string using the HelpMethod's secret key.
func (h *HelpMethod) ValidateToken(tokenString string) (*UserClaims, error) {
	return ValidateToken(tokenString, h.SecretKey)
}

// ValidateToken parses and validates a JWT token using HMAC-SHA256 and extracts the user ID and role.
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

	// Fallback to JWT Subject claim (sub) if UserID is not directly populated in user_id
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
	authHeader = strings.TrimSpace(authHeader)
	if authHeader == "" {
		return "", ErrMissingAuthHeader
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", ErrInvalidAuthFormat
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", ErrEmptyToken
	}

	return token, nil
}

// GetUserID extracts the authenticated user's ID from Fiber locals (as uint or string).
func GetUserID(ctx *fiber.Ctx) (uint, error) {
	val := ctx.Locals("userID")
	if val == nil {
		return 0, errors.New("user ID not found in context")
	}

	switch v := val.(type) {
	case uint:
		if v == 0 {
			return 0, errors.New("invalid user ID")
		}
		return v, nil
	case string:
		id, err := strconv.ParseUint(v, 10, 64)
		if err != nil || id == 0 {
			return 0, errors.New("invalid user ID format")
		}
		return uint(id), nil
	default:
		return 0, errors.New("unsupported user ID type")
	}
}

// GetUserRole extracts the authenticated user's role from Fiber locals.
func GetUserRole(ctx *fiber.Ctx) string {
	if role, ok := ctx.Locals("role").(string); ok {
		return role
	}
	return ""
}
