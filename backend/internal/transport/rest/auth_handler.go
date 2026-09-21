package rest

import (
	"context"
	"errors"
	"strings"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
)

// LoginHandler exposes the generated transport boundary for username/password login.
type LoginHandler struct {
	service *identityauth.LoginService
}

// NewLoginHandler constructs the login HTTP adapter.
func NewLoginHandler(service *identityauth.LoginService) (*LoginHandler, error) {
	if service == nil {
		return nil, errors.New("登录服务为空")
	}
	return &LoginHandler{service: service}, nil
}

// Login handles the generated login operation.
func (handler *LoginHandler) Login(ctx context.Context, request generated.LoginRequestObject) (generated.LoginResponseObject, error) {
	if request.Body == nil {
		return nil, authenticationFailure{}
	}
	result, err := handler.service.Login(ctx, strings.TrimSpace(request.Body.Username), request.Body.Password)
	if err != nil {
		if errors.Is(err, identityauth.ErrInvalidCredentials) {
			return nil, authenticationFailure{}
		}
		return nil, err
	}
	return generated.Login200JSONResponse{
		AccessToken: result.AccessToken,
		TokenType:   generated.Bearer,
		ExpiresAt:   result.ExpiresAt,
	}, nil
}

type authenticationFailure struct{}

func (authenticationFailure) Error() string      { return "invalid credentials" }
func (authenticationFailure) StableCode() string { return "unauthenticated" }
