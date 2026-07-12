package content

import (
	"context"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

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
