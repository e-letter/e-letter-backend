package utils

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenClaims struct {
	UserID            int      `json:"userId"`
	Email             string   `json:"email"`
	Role              string   `json:"role"`
	MainRole          string   `json:"mainRole"`
	SubRoles          []string `json:"subRoles"`
	IsProfileComplete bool     `json:"isProfileComplete"`
	Type              string   `json:"type"`
	jwt.RegisteredClaims
}

var (
	errInvalidToken = errors.New("invalid token")
	errExpiredToken = errors.New("token expired")
)

func GenerateToken(secret string, userID int, email string, role string, tokenType string, expiresIn time.Duration) (string, error) {
	return GenerateTokenFull(secret, userID, email, role, "", nil, false, tokenType, expiresIn)
}

func GenerateTokenFull(secret string, userID int, email string, role string, mainRole string, subRoles []string, isProfileComplete bool, tokenType string, expiresIn time.Duration) (string, error) {
	now := time.Now()
	if subRoles == nil {
		subRoles = []string{}
	}
	if mainRole == "" {
		mainRole = mapRoleToMainRole(role)
	}
	claims := TokenClaims{
		UserID:            userID,
		Email:             email,
		Role:              role,
		MainRole:          mainRole,
		SubRoles:          subRoles,
		IsProfileComplete: isProfileComplete,
		Type:              tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiresIn)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}
	return tokenString, nil
}

func mapRoleToMainRole(role string) string {
	switch role {
	case "student":
		return "Siswa"
	case "teacher":
		return "Guru"
	case "kepala_sekolah":
		return "Kepsek"
	case "admin":
		return "Admin"
	default:
		return "Siswa"
	}
}

func ParseAndValidateToken(tokenString, secret, expectedType string) (TokenClaims, error) {
	var claims TokenClaims

	token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return TokenClaims{}, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return TokenClaims{}, errInvalidToken
	}

	if expectedType != "" && claims.Type != expectedType {
		return TokenClaims{}, fmt.Errorf("unexpected token type: expected %s, got %s", expectedType, claims.Type)
	}

	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
		return TokenClaims{}, errExpiredToken
	}

	return claims, nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum)
}
