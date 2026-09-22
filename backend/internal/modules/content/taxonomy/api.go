// Package taxonomy owns article type and tag behavior.
package taxonomy

import (
	"context"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

const (
	// ActionManageArticleType authorizes article type mutations.
	ActionManageArticleType content.Action = "content.article_type.manage"
	// ActionManageTag authorizes tag mutations.
	ActionManageTag content.Action = "content.tag.manage"
)

// CreateArticleType contains fields for creating an article type.
type CreateArticleType struct {
	Name  string
	Image *string
	Meun  int32
}

// OptionalImage distinguishes an omitted image from an explicit null value.
type OptionalImage struct {
	Provided bool
	Value    *string
}

// PatchArticleType contains partial article type updates.
type PatchArticleType struct {
	ID      int64
	Version uint64
	Name    *string
	Image   OptionalImage
	Meun    *int32
}

// RenameArticleType contains an article type rename command.
type RenameArticleType struct {
	ID      int64
	Version uint64
	Name    string
}

// DeleteArticleType contains an article type deletion command.
type DeleteArticleType struct {
	ID      int64
	Version uint64
}

// CreateTag contains fields for creating a tag.
type CreateTag struct{ Name string }

// RenameTag contains a tag rename command.
type RenameTag struct {
	ID      int64
	Version uint64
	Name    string
}

// DeleteTag contains a tag deletion command.
type DeleteTag struct {
	ID      int64
	Version uint64
}

// ListArticleTypes contains article type filtering and pagination.
type ListArticleTypes struct {
	Page     int
	PageSize int
	Name     string
	// Sort uses the OpenAPI values: createdAt, -createdAt, updatedAt, -updatedAt, title, and -title.
	Sort string
}

// GetArticleType identifies one article type.
type GetArticleType struct{ ID int64 }

// ListTags contains tag filtering and pagination.
type ListTags struct {
	Page     int
	PageSize int
	Name     string
	// Sort uses the OpenAPI values: createdAt, -createdAt, updatedAt, -updatedAt, title, and -title.
	Sort string
}

// GetTag identifies one tag.
type GetTag struct{ ID int64 }

// ListPublicArticleTypes contains public article type filtering and pagination.
type ListPublicArticleTypes struct {
	Page     int
	PageSize int
	Name     string
}

// GetPublicArticleType identifies one publicly visible article type.
type GetPublicArticleType struct{ ID int64 }

// ListPublicTags contains public tag filtering and pagination.
type ListPublicTags struct {
	Page     int
	PageSize int
	Name     string
}

// GetPublicTag identifies one publicly visible tag.
type GetPublicTag struct{ ID int64 }

// ArticleTypeResult is the consumer-facing article type representation.
type ArticleTypeResult struct {
	ID         int64
	Name       string
	Image      *string
	Meun       int32
	Version    uint64
	CreatedAt  time.Time
	ModifiedAt time.Time
}

// ArticleTypePage is a paginated article type result.
type ArticleTypePage struct {
	Items      []ArticleTypeResult
	Number     int
	Size       int
	TotalItems int64
	TotalPages int
}

// TagResult is the consumer-facing tag representation.
type TagResult struct {
	ID         int64
	Name       string
	Version    uint64
	CreatedAt  time.Time
	ModifiedAt time.Time
}

// TagPage is a paginated tag result.
type TagPage struct {
	Items      []TagResult
	Number     int
	Size       int
	TotalItems int64
	TotalPages int
}

// PublicArticleTypeResult is the public article type projection.
type PublicArticleTypeResult struct {
	ID           int64
	Name         string
	Image        *string
	ArticleCount int64
	Version      uint64
}

// PublicArticleTypePage is a paginated public article type result.
type PublicArticleTypePage struct {
	Items      []PublicArticleTypeResult
	Number     int
	Size       int
	TotalItems int64
	TotalPages int
}

// PublicTagResult is the public tag projection.
type PublicTagResult struct {
	ID           int64
	Name         string
	ArticleCount int64
	Version      uint64
}

// PublicTagPage is a paginated public tag result.
type PublicTagPage struct {
	Items      []PublicTagResult
	Number     int
	Size       int
	TotalItems int64
	TotalPages int
}

// API is the narrow taxonomy capability consumed by transport.
type API interface {
	GetArticleType(context.Context, GetArticleType) (ArticleTypeResult, error)
	ListArticleTypes(context.Context, ListArticleTypes) (ArticleTypePage, error)
	GetPublicArticleType(context.Context, GetPublicArticleType) (PublicArticleTypeResult, error)
	ListPublicArticleTypes(context.Context, ListPublicArticleTypes) (PublicArticleTypePage, error)
	GetTag(context.Context, GetTag) (TagResult, error)
	ListTags(context.Context, ListTags) (TagPage, error)
	GetPublicTag(context.Context, GetPublicTag) (PublicTagResult, error)
	ListPublicTags(context.Context, ListPublicTags) (PublicTagPage, error)
	CreateArticleType(context.Context, CreateArticleType) (ArticleTypeResult, error)
	PatchArticleType(context.Context, PatchArticleType) (ArticleTypeResult, error)
	RenameArticleType(context.Context, RenameArticleType) (ArticleTypeResult, error)
	DeleteArticleType(context.Context, DeleteArticleType) error
	CreateTag(context.Context, CreateTag) (TagResult, error)
	RenameTag(context.Context, RenameTag) (TagResult, error)
	DeleteTag(context.Context, DeleteTag) error
}

// Dependencies are the non-persistence collaborators of Service.
type Dependencies struct {
	Clock      content.Clock
	Authorizer content.Authorizer
}

var _ API = (*Service)(nil)
