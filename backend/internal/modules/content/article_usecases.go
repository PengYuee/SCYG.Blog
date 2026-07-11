package content

import (
	"context"
	"fmt"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// CreateArticle validates, authorizes, and persists one draft in a transaction.
func (module *Module) CreateArticle(ctx context.Context, command CreateArticle) (ArticleResult, error) {
	draft, err := parseDraft(command)
	if err != nil {
		return ArticleResult{}, validation(err)
	}
	if err = module.authorizer.Authorize(ctx, ActionCreateArticle, Resource{Kind: "article"}); err != nil {
		return ArticleResult{}, permission(err)
	}
	var result ArticleResult
	err = module.unit.Within(ctx, func(transactionContext context.Context, transaction application.Transaction) error {
		id, reserveErr := transaction.Articles().NextID(transactionContext)
		if reserveErr != nil {
			return reserveErr
		}
		draft.ID = id
		article, createErr := domain.NewArticle(draft, module.clock)
		if createErr != nil {
			return createErr
		}
		if saveErr := transaction.Articles().Save(transactionContext, article); saveErr != nil {
			return saveErr
		}
		result = articleResult(article)
		return nil
	})
	if err != nil {
		return ArticleResult{}, stable(err)
	}
	return result, nil
}

// ReviseArticle revises a mutable article in a transaction.
func (module *Module) ReviseArticle(ctx context.Context, command ReviseArticle) (ArticleResult, error) {
	id, version, revision, err := parseRevision(command)
	if err != nil {
		return ArticleResult{}, validation(err)
	}
	if err = module.authorizer.Authorize(ctx, ActionReviseArticle, Resource{Kind: "article", ID: command.ID}); err != nil {
		return ArticleResult{}, permission(err)
	}
	return module.changeArticle(ctx, id, func(article *domain.Article) error { return article.Revise(version, revision, module.clock) })
}

// PublishArticle publishes a draft in a transaction.
func (module *Module) PublishArticle(ctx context.Context, command PublishArticle) (ArticleResult, error) {
	return module.transitionArticle(ctx, command.ID, command.Version, ActionPublishArticle, func(article *domain.Article, version domain.Version) error {
		return article.Publish(version, module.clock)
	})
}

// ArchiveArticle archives a published article in a transaction.
func (module *Module) ArchiveArticle(ctx context.Context, command ArchiveArticle) (ArticleResult, error) {
	return module.transitionArticle(ctx, command.ID, command.Version, ActionArchiveArticle, func(article *domain.Article, version domain.Version) error {
		return article.Archive(version, module.clock)
	})
}

// DeleteArticle soft-deletes an article in a transaction.
func (module *Module) DeleteArticle(ctx context.Context, command DeleteArticle) error {
	id, version, err := parseIdentity(command.ID, command.Version)
	if err != nil {
		return validation(err)
	}
	if err = module.authorizer.Authorize(ctx, ActionDeleteArticle, Resource{Kind: "article", ID: command.ID}); err != nil {
		return permission(err)
	}
	_, err = module.changeArticle(ctx, id, func(article *domain.Article) error { return article.Delete(version, module.clock) })
	return err
}

func (module *Module) transitionArticle(ctx context.Context, rawID int64, rawVersion uint64, action Action, transition func(*domain.Article, domain.Version) error) (ArticleResult, error) {
	id, version, err := parseIdentity(rawID, rawVersion)
	if err != nil {
		return ArticleResult{}, validation(err)
	}
	if err = module.authorizer.Authorize(ctx, action, Resource{Kind: "article", ID: rawID}); err != nil {
		return ArticleResult{}, permission(err)
	}
	return module.changeArticle(ctx, id, func(article *domain.Article) error { return transition(article, version) })
}

func (module *Module) changeArticle(ctx context.Context, id domain.ArticleID, change func(*domain.Article) error) (ArticleResult, error) {
	var result ArticleResult
	err := module.unit.Within(ctx, func(transactionContext context.Context, transaction application.Transaction) error {
		article, findErr := transaction.Articles().Find(transactionContext, id)
		if findErr != nil {
			return findErr
		}
		if changeErr := change(article); changeErr != nil {
			return changeErr
		}
		if saveErr := transaction.Articles().Save(transactionContext, article); saveErr != nil {
			return saveErr
		}
		result = articleResult(article)
		return nil
	})
	if err != nil {
		return ArticleResult{}, stable(err)
	}
	return result, nil
}

// GetArticle returns one published article projection.
func (module *Module) GetArticle(ctx context.Context, query GetArticle) (ArticleResult, error) {
	id, err := domain.NewArticleID(query.ID)
	if err != nil {
		return ArticleResult{}, validation(err)
	}
	view, err := module.articles.FindPublished(ctx, id)
	if err != nil {
		return ArticleResult{}, stable(err)
	}
	return articleViewResult(view), nil
}

// ListArticles returns a bounded page of published article projections.
func (module *Module) ListArticles(ctx context.Context, query ListArticles) (ArticlePage, error) {
	filter, err := parseFilter(query)
	if err != nil {
		return ArticlePage{}, validation(err)
	}
	page, err := module.articles.ListPublished(ctx, filter)
	if err != nil {
		return ArticlePage{}, stable(err)
	}
	items := make([]ArticleResult, len(page.Items))
	for index, item := range page.Items {
		items[index] = articleViewResult(item)
	}
	return ArticlePage{Items: items, Number: page.Number, Size: page.Size, TotalItems: page.TotalItems, TotalPages: page.TotalPages}, nil
}

func invalidCommand(field string) error { return fmt.Errorf("%s: %w", field, domain.ErrInvalidValue) }
