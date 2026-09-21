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

// Create creates an article and commits its managed image references.
func (workflow *ArticleImages) Create(ctx context.Context, input article.Create) (article.Result, error) {
	status, err := workflow.articles.ValidateCreate(input)
	if err != nil {
		return article.Result{}, err
	}
	keys, err := article.ManagedImageReferences(input.Content)
	if err != nil {
		return article.Result{}, validation(err)
	}
	referenceKeys, err := image.ParseReferenceKeys(keys)
	if err != nil {
		return article.Result{}, stable(err)
	}
	if err := workflow.authorizer.Authorize(ctx, article.ActionCreateArticle, content.Resource{Kind: "article"}); err != nil {
		return article.Result{}, permission()
	}
	if status == article.StatusPublished {
		if err := workflow.authorizer.Authorize(ctx, article.ActionPublishArticle, content.Resource{Kind: "article"}); err != nil {
			return article.Result{}, permission()
		}
	}
	prepared, err := workflow.prepare(ctx, 0, referenceKeys)
	if err != nil {
		return article.Result{}, err
	}
	var result article.Result
	var transactionResult article.Result
	err = workflow.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		transactionResult, err = workflow.articles.CreateInTx(ctx, tx, input)
		if err != nil {
			return err
		}
		if !referenceKeys.Empty() {
			return workflow.images.ReplaceReferencesInTx(ctx, tx, transactionResult.ID, prepared, workflow.clock.Now().UTC())
		}
		return nil
	})
	if err == nil {
		result = transactionResult
	}
	return result, stable(err)
}

// Patch applies an article content patch and replaces its managed image references.
func (workflow *ArticleImages) Patch(ctx context.Context, input article.Patch) (article.Result, error) {
	if err := workflow.articles.ValidatePatch(input); err != nil {
		return article.Result{}, err
	}
	if err := workflow.authorizer.Authorize(ctx, article.ActionReviseArticle, content.Resource{Kind: "article", ID: input.ID}); err != nil {
		return article.Result{}, permission()
	}
	keys, previewErr := workflow.preview(ctx, input)
	if previewErr != nil {
		return article.Result{}, previewErr
	}
	referenceKeys, err := image.ParseReferenceKeys(keys)
	if err != nil {
		return article.Result{}, stable(err)
	}
	prepared, err := workflow.prepare(ctx, input.ID, referenceKeys)
	if err != nil {
		return article.Result{}, err
	}
	var result article.Result
	err = workflow.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var transactionResult article.Result
		transactionResult, err = workflow.articles.PatchInTx(ctx, tx, input)
		if err != nil {
			return err
		}
		result = transactionResult
		return workflow.images.ReplaceReferencesInTx(ctx, tx, transactionResult.ID, prepared, workflow.clock.Now().UTC())
	})
	return result, stable(err)
}

// Publish delegates the article lifecycle transition to the article feature.
func (workflow *ArticleImages) Publish(ctx context.Context, input article.Publish) (article.Result, error) {
	return workflow.articles.Publish(ctx, input)
}

// Archive delegates the article lifecycle transition to the article feature.
func (workflow *ArticleImages) Archive(ctx context.Context, input article.Archive) (article.Result, error) {
	return workflow.articles.Archive(ctx, input)
}

func (workflow *ArticleImages) preview(ctx context.Context, input article.Patch) ([]string, error) {
	var preview article.PatchPreview
	err := workflow.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		preview, err = workflow.articles.PreviewPatchInTx(ctx, tx, input)
		if err != nil {
			return err
		}
		_, err = image.ParseReferenceKeys(preview.Keys)
		return err
	})
	if err != nil {
		return nil, stable(err)
	}
	return preview.Keys, nil
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
