package content

import (
	"context"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// CreateTag validates, authorizes, and persists one tag.
func (module *Module) CreateTag(ctx context.Context, command CreateTag) (TagResult, error) {
	name, err := domain.NewName(command.Name)
	if err != nil {
		return TagResult{}, validation(err)
	}
	if err = module.authorizer.Authorize(ctx, ActionManageTag, Resource{Kind: "tag"}); err != nil {
		return TagResult{}, permission(err)
	}
	var result TagResult
	err = module.unit.Within(ctx, func(transactionContext context.Context, transaction application.Transaction) error {
		id, reserveErr := transaction.Tags().NextID(transactionContext)
		if reserveErr != nil {
			return reserveErr
		}
		tag, createErr := domain.NewTag(id, name, module.clock)
		if createErr != nil {
			return createErr
		}
		if saveErr := transaction.Tags().Save(transactionContext, tag); saveErr != nil {
			return saveErr
		}
		result = tagResult(tag)
		return nil
	})
	if err != nil {
		return TagResult{}, stable(err)
	}
	return result, nil
}

// RenameTag renames one tag in a transaction.
func (module *Module) RenameTag(ctx context.Context, command RenameTag) (TagResult, error) {
	id, err := domain.NewTagID(command.ID)
	if err != nil {
		return TagResult{}, validation(err)
	}
	version, err := domain.NewVersion(command.Version)
	if err != nil {
		return TagResult{}, validation(err)
	}
	name, err := domain.NewName(command.Name)
	if err != nil {
		return TagResult{}, validation(err)
	}
	if err = module.authorizer.Authorize(ctx, ActionManageTag, Resource{Kind: "tag", ID: command.ID}); err != nil {
		return TagResult{}, permission(err)
	}
	var result TagResult
	err = module.unit.Within(ctx, func(transactionContext context.Context, transaction application.Transaction) error {
		tag, findErr := transaction.Tags().Find(transactionContext, id)
		if findErr != nil {
			return findErr
		}
		if renameErr := tag.Rename(version, name, module.clock); renameErr != nil {
			return renameErr
		}
		if saveErr := transaction.Tags().Save(transactionContext, tag); saveErr != nil {
			return saveErr
		}
		result = tagResult(tag)
		return nil
	})
	if err != nil {
		return TagResult{}, stable(err)
	}
	return result, nil
}

// DeleteTag soft-deletes one tag in a transaction.
func (module *Module) DeleteTag(ctx context.Context, command DeleteTag) error {
	id, err := domain.NewTagID(command.ID)
	if err != nil {
		return validation(err)
	}
	version, err := domain.NewVersion(command.Version)
	if err != nil {
		return validation(err)
	}
	if err = module.authorizer.Authorize(ctx, ActionManageTag, Resource{Kind: "tag", ID: command.ID}); err != nil {
		return permission(err)
	}
	err = module.unit.Within(ctx, func(transactionContext context.Context, transaction application.Transaction) error {
		tag, findErr := transaction.Tags().Find(transactionContext, id)
		if findErr != nil {
			return findErr
		}
		if deleteErr := tag.Delete(version, module.clock); deleteErr != nil {
			return deleteErr
		}
		return transaction.Tags().Save(transactionContext, tag)
	})
	return stable(err)
}

// GetTag returns one nondeleted tag projection.
func (module *Module) GetTag(ctx context.Context, query GetTag) (TagResult, error) {
	id, err := domain.NewTagID(query.ID)
	if err != nil {
		return TagResult{}, validation(err)
	}
	view, err := module.taxonomies.FindTag(ctx, id)
	if err != nil {
		return TagResult{}, stable(err)
	}
	return tagViewResult(view), nil
}

// ListTags returns nondeleted tag projections.

func (module *Module) ListTags(ctx context.Context, query ListTags) (TagPage, error) {
	if err := validTaxonomyPage(query.Page, query.PageSize, query.Sort); err != nil {
		return TagPage{}, validation(err)
	}
	views, err := module.taxonomies.ListTags(ctx, query.Name)
	if err != nil {
		return TagPage{}, stable(err)
	}
	items := make([]TagResult, len(views))
	for index, view := range views {
		items[index] = tagViewResult(view)
	}
	sortTags(items, query.Sort)
	start, end := pageRange(len(items), query.Page, query.PageSize)
	return TagPage{Items: items[start:end], Number: query.Page, Size: query.PageSize, TotalItems: int64(len(items)), TotalPages: pages(len(items), query.PageSize)}, nil
}
