package api

import (
	"context"

	"github.com/google/uuid"
	"github.com/messenger/server/internal/auth"
)

type AuthService interface {
	Register(ctx context.Context, in auth.RegisterInput) (userID string, deviceID uuid.UUID, token string, err error)
	Login(ctx context.Context, in auth.LoginInput) (*auth.LoginResult, error)
	ValidateToken(ctx context.Context, token string) (userID int64, deviceID uuid.UUID, err error)
	ValidateWSToken(token string) (userID int64, deviceID uuid.UUID, err error)
	GetUsername(ctx context.Context, userID int64) (string, error)
	RevokeToken(ctx context.Context, token string) error
	IssueWSToken(userID int64, deviceID uuid.UUID) (string, error)
}

type authHandler struct {
	auth AuthService
}
