package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"ds2api/internal/account"
	"ds2api/internal/config"
)

type ctxKey string

const authCtxKey ctxKey = "auth_context"

var (
	ErrUnauthorized = errors.New("unauthorized: missing Bearer token")
	ErrNoAccount    = errors.New("no accounts configured or all accounts are busy")
)

type RequestAuth struct {
	UseConfigToken bool
	DeepSeekToken  string
	AccountID      string
	Account        config.Account
	TriedAccounts  map[string]bool
	resolver       *Resolver
}

type LoginFunc func(ctx context.Context, acc config.Account) (string, error)

type Resolver struct {
	Store *config.Store
	Pool  *account.Pool
	Login LoginFunc
}

func NewResolver(store *config.Store, pool *account.Pool, login LoginFunc) *Resolver {
	return &Resolver{Store: store, Pool: pool, Login: login}
}

func (r *Resolver) Determine(req *http.Request) (*RequestAuth, error) {
	authHeader := req.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, ErrUnauthorized
	}
	callerKey := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	ctx := req.Context()
	if !r.Store.HasAPIKey(callerKey) {
		return &RequestAuth{UseConfigToken: false, DeepSeekToken: callerKey, resolver: r, TriedAccounts: map[string]bool{}}, nil
	}
	target := strings.TrimSpace(req.Header.Get("X-Ds2-Target-Account"))
	acc, ok := r.Pool.Acquire(target, nil)
	if !ok {
		return nil, ErrNoAccount
	}
	a := &RequestAuth{
		UseConfigToken: true,
		AccountID:      acc.Identifier(),
		Account:        acc,
		TriedAccounts:  map[string]bool{},
		resolver:       r,
	}
	if acc.Token == "" {
		if err := r.loginAndPersist(ctx, a); err != nil {
			r.Pool.Release(a.AccountID)
			return nil, err
		}
	} else {
		a.DeepSeekToken = acc.Token
	}
	return a, nil
}

func WithAuth(ctx context.Context, a *RequestAuth) context.Context {
	return context.WithValue(ctx, authCtxKey, a)
}

func FromContext(ctx context.Context) (*RequestAuth, bool) {
	v := ctx.Value(authCtxKey)
	a, ok := v.(*RequestAuth)
	return a, ok
}

func (r *Resolver) loginAndPersist(ctx context.Context, a *RequestAuth) error {
	token, err := r.Login(ctx, a.Account)
	if err != nil {
		return err
	}
	a.Account.Token = token
	a.DeepSeekToken = token
	return r.Store.UpdateAccountToken(a.AccountID, token)
}

func (r *Resolver) RefreshToken(ctx context.Context, a *RequestAuth) bool {
	if !a.UseConfigToken || a.AccountID == "" {
		return false
	}
	_ = r.Store.UpdateAccountToken(a.AccountID, "")
	a.Account.Token = ""
	if err := r.loginAndPersist(ctx, a); err != nil {
		config.Logger.Error("[refresh_token] failed", "account", a.AccountID, "error", err)
		return false
	}
	return true
}

func (r *Resolver) MarkTokenInvalid(a *RequestAuth) {
	if !a.UseConfigToken || a.AccountID == "" {
		return
	}
	a.Account.Token = ""
	a.DeepSeekToken = ""
	_ = r.Store.UpdateAccountToken(a.AccountID, "")
}

func (r *Resolver) SwitchAccount(ctx context.Context, a *RequestAuth) bool {
	if !a.UseConfigToken {
		return false
	}
	if a.TriedAccounts == nil {
		a.TriedAccounts = map[string]bool{}
	}
	if a.AccountID != "" {
		a.TriedAccounts[a.AccountID] = true
		r.Pool.Release(a.AccountID)
	}
	acc, ok := r.Pool.Acquire("", a.TriedAccounts)
	if !ok {
		return false
	}
	a.Account = acc
	a.AccountID = acc.Identifier()
	if acc.Token == "" {
		if err := r.loginAndPersist(ctx, a); err != nil {
			return false
		}
	} else {
		a.DeepSeekToken = acc.Token
	}
	return true
}

func (r *Resolver) Release(a *RequestAuth) {
	if a == nil || !a.UseConfigToken || a.AccountID == "" {
		return
	}
	r.Pool.Release(a.AccountID)
}
