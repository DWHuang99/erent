package auth

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"erent/internal/dto/request"
	casbinrbac "erent/internal/middleware/casbin"
	jwtservice "erent/internal/middleware/jwt"
	rdb "erent/internal/middleware/redis"
	"erent/internal/modules/user"
	"erent/internal/security"

	"github.com/casbin/casbin/v3"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	ErrInvalidRequest      = errors.New("invalid request")
	ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")
	ErrUserExists          = errors.New("username already exists")
	ErrUserDisabled        = errors.New("user is disabled")
)

const defaultRegistrationRoleCode = casbinrbac.RoleUser

type AuthService struct {
	userRepository *user.Repository
	jwtManager     *jwtservice.JWTManager
	redisClient    *redis.Client
	enforcer       *casbin.SyncedEnforcer
	dummyHash      string
}

func NewService(
	userRepository *user.Repository,
	jwtManager *jwtservice.JWTManager,
	redisClient *redis.Client,
	enforcer *casbin.SyncedEnforcer,
) *AuthService {
	dummyHash, _ := security.HashPassword("not-a-real-password")
	return &AuthService{
		userRepository: userRepository,
		jwtManager:     jwtManager,
		redisClient:    redisClient,
		enforcer:       enforcer,
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

func (s *AuthService) Register(ctx context.Context, registerRequest request.RegisterRequest) (*user.User, error) {
	username := strings.TrimSpace(registerRequest.Username)
	passwordLength := len([]byte(registerRequest.Password))
	if utf8.RuneCountInString(username) < 3 || utf8.RuneCountInString(username) > 50 ||
		passwordLength < 8 || passwordLength > 72 ||
		registerRequest.Password != registerRequest.CheckPassword ||
		strings.TrimSpace(registerRequest.Code) == "" ||
		!registerRequest.IAgree {
		return nil, ErrInvalidRequest
	}

	passwordHash, err := security.HashPassword(registerRequest.Password)
	if err != nil {
		return nil, err
	}
	registeredUser, err := s.userRepository.AddUser(ctx, username, passwordHash, defaultRegistrationRoleCode)
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return nil, ErrUserExists
	}
	if err != nil {
		return nil, err
	}
	if _, _, err := casbinrbac.AuthorizationForUser(s.enforcer, registeredUser.ID, registeredUser.RoleCode); err != nil {
		return nil, err
	}
	return registeredUser, nil
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
	accessToken, err := s.accessTokenForUser(userAuth)
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
	accessToken, err := s.accessTokenForUser(userAuth)
	if err != nil {
		return "", "", err
	}
	refreshToken, err := rdb.CreateRefreshToken(s.redisClient, ctx, userAuth.ID, s.jwtManager.RefreshTTL)
	if err != nil {
		return "", "", err
	}
	return accessToken, refreshToken, nil
}

func (s *AuthService) accessTokenForUser(userAuth *user.UserAuth) (string, error) {
	roles, permissions, err := casbinrbac.AuthorizationForUser(s.enforcer, userAuth.ID, userAuth.RoleCode)
	if err != nil {
		return "", err
	}
	return s.jwtManager.GenerateTokenWithPermissions(userAuth.ID, userAuth.Username, roles, permissions)
}
