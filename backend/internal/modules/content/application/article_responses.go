package application

import (
	"context"
	"database/sql"
	"errors"

	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
)

// ArticleResponses assembles article/category responses within a shared read
// snapshot or the transaction that writes the article. Features own their SQL.
type ArticleResponses struct {
	db       *gorm.DB
	articles *article.Service
	taxonomy *taxonomy.Service
	images   *ArticleImages
}

// NewArticleResponses constructs the cross-feature article response workflow.
func NewArticleResponses(db *gorm.DB, articles *article.Service, categories *taxonomy.Service, images *ArticleImages) (*ArticleResponses, error) {
	if db == nil || articles == nil || categories == nil || images == nil {
		return nil, errors.New("article response dependencies are nil")
	}
	return &ArticleResponses{db: db, articles: articles, taxonomy: categories, images: images}, nil
}

// Get reads a published article and its category in one repeatable-read snapshot.
func (workflow *ArticleResponses) Get(ctx context.Context, input article.Get) (article.Result, error) {
	return workflow.get(ctx, input, false)
}

// GetManage authorizes and reads an article/category management snapshot.
func (workflow *ArticleResponses) GetManage(ctx context.Context, input article.Get) (article.Result, error) {
	return workflow.get(ctx, input, true)
}

func (workflow *ArticleResponses) get(ctx context.Context, input article.Get, management bool) (article.Result, error) {
	var out article.Result
	err := workflow.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		if management {
			out, err = workflow.articles.GetManageInTx(ctx, tx, input)
		} else {
			out, err = workflow.articles.GetInTx(ctx, tx, input)
		}
		if err != nil {
			return err
		}
		items := []article.Result{out}
		err = workflow.summaries(ctx, tx, items, false)
		out = items[0]
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return article.Result{}, stable(err)
	}
	return out, nil
}

// List reads a published page and category summaries in one database snapshot.
func (workflow *ArticleResponses) List(ctx context.Context, input article.List) (article.Page, error) {
	return workflow.list(ctx, input, false)
}

// ListManage authorizes and reads a management page/category snapshot.
func (workflow *ArticleResponses) ListManage(ctx context.Context, input article.List) (article.Page, error) {
	return workflow.list(ctx, input, true)
}

func (workflow *ArticleResponses) list(ctx context.Context, input article.List, management bool) (article.Page, error) {
	var out article.Page
	err := workflow.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		if management {
			out, err = workflow.articles.ListManageInTx(ctx, tx, input)
		} else {
			out, err = workflow.articles.ListInTx(ctx, tx, input)
		}
		if err != nil {
			return err
		}
		return workflow.summaries(ctx, tx, out.Items, false)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return article.Page{}, stable(err)
	}
	return out, nil
}

func (workflow *ArticleResponses) summaries(ctx context.Context, tx *gorm.DB, items []article.Result, lock bool) error {
	ids := make([]int64, len(items))
	for index, item := range items {
		ids[index] = item.ArticleTypeID
	}
	categories, err := workflow.taxonomy.GetArticleTypeSummariesInTx(ctx, tx, ids, lock)
	if err != nil {
		return &Error{Code: "internal", Cause: err}
	}
	byID := make(map[int64]taxonomy.ArticleTypeSummary, len(categories))
	for _, category := range categories {
		byID[category.ID] = category
	}
	for index := range items {
		category, exists := byID[items[index].ArticleTypeID]
		if !exists {
			return errors.New("article response category is missing")
		}
		items[index].ArticleTypeName = category.Name
		items[index].ArticleTypeImage = category.Image
	}
	return nil
}

// Create prepares Blob references outside the transaction and returns the exact
// newly written article with category data assembled before commit.
func (workflow *ArticleResponses) Create(ctx context.Context, input article.Create) (article.Result, error) {
	prepared, err := workflow.images.PrepareCreate(ctx, input)
	if err != nil {
		return article.Result{}, stable(err)
	}
	return workflow.write(ctx, func(tx *gorm.DB) (article.Result, error) {
		return workflow.images.CreateInTx(ctx, tx, prepared)
	})
}

// Patch preserves tag presence and prepares Blob references before the write transaction.
func (workflow *ArticleResponses) Patch(ctx context.Context, input article.Patch) (article.Result, error) {
	prepared, err := workflow.images.PreparePatch(ctx, input)
	if err != nil {
		return article.Result{}, stable(err)
	}
	return workflow.write(ctx, func(tx *gorm.DB) (article.Result, error) {
		return workflow.images.PatchInTx(ctx, tx, prepared)
	})
}

// Publish returns the exact transition result with its category before commit.
func (workflow *ArticleResponses) Publish(ctx context.Context, input article.Publish) (article.Result, error) {
	return workflow.write(ctx, func(tx *gorm.DB) (article.Result, error) {
		return workflow.images.PublishInTx(ctx, tx, input)
	})
}

// Archive returns the exact transition result with its category before commit.
func (workflow *ArticleResponses) Archive(ctx context.Context, input article.Archive) (article.Result, error) {
	return workflow.write(ctx, func(tx *gorm.DB) (article.Result, error) {
		return workflow.images.ArchiveInTx(ctx, tx, input)
	})
}

func (workflow *ArticleResponses) write(ctx context.Context, change func(*gorm.DB) (article.Result, error)) (article.Result, error) {
	var out article.Result
	err := workflow.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		out, err = change(tx)
		if err != nil {
			return err
		}
		items := []article.Result{out}
		err = workflow.summaries(ctx, tx, items, true)
		out = items[0]
		return err
	})
	if err != nil {
		return article.Result{}, stable(err)
	}
	return out, nil
}
