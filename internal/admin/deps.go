package admin

import (
	"context"
	"net/http"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/deepseek"
)

type ConfigStore interface {
	Snapshot() config.Config
	Keys() []string
	Accounts() []config.Account
	FindAccount(identifier string) (config.Account, bool)
	UpdateAccountToken(identifier, token string) error
	Update(mutator func(*config.Config) error) error
	ExportJSONAndBase64() (string, string, error)
	IsEnvBacked() bool
	SetVercelSync(hash string, ts int64) error
	AdminPasswordHash() string
	AdminJWTExpireHours() int
	AdminJWTValidAfterUnix() int64
	RuntimeAccountMaxInflight() int
	RuntimeAccountMaxQueue(defaultSize int) int
	RuntimeGlobalMaxInflight(defaultSize int) int
}

type PoolController interface {
	Reset()
	Status() map[string]any
	ApplyRuntimeLimits(maxInflightPerAccount, maxQueueSize, globalMaxInflight int)
}

type StreamLeaseStatsProvider interface {
	StreamLeaseStats() map[string]any
}

type DeepSeekCaller interface {
	Login(ctx context.Context, acc config.Account) (string, error)
	CreateSession(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error)
	GetPow(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error)
	CallCompletion(ctx context.Context, a *auth.RequestAuth, payload map[string]any, powResp string, maxAttempts int) (*http.Response, error)
}

var _ ConfigStore = (*config.Store)(nil)
var _ PoolController = (*account.Pool)(nil)
var _ DeepSeekCaller = (*deepseek.Client)(nil)
