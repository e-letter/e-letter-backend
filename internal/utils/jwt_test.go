package utils

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret-key-for-unit-testing-12345"

func TestGenerateToken_AccessToken(t *testing.T) {
	token, err := GenerateToken(testSecret, 1, "student@test.id", "student", "access", 30*time.Minute)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := ParseAndValidateToken(token, testSecret, "access")
	require.NoError(t, err)
	assert.Equal(t, 1, claims.UserID)
	assert.Equal(t, "student@test.id", claims.Email)
	assert.Equal(t, "student", claims.Role)
	assert.Equal(t, "access", claims.Type)
	assert.Equal(t, "Siswa", claims.MainRole)
	assert.True(t, claims.ExpiresAt.Time.After(time.Now()))
}

func TestGenerateToken_RefreshToken(t *testing.T) {
	token, err := GenerateToken(testSecret, 2, "teacher@test.id", "teacher", "refresh", 30*24*time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := ParseAndValidateToken(token, testSecret, "refresh")
	require.NoError(t, err)
	assert.Equal(t, 2, claims.UserID)
	assert.Equal(t, "refresh", claims.Type)
	assert.Equal(t, "Guru", claims.MainRole)

	remaining := time.Until(claims.ExpiresAt.Time)
	assert.Greater(t, remaining.Hours(), 29*24.0)
	assert.Less(t, remaining.Hours(), 31*24.0)
}

func TestGenerateTokenFull_WithSubRoles(t *testing.T) {
	token, err := GenerateTokenFull(testSecret, 3, "teacher@test.id", "teacher", "Guru", []string{"Walkes", "Mapel"}, true, "access", 30*time.Minute)
	require.NoError(t, err)

	claims, err := ParseAndValidateToken(token, testSecret, "access")
	require.NoError(t, err)
	assert.Equal(t, 3, claims.UserID)
	assert.Equal(t, "Guru", claims.MainRole)
	assert.Equal(t, []string{"Walkes", "Mapel"}, claims.SubRoles)
	assert.True(t, claims.IsProfileComplete)
}

func TestGenerateTokenFull_NilSubRoles(t *testing.T) {
	token, err := GenerateTokenFull(testSecret, 4, "test@test.id", "student", "Siswa", nil, false, "access", 30*time.Minute)
	require.NoError(t, err)

	claims, err := ParseAndValidateToken(token, testSecret, "access")
	require.NoError(t, err)
	assert.Equal(t, []string{}, claims.SubRoles)
}

func TestParseAndValidateToken_SigningMethodMismatch(t *testing.T) {
	headerB64 := base64.RawStdEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payloadB64 := base64.RawStdEncoding.EncodeToString([]byte(`{"userId":1,"role":"student","type":"access"}`))
	sigB64 := base64.RawStdEncoding.EncodeToString([]byte("signature"))
	rs256Token := headerB64 + "." + payloadB64 + "." + sigB64
	_, err := ParseAndValidateToken(rs256Token, testSecret, "access")
	assert.ErrorContains(t, err, "unexpected signing method")
}

func TestParseAndValidateToken_ExpiredToken(t *testing.T) {
	token, err := GenerateToken(testSecret, 5, "expired@test.id", "student", "access", -1*time.Hour)
	require.NoError(t, err)

	_, err = ParseAndValidateToken(token, testSecret, "access")
	assert.ErrorContains(t, err, "expired")
}

func TestParseAndValidateToken_WrongSecret(t *testing.T) {
	token, err := GenerateToken(testSecret, 6, "test@test.id", "student", "access", 30*time.Minute)
	require.NoError(t, err)

	_, err = ParseAndValidateToken(token, "wrong-secret", "access")
	assert.Error(t, err)
}

func TestParseAndValidateToken_WrongType(t *testing.T) {
	accessToken, err := GenerateToken(testSecret, 7, "test@test.id", "student", "access", 30*time.Minute)
	require.NoError(t, err)

	_, err = ParseAndValidateToken(accessToken, testSecret, "refresh")
	assert.ErrorContains(t, err, "unexpected token type")
}

func TestParseAndValidateToken_RefreshTokenNotAcceptedAsAccess(t *testing.T) {
	refreshToken, err := GenerateToken(testSecret, 8, "test@test.id", "teacher", "refresh", 30*24*time.Hour)
	require.NoError(t, err)

	_, err = ParseAndValidateToken(refreshToken, testSecret, "access")
	assert.ErrorContains(t, err, "unexpected token type")
}

func TestMapRoleToMainRole(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"student", "Siswa"},
		{"teacher", "Guru"},
		{"kepala_sekolah", "Kepsek"},
		{"admin", "Admin"},
		{"unknown", "Siswa"},
		{"", "Siswa"},
	}
	for _, tc := range tests {
		result := mapRoleToMainRole(tc.input)
		assert.Equal(t, tc.expected, result, "mapRoleToMainRole(%q)", tc.input)
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	token := "my-test-token-string"
	hash1 := HashToken(token)
	hash2 := HashToken(token)
	assert.Equal(t, hash1, hash2)
	assert.Len(t, hash1, 64)
}

func TestHashToken_DifferentInputs(t *testing.T) {
	h1 := HashToken("token-a")
	h2 := HashToken("token-b")
	assert.NotEqual(t, h1, h2)
}

func TestAccessTokenExpiry_30Minutes(t *testing.T) {
	token, err := GenerateToken(testSecret, 9, "test@test.id", "student", "access", 30*time.Minute)
	require.NoError(t, err)

	claims, err := ParseAndValidateToken(token, testSecret, "access")
	require.NoError(t, err)

	issued := claims.IssuedAt.Time
	expires := claims.ExpiresAt.Time
	diff := expires.Sub(issued)
	assert.Equal(t, 30*time.Minute, diff)
}

func TestRefreshTokenExpiry_30Days(t *testing.T) {
	token, err := GenerateToken(testSecret, 10, "test@test.id", "teacher", "refresh", 30*24*time.Hour)
	require.NoError(t, err)

	claims, err := ParseAndValidateToken(token, testSecret, "refresh")
	require.NoError(t, err)

	issued := claims.IssuedAt.Time
	expires := claims.ExpiresAt.Time
	diff := expires.Sub(issued)
	assert.Equal(t, 30*24*time.Hour, diff)
}

func TestTokenClaims_RegisteredClaims(t *testing.T) {
	now := time.Now()
	token, err := GenerateTokenFull(testSecret, 11, "test@test.id", "student", "Siswa", []string{}, false, "access", 30*time.Minute)
	require.NoError(t, err)

	parsed, err := jwt.ParseWithClaims(token, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(testSecret), nil
	})
	require.NoError(t, err)

	claims, ok := parsed.Claims.(*TokenClaims)
	require.True(t, ok)

	assert.WithinDuration(t, now, claims.IssuedAt.Time, 5*time.Second)
	assert.True(t, claims.ExpiresAt.Time.After(now))
	assert.False(t, claims.ExpiresAt.Time.After(now.Add(31*time.Minute)))
}

func TestTokenTypeEnforcement_AccessEndpoint(t *testing.T) {
	refreshTok, err := GenerateToken(testSecret, 12, "test@test.id", "student", "refresh", 30*24*time.Hour)
	require.NoError(t, err)

	_, err = ParseAndValidateToken(refreshTok, testSecret, "access")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected token type")

	accessTok, err := GenerateToken(testSecret, 12, "test@test.id", "student", "access", 30*time.Minute)
	require.NoError(t, err)

	_, err = ParseAndValidateToken(accessTok, testSecret, "access")
	assert.NoError(t, err)
}

func TestEmptySecret(t *testing.T) {
	token, err := GenerateToken("", 1, "test@test.id", "student", "access", 30*time.Minute)
	require.NoError(t, err)
	claims, err := ParseAndValidateToken(token, "", "access")
	require.NoError(t, err)
	assert.Equal(t, 1, claims.UserID)
	assert.Equal(t, "access", claims.Type)
}

func TestParseEmptyTokenString(t *testing.T) {
	_, err := ParseAndValidateToken("", testSecret, "access")
	assert.Error(t, err)
}
