package auth

import (
	"context"
	"errors"
	"time"

	"github.com/DWHuang99/erent/internal/dto/request"
	jwtservice "github.com/DWHuang99/erent/internal/middleware/jwt"
	rdb "github.com/DWHuang99/erent/internal/middleware/redis"
	"github.com/DWHuang99/erent/internal/modules/user"
	"github.com/DWHuang99/erent/internal/security"
	"github.com/redis/go-redis/v9"
)

var (
	ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")
	ErrUserDisabled        = errors.New("user is disabled")
)

type UserAuthRepository interface {
	GetUserAuthByUsername(context.Context, string) (*user.UserAuth, error)
	GetUserAuthByID(context.Context, uint64) (*user.UserAuth, error)
}

type AuthService struct {
	userRepository UserAuthRepository
	jwtManager     *jwtservice.JWTManager
	redisClient    *redis.Client
	dummyHash      string
}

func NewService(userRepository UserAuthRepository, jwtManager *jwtservice.JWTManager, redisClient *redis.Client) *AuthService {
	dummyHash, _ := security.HashPassword("not-a-real-password")
	return &AuthService{
		userRepository: userRepository,
		jwtManager:     jwtManager,
		redisClient:    redisClient,
		dummyHash:      dummyHash,
	}
}

func (s *AuthService) Login(ctx context.Context, loginRequest request.LoginRequest) (string, string, bool, error) {
	userAuth, err := s.userRepository.GetUserAuthByUsername(ctx, loginRequest.Username)
	if errors.Is(err, user.ErrNotFound) {
		security.VerifyPassword(loginRequest.Password, s.dummyHash)
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	if !security.VerifyPassword(loginRequest.Password, userAuth.PasswordHash) {
		return "", "", false, nil
	}
	if !userAuth.IsActive {
		return "", "", true, ErrUserDisabled
	}
	accessToken, refreshToken, err := s.issueTokensForUser(ctx, userAuth)
	return accessToken, refreshToken, true, err
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (string, string, error) {
	newRefreshToken, userID, err := rdb.RotateRefreshToken(s.redisClient, ctx, refreshToken, s.jwtManager.RefreshTTL)
	if errors.Is(err, rdb.ErrRefreshTokenNotFound) {
		return "", "", ErrInvalidRefreshToken
	}
	if err != nil {
		return "", "", err
	}
	userAuth, err := s.userRepository.GetUserAuthByID(ctx, userID)
	if errors.Is(err, user.ErrNotFound) {
		return "", "", ErrInvalidRefreshToken
	}
	if err != nil {
		return "", "", err
	}
	if !userAuth.IsActive {
		return "", "", ErrUserDisabled
	}
	accessToken, err := s.jwtManager.GenerateToken(userAuth.ID, userAuth.Username, userAuth.RoleCode)
	if err != nil {
		return "", "", err
	}
	return accessToken, newRefreshToken, nil
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	return rdb.DeleteRefreshToken(s.redisClient, ctx, refreshToken)
}

func (s *AuthService) RefreshTTL() time.Duration {
	return s.jwtManager.RefreshTTL
}

func (s *AuthService) issueTokensForUser(ctx context.Context, userAuth *user.UserAuth) (string, string, error) {
	accessToken, err := s.jwtManager.GenerateToken(userAuth.ID, userAuth.Username, userAuth.RoleCode)
	if err != nil {
		return "", "", err
	}
	refreshToken, err := rdb.CreateRefreshToken(s.redisClient, ctx, userAuth.ID, s.jwtManager.RefreshTTL)
	if err != nil {
		return "", "", err
	}
	return accessToken, refreshToken, nil
}
