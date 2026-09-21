package content

import (
	"context"
	"errors"
	"reflect"
)

// Action identifies a protocol-independent authorization operation.
type Action string

// Resource identifies the resource being authorized.
type Resource struct {
	Kind string
	ID   int64
}

// Authorizer decides whether a subject may perform an action.
type Authorizer interface {
	Authorize(context.Context, Action, Resource) error
}

// ErrPermissionDenied is the stable authorization-denied sentinel.
var ErrPermissionDenied = errors.New("内容：权限不足")

// AuthorizerOrDeny preserves a valid authorizer and safely defaults to deny-all.
func AuthorizerOrDeny(candidate Authorizer) Authorizer {
	value := reflect.ValueOf(candidate)
	if !value.IsValid() {
		return DenyAll{}
	}
	switch value.Kind() { //nolint:exhaustive // only nullable kinds can be typed-nil.
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return DenyAll{}
		}
	}
	return candidate
}

// DenyAll rejects every authorization request.
type DenyAll struct{}

// Authorize implements deny-all authorization.
func (DenyAll) Authorize(context.Context, Action, Resource) error {
	return ErrPermissionDenied
}

// InvalidAuthorIDError indicates an invalid current-author identifier.
type InvalidAuthorIDError struct{}

func (InvalidAuthorIDError) Error() string { return "作者标识必须为 32 位小写十六进制" }

// CurrentAuthorUnavailableError indicates that no current author is available.
type CurrentAuthorUnavailableError struct{}

func (CurrentAuthorUnavailableError) Error() string { return "当前作者身份不可用" }

// AuthorID identifies the current author.
type AuthorID struct{ value string }

// NewAuthorID parses a strict lowercase hexadecimal author identifier.
func NewAuthorID(raw string) (AuthorID, error) {
	if len(raw) != 32 {
		return AuthorID{}, InvalidAuthorIDError{}
	}
	for index := range len(raw) {
		character := raw[index]
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return AuthorID{}, InvalidAuthorIDError{}
		}
	}
	return AuthorID{value: raw}, nil
}

// String returns the author identifier text.
func (authorID AuthorID) String() string { return authorID.value }

// CurrentAuthorProvider supplies the current author identity.
type CurrentAuthorProvider interface {
	CurrentAuthor(context.Context) (AuthorID, error)
}

// FixedCurrentAuthorProvider always returns one configured author.
type FixedCurrentAuthorProvider struct{ authorID AuthorID }

// NewFixedCurrentAuthorProvider constructs a fixed current-author provider.
func NewFixedCurrentAuthorProvider(authorID AuthorID) FixedCurrentAuthorProvider {
	return FixedCurrentAuthorProvider{authorID: authorID}
}

// CurrentAuthor returns the configured author.
func (provider FixedCurrentAuthorProvider) CurrentAuthor(context.Context) (AuthorID, error) {
	return provider.authorID, nil
}

// CurrentAuthorProviderOrUnavailable preserves a provider and safely defaults to unavailable.
func CurrentAuthorProviderOrUnavailable(candidate CurrentAuthorProvider) CurrentAuthorProvider {
	if nilLike(candidate) {
		return unavailableCurrentAuthorProvider{}
	}
	return candidate
}

type unavailableCurrentAuthorProvider struct{}

func (unavailableCurrentAuthorProvider) CurrentAuthor(context.Context) (AuthorID, error) {
	return AuthorID{}, CurrentAuthorUnavailableError{}
}

// DevelopmentAuthorizer allows actions when a development author is configured.
type DevelopmentAuthorizer struct{ authorID AuthorID }

// NewDevelopmentAuthorizer constructs a development authorizer.
func NewDevelopmentAuthorizer(authorID AuthorID) DevelopmentAuthorizer {
	return DevelopmentAuthorizer{authorID: authorID}
}

// Authorize permits actions when a development author is configured.
func (authorizer DevelopmentAuthorizer) Authorize(ctx context.Context, action Action, resource Resource) error {
	if authorizer.authorID.String() == "" {
		return DenyAll{}.Authorize(ctx, action, resource)
	}
	return nil
}

func nilLike(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() { //nolint:exhaustive // only nullable kinds can be typed-nil.
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
