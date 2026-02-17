package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/messenger/server/internal/auth"
	"github.com/messenger/server/internal/db"
	"github.com/messenger/server/internal/federation"
)

type AuthService interface {
	Register(ctx context.Context, in auth.RegisterInput) (userID string, deviceID uuid.UUID, token string, err error)
	Login(ctx context.Context, in auth.LoginInput) (*auth.LoginResult, error)
	ValidateToken(ctx context.Context, token string) (userID int64, deviceID uuid.UUID, err error)
	ValidateWSToken(token string) (userID int64, deviceID uuid.UUID, err error)
	GetUsername(ctx context.Context, userID int64) (string, error)
	RevokeToken(ctx context.Context, token string) error
	IssueWSToken(userID int64, deviceID uuid.UUID) (string, error)
	ListDevices(ctx context.Context, userID int64, currentDeviceID uuid.UUID) ([]auth.DeviceInfo, error)
	RevokeDevice(ctx context.Context, userID int64, deviceID uuid.UUID) error
	RenameDevice(ctx context.Context, userID int64, deviceID uuid.UUID, name string) error
	CreateProxySession(ctx context.Context, homeDomain, homeToken, userID, deviceID string, expiresIn time.Duration) (localToken string, err error)
	GetProxySessionByToken(ctx context.Context, token string) (*auth.ProxySession, error)
}

type authHandler struct {
	auth          AuthService
	domain        string
	fedClient     *federation.Client
	db            *db.DB
	fedPeers     []string // FEDERATION_PEERS: manual bootstrap
	discoveryHub string   // FEDERATION_DISCOVERY_HUB: auto-fetch servers (default astralrelay.online for selfhosts)
}
