package taxonomy

import (
	"context"
)

// GetTag reads one non-deleted tag.
func (service *Service) GetTag(ctx context.Context, query GetTag) (TagResult, error) {
	if err := parseID(query.ID, "tag_id"); err != nil {
		return TagResult{}, err
	}
	entity, err := service.repo.internal().findTag(ctx, query.ID, false)
	if err != nil {
		return TagResult{}, stable(err)
	}
	return tagResult(entity), nil
}

// ListTags reads filtered and paginated tags.
func (service *Service) ListTags(ctx context.Context, query ListTags) (TagPage, error) {
	if err := validPage(query.Page, query.PageSize, query.Sort); err != nil {
		return TagPage{}, err
	}
	items, total, err := service.repo.internal().listTags(ctx, query.Name, query.Page, query.PageSize, query.Sort)
	if err != nil {
		return TagPage{}, stable(err)
	}
	result := make([]TagResult, len(items))
	for index, item := range items {
		result[index] = tagResult(item)
	}
	return TagPage{Items: result, Number: query.Page, Size: query.PageSize, TotalItems: total, TotalPages: pageCount(total, query.PageSize)}, nil
}
