package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type TokenClaims struct {
	UserID int    `json:"userId"`
	Email  string `json:"email"`
	Role   int    `json:"role"`
	Type   string `json:"type"`
	Exp    int64  `json:"exp"`
	Iat    int64  `json:"iat"`
}

var (
	errInvalidToken = errors.New("invalid token")
	errExpiredToken = errors.New("token expired")
)

func GenerateToken(secret string, claims TokenClaims) (string, error) {
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	headerEncoded := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadEncoded := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := headerEncoded + "." + payloadEncoded

	signature := signHS256(signingInput, secret)
	return signingInput + "." + signature, nil
}

func ParseAndValidateToken(token, secret, expectedType string) (TokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return TokenClaims{}, errInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := signHS256(signingInput, secret)
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return TokenClaims{}, errInvalidToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return TokenClaims{}, errInvalidToken
	}

	var claims TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return TokenClaims{}, errInvalidToken
	}

	if expectedType != "" && claims.Type != expectedType {
		return TokenClaims{}, fmt.Errorf("unexpected token type: %s", claims.Type)
	}

	if claims.Exp <= time.Now().Unix() {
		return TokenClaims{}, errExpiredToken
	}

	return claims, nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum)
}

func signHS256(input, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(input))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
