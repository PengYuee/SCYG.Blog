// Package article owns article commands, lifecycle rules, and persistence.
package article

import (
	"context"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

// Authorization actions understood by the article feature.
const (
	ActionCreateArticle  content.Action = "content.article.create"
	ActionReviseArticle  content.Action = "content.article.revise"
	ActionPublishArticle content.Action = "content.article.publish"
	ActionArchiveArticle content.Action = "content.article.archive"
	ActionDeleteArticle  content.Action = "content.article.delete"
)

// ArticleCreationStatus selects the initial lifecycle state for a new article.
type ArticleCreationStatus int32

const (
	// ArticleCreationStatusDraft creates an editable draft.
	ArticleCreationStatusDraft ArticleCreationStatus = 1
	// ArticleCreationStatusPublished creates an already published article.
	ArticleCreationStatusPublished ArticleCreationStatus = 2
)

// Create contains the validated article fields used by creation.
type Create struct {
	Status                       ArticleCreationStatus
	ArticleTypeID                int64
	Title, Slug, Digest, Content string
	TagIDs                       []int64
}

// Revise contains the complete replacement fields for an existing article.
type Revise struct {
	ID                           int64
	Version                      uint64
	ArticleTypeID                int64
	Title, Slug, Digest, Content string
	TagIDs                       []int64
}

// Patch contains optional fields for a partial article update. Lifecycle
// transitions are exposed as separate commands.
type Patch struct {
	ID                           int64
	Version                      uint64
	ArticleTypeID                *int64
	Title, Slug, Digest, Content *string
	TagIDs                       *[]int64
}

// Publish requests a version-checked publication transition.
type Publish struct {
	ID      int64
	Version uint64
}

// Archive requests a version-checked archival transition.
type Archive struct {
	ID      int64
	Version uint64
}

// Delete requests a version-checked soft deletion.
type Delete struct {
	ID      int64
	Version uint64
}

// Get identifies one article for a read operation.
type Get struct{ ID int64 }

// List contains article filtering, sorting, and pagination parameters.
type List struct {
	Page, PageSize       int
	ArticleTypeID, TagID int64
	Query, Sort          string
}

// Result is the protocol-neutral article read result.
type Result struct {
	ID, ArticleTypeID                    int64
	Title, Slug, Digest, Content, Status string
	TagIDs                               []int64
	Support, Comment, Visited            int64
	Version                              uint64
	CreatedAt, ModifiedAt                time.Time
}

// Page is a paginated article result.
type Page struct {
	Items        []Result
	Number, Size int
	TotalItems   int64
	TotalPages   int
}

// PatchPreview contains the article state and image keys after patch replay.
type PatchPreview struct {
	Article *Article
	Keys    []string
}

// API exposes article read and lifecycle operations to consumers.
type API interface {
	Get(context.Context, Get) (Result, error)
	List(context.Context, List) (Page, error)
	Delete(context.Context, Delete) error
	Publish(context.Context, Publish) (Result, error)
	Archive(context.Context, Archive) (Result, error)
}

// Dependencies supplies shared collaborators to the article service.
type Dependencies struct {
	Clock      content.Clock
	Authorizer content.Authorizer
}
