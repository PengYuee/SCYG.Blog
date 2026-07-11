package content

import (
	"context"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// CreateArticleType validates, authorizes, and persists one article type.
func (module *Module) CreateArticleType(ctx context.Context, command CreateArticleType) (ArticleTypeResult, error) {
	name, err := domain.NewName(command.Name)
	if err != nil {
		return ArticleTypeResult{}, validation(err)
	}
	if err = module.authorizer.Authorize(ctx, ActionManageArticleType, Resource{Kind: "article_type"}); err != nil {
		return ArticleTypeResult{}, permission(err)
	}
	var result ArticleTypeResult
	err = module.unit.Within(ctx, func(transactionContext context.Context, transaction application.Transaction) error {
		id, reserveErr := transaction.ArticleTypes().NextID(transactionContext)
		if reserveErr != nil {
			return reserveErr
		}
		item, createErr := domain.NewArticleTypeWithDetails(id, name, command.Image, command.Meun, module.clock)
		if createErr != nil {
			return createErr
		}
		if saveErr := transaction.ArticleTypes().Save(transactionContext, item); saveErr != nil {
			return saveErr
		}
		result = articleTypeResult(item)
		return nil
	})
	if err != nil {
		return ArticleTypeResult{}, stable(err)
	}
	return result, nil
}

// PatchArticleType partially updates one article type in a transaction.
func (module *Module) PatchArticleType(ctx context.Context, command PatchArticleType) (ArticleTypeResult, error) {
	id, err := domain.NewArticleTypeID(command.ID)
	if err != nil {
		return ArticleTypeResult{}, validation(err)
	}
	version, err := domain.NewVersion(command.Version)
	if err != nil {
		return ArticleTypeResult{}, validation(err)
	}
	var name *domain.Name
	if command.Name != nil {
		parsed, parseErr := domain.NewName(*command.Name)
		if parseErr != nil {
			return ArticleTypeResult{}, validation(parseErr)
		}
		name = &parsed
	}
	if command.Name == nil && !command.Image.Provided && command.Meun == nil {
		return ArticleTypeResult{}, validation(invalidCommand("patch"))
	}
	if err = module.authorizer.Authorize(ctx, ActionManageArticleType, Resource{Kind: "article_type", ID: command.ID}); err != nil {
		return ArticleTypeResult{}, permission(err)
	}
	var result ArticleTypeResult
	err = module.unit.Within(ctx, func(transactionContext context.Context, transaction application.Transaction) error {
		item, findErr := transaction.ArticleTypes().Find(transactionContext, id)
		if findErr != nil {
			return findErr
		}
		patch := domain.ArticleTypePatch{Name: name, ImageProvided: command.Image.Provided, Image: command.Image.Value, Meun: command.Meun}
		if patchErr := item.Patch(version, patch, module.clock); patchErr != nil {
			return patchErr
		}
		if saveErr := transaction.ArticleTypes().Save(transactionContext, item); saveErr != nil {
			return saveErr
		}
		result = articleTypeResult(item)
		return nil
	})
	if err != nil {
		return ArticleTypeResult{}, stable(err)
	}
	return result, nil
}

// RenameArticleType renames one article type in a transaction.
func (module *Module) RenameArticleType(ctx context.Context, command RenameArticleType) (ArticleTypeResult, error) {
	id, err := domain.NewArticleTypeID(command.ID)
	if err != nil {
		return ArticleTypeResult{}, validation(err)
	}
	version, err := domain.NewVersion(command.Version)
	if err != nil {
		return ArticleTypeResult{}, validation(err)
	}
	name, err := domain.NewName(command.Name)
	if err != nil {
		return ArticleTypeResult{}, validation(err)
	}
	if err = module.authorizer.Authorize(ctx, ActionManageArticleType, Resource{Kind: "article_type", ID: command.ID}); err != nil {
		return ArticleTypeResult{}, permission(err)
	}
	var result ArticleTypeResult
	err = module.unit.Within(ctx, func(transactionContext context.Context, transaction application.Transaction) error {
		item, findErr := transaction.ArticleTypes().Find(transactionContext, id)
		if findErr != nil {
			return findErr
		}
		if renameErr := item.Rename(version, name, module.clock); renameErr != nil {
			return renameErr
		}
		if saveErr := transaction.ArticleTypes().Save(transactionContext, item); saveErr != nil {
			return saveErr
		}
		result = articleTypeResult(item)
		return nil
	})
	if err != nil {
		return ArticleTypeResult{}, stable(err)
	}
	return result, nil
}

// DeleteArticleType soft-deletes one article type in a transaction.
func (module *Module) DeleteArticleType(ctx context.Context, command DeleteArticleType) error {
	id, err := domain.NewArticleTypeID(command.ID)
	if err != nil {
		return validation(err)
	}
	version, err := domain.NewVersion(command.Version)
	if err != nil {
		return validation(err)
	}
	if err = module.authorizer.Authorize(ctx, ActionManageArticleType, Resource{Kind: "article_type", ID: command.ID}); err != nil {
		return permission(err)
	}
	err = module.unit.Within(ctx, func(transactionContext context.Context, transaction application.Transaction) error {
		item, findErr := transaction.ArticleTypes().Find(transactionContext, id)
		if findErr != nil {
			return findErr
		}
		if deleteErr := item.Delete(version, module.clock); deleteErr != nil {
			return deleteErr
		}
		return transaction.ArticleTypes().Save(transactionContext, item)
	})
	return stable(err)
}

// GetArticleType returns one nondeleted article type projection.
func (module *Module) GetArticleType(ctx context.Context, query GetArticleType) (ArticleTypeResult, error) {
	id, err := domain.NewArticleTypeID(query.ID)
	if err != nil {
		return ArticleTypeResult{}, validation(err)
	}
	view, err := module.taxonomies.FindArticleType(ctx, id)
	if err != nil {
		return ArticleTypeResult{}, stable(err)
	}
	return articleTypeViewResult(view), nil
}

// ListArticleTypes returns nondeleted article type projections.

func (module *Module) ListArticleTypes(ctx context.Context, query ListArticleTypes) (ArticleTypePage, error) {
	if err := validTaxonomyPage(query.Page, query.PageSize, query.Sort); err != nil {
		return ArticleTypePage{}, validation(err)
	}
	views, err := module.taxonomies.ListArticleTypes(ctx, query.Name)
	if err != nil {
		return ArticleTypePage{}, stable(err)
	}
	items := make([]ArticleTypeResult, len(views))
	for index, view := range views {
		items[index] = articleTypeViewResult(view)
	}
	sortArticleTypes(items, query.Sort)
	start, end := pageRange(len(items), query.Page, query.PageSize)
	return ArticleTypePage{Items: items[start:end], Number: query.Page, Size: query.PageSize, TotalItems: int64(len(items)), TotalPages: pages(len(items), query.PageSize)}, nil
}
