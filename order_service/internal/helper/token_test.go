package helper_test

import (
	"fmt"
	"testing"
	"time"

	"order_service/internal/helper"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "my-test-secret-key-12345"

func generateTestToken(secret string, claims *helper.UserClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func TestValidateToken_Success(t *testing.T) {
	claims := &helper.UserClaims{
		UserID: 42,
		Role:   "customer",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	tokenStr, err := generateTestToken(testSecret, claims)
	require.NoError(t, err)

	th := helper.NewTokenHelper(testSecret)
	parsedClaims, err := th.ValidateToken(tokenStr)
	require.NoError(t, err)
	assert.Equal(t, uint(42), parsedClaims.UserID)
	assert.Equal(t, "customer", parsedClaims.Role)
}

func TestValidateToken_FallbackToSubject(t *testing.T) {
	claims := &helper.UserClaims{
		Role: "customer",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "99",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}

	tokenStr, err := generateTestToken(testSecret, claims)
	require.NoError(t, err)

	th := helper.NewTokenHelper(testSecret)
	parsedClaims, err := th.ValidateToken(tokenStr)
	require.NoError(t, err)
	assert.Equal(t, uint(99), parsedClaims.UserID)
}

func TestValidateToken_Expired(t *testing.T) {
	claims := &helper.UserClaims{
		UserID: 42,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)), // Expired
		},
	}

	tokenStr, err := generateTestToken(testSecret, claims)
	require.NoError(t, err)

	th := helper.NewTokenHelper(testSecret)
	_, err = th.ValidateToken(tokenStr)
	assert.ErrorIs(t, err, helper.ErrTokenExpired)
}

func TestValidateToken_InvalidSignature(t *testing.T) {
	claims := &helper.UserClaims{
		UserID: 42,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}

	tokenStr, err := generateTestToken("wrong-secret-key", claims)
	require.NoError(t, err)

	th := helper.NewTokenHelper(testSecret)
	_, err = th.ValidateToken(tokenStr)
	assert.ErrorIs(t, err, helper.ErrInvalidToken)
}

func TestExtractTokenFromHeader(t *testing.T) {
	tests := []struct {
		name        string
		header      string
		expected    string
		expectedErr error
	}{
		{
			name:        "Valid Bearer Header",
			header:      "Bearer abc.def.ghi",
			expected:    "abc.def.ghi",
			expectedErr: nil,
		},
		{
			name:        "Missing Header",
			header:      "",
			expected:    "",
			expectedErr: helper.ErrMissingAuthHeader,
		},
		{
			name:        "Invalid Format - No Bearer",
			header:      "Basic abcdef",
			expected:    "",
			expectedErr: helper.ErrInvalidAuthFormat,
		},
		{
			name:        "Empty Token",
			header:      "Bearer ",
			expected:    "",
			expectedErr: helper.ErrEmptyToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := helper.ExtractTokenFromHeader(tt.header)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, token)
			}
		})
	}
}

func TestValidateToken_ZeroUserID(t *testing.T) {
	claims := &helper.UserClaims{
		UserID: 0,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}

	tokenStr, err := generateTestToken(testSecret, claims)
	require.NoError(t, err)

	th := helper.NewTokenHelper(testSecret)
	_, err = th.ValidateToken(tokenStr)
	assert.ErrorIs(t, err, helper.ErrInvalidClaims)
	assert.Contains(t, fmt.Sprint(err), "missing or zero")
}

