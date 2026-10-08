// Package application coordinates transactions that span content features.
package application

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/image"
)

// ActionCommitArticleImages authorizes replacing article image references.
const ActionCommitArticleImages content.Action = "content.article_images.commit"

// Dependencies supplies the collaborators required by the workflow.
type Dependencies struct {
	DB            *gorm.DB
	Clock         content.Clock
	Authorizer    content.Authorizer
	CurrentAuthor content.CurrentAuthorProvider
	Articles      *article.Service
	Images        *image.Service
}

// ArticleImages coordinates article writes and image-reference updates in one transaction.
type ArticleImages struct {
	db            *gorm.DB
	clock         content.Clock
	authorizer    content.Authorizer
	currentAuthor content.CurrentAuthorProvider
	articles      *article.Service
	images        *image.Service
}

// NewArticleImages validates and constructs the cross-feature workflow.
func NewArticleImages(dependencies Dependencies) (*ArticleImages, error) {
	if dependencies.DB == nil {
		return nil, errors.New("article images database is nil")
	}
	if dependencies.Clock == nil {
		return nil, errors.New("article images clock is nil")
	}
	if dependencies.Articles == nil {
		return nil, errors.New("article service is nil")
	}
	if dependencies.Images == nil {
		return nil, errors.New("image service is nil")
	}
	return &ArticleImages{db: dependencies.DB, clock: dependencies.Clock, authorizer: content.AuthorizerOrDeny(dependencies.Authorizer), currentAuthor: content.CurrentAuthorProviderOrUnavailable(dependencies.CurrentAuthor), articles: dependencies.Articles, images: dependencies.Images}, nil
}

// PreparedCreate holds validated input and Blob-checked references before a write transaction.
type PreparedCreate struct {
	input         article.Create
	references    image.PreparedReferences
	hasReferences bool
}

// PreparedPatch holds one version-checked preview and its Blob-checked references.
type PreparedPatch struct {
	preview       article.PatchPreview
	references    image.PreparedReferences
	hasReferences bool
}

// PrepareCreate validates and authorizes creation without opening a write transaction.
func (workflow *ArticleImages) PrepareCreate(ctx context.Context, input article.Create) (PreparedCreate, error) {
	status, err := workflow.articles.ValidateCreate(input)
	if err != nil {
		return PreparedCreate{}, err
	}
	if err := workflow.authorizer.Authorize(ctx, article.ActionCreateArticle, content.Resource{Kind: "article"}); err != nil {
		return PreparedCreate{}, permission()
	}
	if status == article.StatusPublished {
		if err := workflow.authorizer.Authorize(ctx, article.ActionPublishArticle, content.Resource{Kind: "article"}); err != nil {
			return PreparedCreate{}, permission()
		}
	}
	keys, err := article.ManagedImageReferences(input.Content)
	if err != nil {
		return PreparedCreate{}, validation(err)
	}
	references, err := image.ParseReferenceKeys(keys)
	if err != nil {
		return PreparedCreate{}, stable(err)
	}
	prepared, err := workflow.prepare(ctx, 0, references)
	if err != nil {
		return PreparedCreate{}, err
	}
	return PreparedCreate{input: input, references: prepared, hasReferences: !references.Empty()}, nil
}

// Create prepares images outside the atomic article/reference write.
func (workflow *ArticleImages) Create(ctx context.Context, input article.Create) (article.Result, error) {
	prepared, err := workflow.PrepareCreate(ctx, input)
	if err != nil {
		return article.Result{}, stable(err)
	}
	var out article.Result
	err = workflow.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		out, err = workflow.CreateInTx(ctx, tx, prepared)
		return err
	})
	if err != nil {
		return article.Result{}, stable(err)
	}
	return out, stable(err)
}

// CreateInTx commits previously prepared input without Blob I/O.
func (workflow *ArticleImages) CreateInTx(ctx context.Context, tx *gorm.DB, prepared PreparedCreate) (article.Result, error) {
	if err := workflow.authorizer.Authorize(ctx, article.ActionCreateArticle, content.Resource{Kind: "article"}); err != nil {
		return article.Result{}, permission()
	}
	if prepared.input.Status == article.ArticleCreationStatusPublished {
		if err := workflow.authorizer.Authorize(ctx, article.ActionPublishArticle, content.Resource{Kind: "article"}); err != nil {
			return article.Result{}, permission()
		}
	}
	if prepared.hasReferences {
		if err := workflow.authorizer.Authorize(ctx, ActionCommitArticleImages, content.Resource{Kind: "article"}); err != nil {
			return article.Result{}, permission()
		}
	}
	out, err := workflow.articles.CreateInTx(ctx, tx, prepared.input)
	if err != nil {
		return article.Result{}, stable(err)
	}
	if prepared.hasReferences {
		err = workflow.images.ReplaceReferencesInTx(ctx, tx, out.ID, prepared.references, workflow.clock.Now().UTC())
	}
	return out, stable(err)
}

// PreparePatch previews the patch and loads images before opening the write transaction.
func (workflow *ArticleImages) PreparePatch(ctx context.Context, input article.Patch) (PreparedPatch, error) {
	if err := workflow.articles.ValidatePatch(input); err != nil {
		return PreparedPatch{}, err
	}
	if err := workflow.authorizer.Authorize(ctx, article.ActionReviseArticle, content.Resource{Kind: "article", ID: input.ID}); err != nil {
		return PreparedPatch{}, permission()
	}
	preview, err := workflow.articles.PreviewPatch(ctx, input)
	if err != nil {
		return PreparedPatch{}, stable(err)
	}
	references, err := image.ParseReferenceKeys(preview.Keys)
	if err != nil {
		return PreparedPatch{}, stable(err)
	}
	prepared, err := workflow.prepare(ctx, input.ID, references)
	if err != nil {
		return PreparedPatch{}, err
	}
	return PreparedPatch{preview: preview, references: prepared, hasReferences: !references.Empty()}, nil
}

// Patch prepares once and atomically persists the preview and references.
func (workflow *ArticleImages) Patch(ctx context.Context, input article.Patch) (article.Result, error) {
	prepared, err := workflow.PreparePatch(ctx, input)
	if err != nil {
		return article.Result{}, stable(err)
	}
	var out article.Result
	err = workflow.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		out, err = workflow.PatchInTx(ctx, tx, prepared)
		return err
	})
	if err != nil {
		return article.Result{}, stable(err)
	}
	return out, stable(err)
}

// PatchInTx conditionally saves the prepared version and commits references without Blob I/O.
func (workflow *ArticleImages) PatchInTx(ctx context.Context, tx *gorm.DB, prepared PreparedPatch) (article.Result, error) {
	if prepared.preview.Article == nil {
		return article.Result{}, stable(errors.New("article patch preview is nil"))
	}
	if err := workflow.authorizer.Authorize(ctx, article.ActionReviseArticle, content.Resource{Kind: "article", ID: prepared.preview.Article.ID().Int64()}); err != nil {
		return article.Result{}, permission()
	}
	if prepared.hasReferences {
		if err := workflow.authorizer.Authorize(ctx, ActionCommitArticleImages, content.Resource{Kind: "article", ID: prepared.preview.Article.ID().Int64()}); err != nil {
			return article.Result{}, permission()
		}
	}
	out, err := workflow.articles.SavePatchPreviewInTx(ctx, tx, prepared.preview)
	if err != nil {
		return article.Result{}, stable(err)
	}
	err = workflow.images.ReplaceReferencesInTx(ctx, tx, out.ID, prepared.references, workflow.clock.Now().UTC())
	return out, stable(err)
}

// Publish delegates the article lifecycle transition.
func (workflow *ArticleImages) Publish(ctx context.Context, input article.Publish) (article.Result, error) {
	return workflow.articles.Publish(ctx, input)
}

// Archive delegates the article lifecycle transition.
func (workflow *ArticleImages) Archive(ctx context.Context, input article.Archive) (article.Result, error) {
	return workflow.articles.Archive(ctx, input)
}

// PublishInTx publishes using the caller-owned transaction.
func (workflow *ArticleImages) PublishInTx(ctx context.Context, tx *gorm.DB, input article.Publish) (article.Result, error) {
	return workflow.articles.PublishInTx(ctx, tx, input)
}

// ArchiveInTx archives using the caller-owned transaction.
func (workflow *ArticleImages) ArchiveInTx(ctx context.Context, tx *gorm.DB, input article.Archive) (article.Result, error) {
	return workflow.articles.ArchiveInTx(ctx, tx, input)
}

func (workflow *ArticleImages) prepare(ctx context.Context, articleID int64, keys image.ReferenceKeys) (image.PreparedReferences, error) {
	if keys.Empty() {
		return image.PreparedReferences{}, nil
	}
	if err := workflow.authorizer.Authorize(ctx, ActionCommitArticleImages, content.Resource{Kind: "article", ID: articleID}); err != nil {
		return image.PreparedReferences{}, permission()
	}
	author, err := workflow.currentAuthor.CurrentAuthor(ctx)
	if err != nil {
		return image.PreparedReferences{}, permission()
	}
	return workflow.images.PrepareReferences(ctx, author, keys)
}

// CreateContext preserves the context-aware article creation entry point.
func (workflow *ArticleImages) CreateContext(ctx context.Context, input article.Create) (article.Result, error) {
	return workflow.Create(ctx, input)
}

// Error is the stable application-layer error envelope.
type Error struct {
	Code  string
	Cause error
}

func (e *Error) Error() string {
	if e == nil {
		return "application"
	}
	if e.Cause == nil {
		return e.Code
	}
	return e.Code + ": " + e.Cause.Error()
}
func (e *Error) Unwrap() error { return e.Cause }

// StableCode returns the consumer-facing application error code.
func (e *Error) StableCode() string {
	if e == nil {
		return "internal"
	}
	return e.Code
}
func validation(err error) error { return &Error{Code: "validation", Cause: err} }
func permission() error          { return &Error{Code: "permission_denied", Cause: content.ErrPermissionDenied} }

func stable(err error) error {
	if err == nil {
		return nil
	}
	var known *Error
	if errors.As(err, &known) {
		return known
	}
	var imageErr *image.Error
	if errors.As(err, &imageErr) {
		return &Error{Code: imageErr.StableCode(), Cause: imageErr}
	}
	var articleErr *article.Error
	if errors.As(err, &articleErr) {
		return &Error{Code: articleErr.StableCode(), Cause: articleErr}
	}
	var versionConflict *article.VersionConflict
	if errors.As(err, &versionConflict) {
		return &Error{Code: "stale_version", Cause: err}
	}
	switch {
	case errors.Is(err, article.ErrInvalidValue), errors.Is(err, article.ErrDuplicateTag), errors.Is(err, article.ErrArticleTypeRequired), errors.Is(err, article.ErrContentRequired):
		return &Error{Code: "validation", Cause: err}
	case errors.Is(err, article.ErrInvalidTransition), errors.Is(err, article.ErrNoChange), errors.Is(err, article.ErrDeleted), errors.Is(err, article.ErrStaleVersion):
		return &Error{Code: "failed_precondition", Cause: err}
	}
	return &Error{Code: "internal", Cause: err}
}
