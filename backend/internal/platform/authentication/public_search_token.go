package authentication

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const publicSearchPurpose = "public_search"

// PublicSearchTokens is an independent credential domain, never an author Principal.
type PublicSearchTokens struct {
	key    []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

type publicClaims struct {
	Purpose string `json:"purpose"`
	jwt.RegisteredClaims
}

type anonymousContextKey struct{}

// WithAnonymousSubject attaches only an authenticated anonymous subject.
func WithAnonymousSubject(ctx context.Context, subject string) context.Context {
	return context.WithValue(ctx, anonymousContextKey{}, subject)
}

// AnonymousSubject returns the anonymous credential identity, independent of author authentication.
func AnonymousSubject(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(anonymousContextKey{}).(string)
	return value, ok && value != ""
}

// NewPublicSearchTokens derives a signing key for this purpose and verification context only.
func NewPublicSearchTokens(secret []byte, issuer string, ttl time.Duration, now func() time.Time) (*PublicSearchTokens, error) {
	if len(secret) < 32 || strings.TrimSpace(issuer) == "" || ttl <= 0 || now == nil {
		return nil, errors.New("invalid public search token configuration")
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte("scyg.public-search.signing.v1"))
	return &PublicSearchTokens{key: mac.Sum(nil), issuer: issuer + "/public-search", ttl: ttl, now: now}, nil
}

// Issue creates a fresh anonymous identity without registering a Blog account.
func (s *PublicSearchTokens) Issue() (string, time.Time, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", time.Time{}, err
	}
	now := s.now().UTC()
	expires := now.Add(s.ttl)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, publicClaims{Purpose: publicSearchPurpose, RegisteredClaims: jwt.RegisteredClaims{Issuer: s.issuer, Subject: "anonymous:" + id.String(), Audience: jwt.ClaimStrings{publicSearchPurpose}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires)}})
	raw, err := token.SignedString(s.key)
	return raw, expires, err
}

// Verify checks the independent signature, audience, purpose, expiry and anonymous subject.
func (s *PublicSearchTokens) Verify(raw string) (string, error) {
	invalid := errors.New("invalid public search credential")
	parsed, err := jwt.ParseWithClaims(raw, &publicClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, invalid
		}
		return s.key, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(s.issuer), jwt.WithAudience(publicSearchPurpose), jwt.WithExpirationRequired(), jwt.WithTimeFunc(s.now))
	if err != nil || !parsed.Valid {
		return "", invalid
	}
	value, ok := parsed.Claims.(*publicClaims)
	if !ok || value.Purpose != publicSearchPurpose || !strings.HasPrefix(value.Subject, "anonymous:") {
		return "", invalid
	}
	id, err := uuid.Parse(strings.TrimPrefix(value.Subject, "anonymous:"))
	if err != nil || id.Version() != 4 || id.Variant() != uuid.RFC4122 || value.Subject != "anonymous:"+id.String() {
		return "", invalid
	}
	return value.Subject, nil
}
