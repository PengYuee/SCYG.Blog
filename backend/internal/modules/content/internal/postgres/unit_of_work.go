package postgres

import (
	"context"
	"errors"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	"gorm.io/gorm"
)

// UnitOfWork adapts platform transactions to content transaction-scoped repositories.
type UnitOfWork struct{ transaction *database.UnitOfWork }

// NewUnitOfWork creates the content transaction adapter.
func NewUnitOfWork(transaction *database.UnitOfWork) (*UnitOfWork, error) {
	if transaction == nil {
		return nil, errors.New("content postgres unit of work is nil")
	}
	return &UnitOfWork{transaction: transaction}, nil
}

// Within executes content command repositories on exactly one transaction handle.
func (unit *UnitOfWork) Within(ctx context.Context, callback func(context.Context, application.Transaction) error) error {
	if callback == nil {
		return errors.New("content transaction callback is nil")
	}
	return translate(unit.transaction.WithinTransaction(ctx, func(transactionContext context.Context, handle *gorm.DB) error {
		return callback(transactionContext, &repositories{articles: &articleRepository{db: handle}, articleTypes: &articleTypeRepository{db: handle}, tags: &tagRepository{db: handle}})
	}))
}

type repositories struct {
	articles     *articleRepository
	articleTypes *articleTypeRepository
	tags         *tagRepository
}

func (repos *repositories) Articles() application.ArticleRepository { return repos.articles }
func (repos *repositories) ArticleTypes() application.ArticleTypeRepository {
	return repos.articleTypes
}
func (repos *repositories) Tags() application.TagRepository { return repos.tags }

var _ application.UnitOfWork = (*UnitOfWork)(nil)
