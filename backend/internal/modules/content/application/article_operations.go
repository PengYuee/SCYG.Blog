package application

import (
	"context"
	"database/sql"
	"errors"

	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/operation"
)

// OperationDependencies wires the existing image workflow and success ledger.
type OperationDependencies struct {
	DB            *gorm.DB
	Articles      *article.Service
	ArticleImages *ArticleImages
	Operations    *operation.Service
}

// ArticleOperations provides globally idempotent Blog writes.
type ArticleOperations struct{ dependencies OperationDependencies }

// NewArticleOperations constructs the cross-feature write transaction owner.
func NewArticleOperations(dependencies OperationDependencies) (*ArticleOperations, error) {
	if dependencies.DB == nil || dependencies.Articles == nil || dependencies.ArticleImages == nil || dependencies.Operations == nil {
		return nil, errors.New("article operations dependencies are incomplete")
	}
	return &ArticleOperations{dependencies: dependencies}, nil
}

func (workflow *ArticleOperations) execute(ctx context.Context, key string, write func(*gorm.DB) (article.Result, error)) (article.Result, error) {
	var result article.Result
	err := workflow.dependencies.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		id, replay, err := workflow.dependencies.Operations.ReserveInTx(ctx, tx, key)
		if err != nil {
			return err
		}
		if replay {
			result, err = workflow.dependencies.Articles.GetManageInTx(ctx, tx, article.Get{ID: id})
			return err
		}
		written, err := write(tx)
		if err != nil {
			return err
		}
		if err := workflow.dependencies.Operations.CompleteInTx(ctx, tx, key, written.ID); err != nil {
			return err
		}
		// The response is the same management projection on fresh writes and replays.
		result, err = workflow.dependencies.Articles.GetManageInTx(ctx, tx, article.Get{ID: written.ID})
		return err
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return article.Result{}, stable(err)
	}
	return result, nil
}

// Create prepares Blob reads outside the transaction and defers preparation
// errors until key arbitration has ruled out a successful replay.
func (workflow *ArticleOperations) Create(ctx context.Context, key string, input article.Create) (article.Result, error) {
	prepared, prepareErr := workflow.dependencies.ArticleImages.PrepareCreate(ctx, input)
	return workflow.execute(ctx, key, func(tx *gorm.DB) (article.Result, error) {
		if prepareErr != nil {
			return article.Result{}, stable(prepareErr)
		}
		return workflow.dependencies.ArticleImages.CreateInTx(ctx, tx, prepared)
	})
}

// Patch prepares once outside the transaction; preparation errors apply only to
// new writes, while the final conditional save fences the prepared version.
func (workflow *ArticleOperations) Patch(ctx context.Context, key string, input article.Patch) (article.Result, error) {
	prepared, prepareErr := workflow.dependencies.ArticleImages.PreparePatch(ctx, input)
	return workflow.execute(ctx, key, func(tx *gorm.DB) (article.Result, error) {
		if prepareErr != nil {
			return article.Result{}, stable(prepareErr)
		}
		return workflow.dependencies.ArticleImages.PatchInTx(ctx, tx, prepared)
	})
}

// Publish arbitrates before validating the lifecycle request.
func (workflow *ArticleOperations) Publish(ctx context.Context, key string, input article.Publish) (article.Result, error) {
	return workflow.execute(ctx, key, func(tx *gorm.DB) (article.Result, error) {
		return workflow.dependencies.ArticleImages.PublishInTx(ctx, tx, input)
	})
}

// Archive arbitrates before validating the lifecycle request.
func (workflow *ArticleOperations) Archive(ctx context.Context, key string, input article.Archive) (article.Result, error) {
	return workflow.execute(ctx, key, func(tx *gorm.DB) (article.Result, error) {
		return workflow.dependencies.ArticleImages.ArchiveInTx(ctx, tx, input)
	})
}
