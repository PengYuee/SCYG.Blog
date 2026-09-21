package auth

import (
	"context"
	"errors"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/user"
)

// UserReader is the persistence boundary required by login.
type UserReader interface {
	FindActiveByUsername(context.Context, string) (*user.User, error)
}

// TokenIssuer is the credential boundary required by login.
type TokenIssuer interface {
	Issue(Principal) (string, time.Time, error)
}

// LoginResult is the protocol-neutral result of a successful login.
type LoginResult struct {
	AccessToken string
	TokenType   string
	ExpiresAt   time.Time
	Principal   Principal
}

// LoginService authenticates users and issues access tokens.
type LoginService struct {
	users  UserReader
	tokens TokenIssuer
}

// NewLoginService constructs the login use case.
func NewLoginService(users UserReader, tokens TokenIssuer) (*LoginService, error) {
	if users == nil {
		return nil, ErrInvalidCredentials
	}
	if tokens == nil {
		return nil, ErrInvalidToken
	}
	return &LoginService{users: users, tokens: tokens}, nil
}

// Login verifies credentials without revealing whether a username exists.
func (service *LoginService) Login(ctx context.Context, username, password string) (LoginResult, error) {
	if service == nil || username == "" || password == "" {
		return LoginResult{}, ErrInvalidCredentials
	}
	account, err := service.users.FindActiveByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) || errors.Is(err, user.ErrInvalidCredentials) {
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, err
	}
	if account == nil || account.VerifyPassword(password) != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	principal := Principal{UserID: account.ID, Username: account.Username}
	token, expiresAt, err := service.tokens.Issue(principal)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{AccessToken: token, TokenType: "Bearer", ExpiresAt: expiresAt, Principal: principal}, nil
}
