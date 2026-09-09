package jwtservice

import (
	"strconv"
	"time"

	"erent/internal/config"

	"github.com/golang-jwt/jwt/v5"
)

type JWTManager struct {
	secret     []byte
	issuer     string
	audience   string
	ttl        time.Duration
	RefreshTTL time.Duration
}

func NewJWTManager(configuration config.JWTConfig) *JWTManager {
	return &JWTManager{
		secret:     []byte(configuration.Secret),
		issuer:     configuration.Issuer,
		audience:   configuration.Audience,
		ttl:        configuration.AccessTTL,
		RefreshTTL: configuration.RefreshTTL,
	}
}

func (m *JWTManager) GenerateToken(userID uint64, username, role string) (string, error) {
	return m.GenerateTokenWithPermissions(userID, username, []string{role}, nil)
}

func (m *JWTManager) GenerateTokenWithPermissions(userID uint64, username string, roles, permissions []string) (string, error) {
	now := time.Now()
	claims := Claims{
		Username:    username,
		Role:        roles,
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   strconv.FormatUint(userID, 10),
			Audience:  jwt.ClaimStrings{m.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *JWTManager) ParseToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(*jwt.Token) (any, error) {
		return m.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithAudience(m.audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
