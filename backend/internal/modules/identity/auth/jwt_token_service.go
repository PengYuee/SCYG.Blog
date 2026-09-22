package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/user"
)

// TokenService signs and verifies short-lived HMAC JWT access tokens.
type TokenService struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

// NewTokenService constructs a token service using HS256.
func NewTokenService(secret []byte, issuer string, ttl time.Duration, now func() time.Time) (*TokenService, error) {
	if len(secret) < 32 || strings.TrimSpace(issuer) == "" || ttl <= 0 || now == nil {
		return nil, errors.New("invalid token service configuration")
	}
	return &TokenService{secret: append([]byte(nil), secret...), issuer: issuer, ttl: ttl, now: now}, nil
}

type claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Issue creates an access token for the supplied principal.
func (service *TokenService) Issue(principal Principal) (string, time.Time, error) {
	if service == nil {
		return "", time.Time{}, ErrInvalidToken
	}
	now := service.now().UTC()
	expiresAt := now.Add(service.ttl)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		Username: principal.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    service.issuer,
			Subject:   principal.UserID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	})
	signed, err := token.SignedString(service.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

// Verify validates signature, algorithm, issuer, expiry and subject.
func (service *TokenService) Verify(raw string) (Principal, error) {
	if service == nil || strings.TrimSpace(raw) == "" {
		return Principal{}, ErrInvalidToken
	}
	parsed, err := jwt.ParseWithClaims(raw, &claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return service.secret, nil
	}, jwt.WithIssuer(service.issuer), jwt.WithExpirationRequired(), jwt.WithTimeFunc(service.now))
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	value, ok := parsed.Claims.(*claims)
	if !ok || !parsed.Valid {
		return Principal{}, ErrInvalidToken
	}
	id, err := user.ParseID(value.Subject)
	if err != nil || strings.TrimSpace(value.Username) == "" {
		return Principal{}, ErrInvalidToken
	}
	return Principal{UserID: id, Username: value.Username}, nil
}
