package content

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"
)

// Action identifies an authorization decision independent of transport.
type Action string

const (
	// ActionCreateArticle authorizes draft creation.
	ActionCreateArticle Action = "content.article.create"
	// ActionReviseArticle authorizes article revision.
	ActionReviseArticle Action = "content.article.revise"
	// ActionPublishArticle authorizes publication.
	ActionPublishArticle Action = "content.article.publish"
	// ActionArchiveArticle authorizes archival.
	ActionArchiveArticle Action = "content.article.archive"
	// ActionDeleteArticle authorizes article deletion.
	ActionDeleteArticle Action = "content.article.delete"
	// ActionManageArticleType authorizes article-type changes.
	ActionManageArticleType Action = "content.article_type.manage"
	// ActionManageTag authorizes tag changes.
	ActionManageTag Action = "content.tag.manage"
)

// Resource identifies the domain object under authorization.
type Resource struct {
	Kind string
	ID   int64
}

// Authorizer decides whether an action is allowed for a resource.
type Authorizer interface {
	Authorize(context.Context, Action, Resource) error
}

// AuthorizerOrDeny preserves a concrete authorizer and safely defaults nil interfaces and typed nils to DenyAll.
func AuthorizerOrDeny(candidate Authorizer) Authorizer {
	value := reflect.ValueOf(candidate)
	if !value.IsValid() {
		return DenyAll{}
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return DenyAll{}
		}
	}
	return candidate
}

// DenyAll rejects every authorization decision and is the safe default implementation.
type DenyAll struct{}

// Authorize always returns permission_denied.
func (DenyAll) Authorize(context.Context, Action, Resource) error {
	return &ApplicationError{Code: CodePermissionDenied, Kind: KindPermission, Cause: ErrPermissionDenied}
}

// ErrorCode is a stable protocol-neutral application error code.
type ErrorCode string

// ErrorKind groups application failures by caller behavior.
type ErrorKind string

const (
	CodeValidation         ErrorCode = "validation"
	CodePermissionDenied   ErrorCode = "permission_denied"
	CodeNotFound           ErrorCode = "not_found"
	CodeAlreadyExists      ErrorCode = "already_exists"
	CodeFailedPrecondition ErrorCode = "failed_precondition"
	CodeVersionRequired    ErrorCode = "version_required"
	CodeStaleVersion       ErrorCode = "stale_version"
	CodeInternal           ErrorCode = "internal"
	KindValidation         ErrorKind = "validation"
	KindPermission         ErrorKind = "permission"
	KindMissing            ErrorKind = "missing"
	KindConflict           ErrorKind = "conflict"
	KindInternal           ErrorKind = "internal"
)

// Stable application sentinels expose business semantics without adapter causes.
var (
	ErrConflict           = errors.New("content: conflict")
	ErrFailedPrecondition = errors.New("content: failed precondition")
	ErrNotFound           = errors.New("content: not found")
	ErrPersistence        = errors.New("content: persistence failure")
)

// ErrPermissionDenied is the stable authorization sentinel.
var ErrPermissionDenied = errors.New("content: permission denied")

// ApplicationError carries stable semantics without an HTTP status.
type ApplicationError struct {
	Code            ErrorCode
	Kind            ErrorKind
	Cause           error
	ExpectedVersion uint64
	ActualVersion   uint64
}

// Error renders stable failure semantics.
func (failure *ApplicationError) Error() string {
	return fmt.Sprintf("%s: %v", failure.Code, failure.Cause)
}

// Unwrap preserves errors.Is and errors.As chains.
func (failure *ApplicationError) Unwrap() error { return failure.Cause }

// CreateArticle creates an article draft.
type CreateArticle struct {
	ArticleTypeID int64
	Title         string
	Slug          string
	Digest        string
	Content       string
	TagIDs        []int64
}

// ReviseArticle revises a versioned article.
type ReviseArticle struct {
	ID            int64
	Version       uint64
	ArticleTypeID int64
	Title         string
	Slug          string
	Digest        string
	Content       string
	TagIDs        []int64
}

// PatchArticle partially updates article values and optionally changes lifecycle status.
type PatchArticle struct {
	ID            int64
	Version       uint64
	ArticleTypeID *int64
	Title         *string
	Slug          *string
	Digest        *string
	Content       *string
	TagIDs        *[]int64
	Status        *string
}

// PublishArticle publishes a versioned draft.
type PublishArticle struct {
	ID      int64
	Version uint64
}

// ArchiveArticle archives a versioned published article.
type ArchiveArticle struct {
	ID      int64
	Version uint64
}

// DeleteArticle soft-deletes a versioned article.
type DeleteArticle struct {
	ID      int64
	Version uint64
}

// CreateArticleType creates an article type.
type CreateArticleType struct{ Name string }

// RenameArticleType renames a versioned article type.
type RenameArticleType struct {
	ID      int64
	Version uint64
	Name    string
}

// DeleteArticleType soft-deletes a versioned article type.
type DeleteArticleType struct {
	ID      int64
	Version uint64
}

// CreateTag creates a tag.
type CreateTag struct{ Name string }

// RenameTag renames a versioned tag.
type RenameTag struct {
	ID      int64
	Version uint64
	Name    string
}

// DeleteTag soft-deletes a versioned tag.
type DeleteTag struct {
	ID      int64
	Version uint64
}

// GetArticle finds one public article.
type GetArticle struct{ ID int64 }

// ListArticles lists public articles with bounded filters supplied by a caller.
type ListArticles struct {
	Page          int
	PageSize      int
	ArticleTypeID int64
	TagID         int64
	Query         string
	Sort          string
}

// ListArticleTypes lists article types with protocol-neutral filtering and paging.
type ListArticleTypes struct {
	Page     int
	PageSize int
	Name     string
	Sort     string
}

// GetArticleType finds one nondeleted article type.
type GetArticleType struct{ ID int64 }

// ListTags lists tags with protocol-neutral filtering and paging.
type ListTags struct {
	Page     int
	PageSize int
	Name     string
	Sort     string
}

// GetTag finds one nondeleted tag.
type GetTag struct{ ID int64 }

// ArticleResult is a protocol-neutral article result.
type ArticleResult struct {
	ID            int64
	ArticleTypeID int64
	Title         string
	Slug          string
	Digest        string
	Content       string
	Status        string
	TagIDs        []int64
	Version       uint64
	CreatedAt     time.Time
	ModifiedAt    time.Time
}

// ArticleTypeResult is a protocol-neutral article-type result.
type ArticleTypeResult struct {
	ID         int64
	Name       string
	Version    uint64
	CreatedAt  time.Time
	ModifiedAt time.Time
}

// TagResult is a protocol-neutral tag result.
type TagResult struct {
	ID         int64
	Name       string
	Version    uint64
	CreatedAt  time.Time
	ModifiedAt time.Time
}

// ArticlePage is a protocol-neutral article page.
type ArticlePage struct {
	Items      []ArticleResult
	Number     int
	Size       int
	TotalItems int64
	TotalPages int
}

// ArticleTypePage is a protocol-neutral article-type page.
type ArticleTypePage struct {
	Items      []ArticleTypeResult
	Number     int
	Size       int
	TotalItems int64
	TotalPages int
}

// TagPage is a protocol-neutral tag page.
type TagPage struct {
	Items      []TagResult
	Number     int
	Size       int
	TotalItems int64
	TotalPages int
}
