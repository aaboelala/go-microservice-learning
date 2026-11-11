// This file handles JWT token generation and verification.
//
// WHY JWT?
//
// In a microservices architecture, services need to verify user identity without
// calling back to the User Service on every request. JWTs solve this:
//   - The User Service issues a signed JWT at login
//   - The client sends it with every request (Authorization: Bearer <token>)
//   - ANY service can verify the JWT using the shared secret (or public key)
//   - No database lookup needed for authentication
//
// HOW does JWT work?
//
// A JWT has three parts: Header.Payload.Signature
//   - Header: algorithm (HS256) and token type (JWT)
//   - Payload: claims (user ID, role, expiration, etc.)
//   - Signature: HMAC-SHA256(Header + "." + Payload, secret)
//
// The signature ensures the token hasn't been tampered with. If anyone modifies
// the payload (e.g., changes role from "user" to "admin"), the signature won't match.
//
// TRADE-OFFS:
//   - JWTs cannot be revoked without a blacklist. If a token is stolen, it's valid
//     until it expires. Mitigation: short expiration + refresh tokens (not implemented yet).
//   - HS256 (symmetric) vs RS256 (asymmetric): HS256 is simpler (one shared secret).
//     RS256 is better for multi-service (public key can be shared without risk).
//     We use HS256 for simplicity. Switch to RS256 when you add more services.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims extends jwt.RegisteredClaims with our custom fields.
//
// RegisteredClaims provides standard JWT fields: Subject, ExpiresAt, IssuedAt, etc.
// We add UserID and Role because:
//   - UserID: so protected endpoints can look up the current user without a DB query
//   - Role: so authorization middleware can check permissions without a DB query
type Claims struct {
	UserID uint   `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// JWTService handles JWT generation and verification.
//
// WHY is it a struct instead of standalone functions?
//
// Because it needs configuration (secret, expiration). By putting these in a struct:
//   - We configure once at startup, use everywhere
//   - We can define an interface for testing (mock the JWT service)
//   - We avoid passing secret/expiration to every function call
//
// WHY is there an interface?
//
// The middleware and service layer depend on the JWTService interface.
// In tests, you can inject a mock that always returns a valid/invalid token.
type JWTService interface {
	GenerateToken(userID uint, email, role string) (string, error)
	ValidateToken(tokenString string) (*Claims, error)
}

type jwtService struct {
	secret     []byte
	expiration time.Duration
}

// NewJWTService creates a new JWT service with the given secret and expiration.
// Called once in main.go, the result is injected into the service and middleware.
func NewJWTService(secret string, expiration time.Duration) JWTService {
	return &jwtService{
		secret:     []byte(secret),
		expiration: expiration,
	}
}

// GenerateToken creates a signed JWT for the given user.
//
// The token contains:
//   - sub (subject): the user's email — standard JWT claim
//   - user_id: our custom claim for quick user lookup
//   - role: our custom claim for authorization
//   - exp (expires at): current time + configured expiration
//   - iat (issued at): current time
//
// The token is signed with HS256 (HMAC-SHA256) using the secret key.
func (s *jwtService) GenerateToken(userID uint, email, role string) (string, error) {
	now := time.Now()

	claims := &Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   email,
			ExpiresAt: jwt.NewNumericDate(now.Add(s.expiration)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// ValidateToken parses and validates a JWT token string.
//
// It checks:
//   - The signature is valid (token hasn't been tampered with)
//   - The token hasn't expired
//   - The signing method is HS256 (prevents algorithm confusion attacks)
//
// Algorithm confusion attack: If we don't check the signing method, an attacker
// could create a token signed with "none" or RS256 using the public key as the
// HMAC secret. By explicitly checking for HS256, we prevent this.
func (s *jwtService) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify the signing method is what we expect
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
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
