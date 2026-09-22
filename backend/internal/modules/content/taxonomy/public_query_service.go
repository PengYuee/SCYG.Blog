package taxonomy

import "context"

// GetPublicArticleType reads an article type only when it has published articles.
func (service *Service) GetPublicArticleType(ctx context.Context, query GetPublicArticleType) (PublicArticleTypeResult, error) {
	if err := parseID(query.ID, "article_type_id"); err != nil {
		return PublicArticleTypeResult{}, err
	}
	row, err := service.repo.internal().findPublicArticleType(ctx, query.ID)
	if err != nil {
		return PublicArticleTypeResult{}, stable(err)
	}
	result, err := publicArticleTypeResult(row)
	if err != nil {
		return PublicArticleTypeResult{}, stable(err)
	}
	return result, nil
}

// ListPublicArticleTypes reads only article types referenced by published articles.
func (service *Service) ListPublicArticleTypes(ctx context.Context, query ListPublicArticleTypes) (PublicArticleTypePage, error) {
	if err := validPublicPage(query.Page, query.PageSize); err != nil {
		return PublicArticleTypePage{}, err
	}
	rows, total, err := service.repo.internal().listPublicArticleTypes(ctx, query.Name, query.Page, query.PageSize)
	if err != nil {
		return PublicArticleTypePage{}, stable(err)
	}
	items := make([]PublicArticleTypeResult, len(rows))
	for index, row := range rows {
		items[index], err = publicArticleTypeResult(row)
		if err != nil {
			return PublicArticleTypePage{}, stable(err)
		}
	}
	return PublicArticleTypePage{Items: items, Number: query.Page, Size: query.PageSize, TotalItems: total, TotalPages: pageCount(total, query.PageSize)}, nil
}

// GetPublicTag reads a tag only when it is attached to published articles.
func (service *Service) GetPublicTag(ctx context.Context, query GetPublicTag) (PublicTagResult, error) {
	if err := parseID(query.ID, "tag_id"); err != nil {
		return PublicTagResult{}, err
	}
	row, err := service.repo.internal().findPublicTag(ctx, query.ID)
	if err != nil {
		return PublicTagResult{}, stable(err)
	}
	result, err := publicTagResult(row)
	if err != nil {
		return PublicTagResult{}, stable(err)
	}
	return result, nil
}

// ListPublicTags reads only tags attached to published articles.
func (service *Service) ListPublicTags(ctx context.Context, query ListPublicTags) (PublicTagPage, error) {
	if err := validPublicPage(query.Page, query.PageSize); err != nil {
		return PublicTagPage{}, err
	}
	rows, total, err := service.repo.internal().listPublicTags(ctx, query.Name, query.Page, query.PageSize)
	if err != nil {
		return PublicTagPage{}, stable(err)
	}
	items := make([]PublicTagResult, len(rows))
	for index, row := range rows {
		items[index], err = publicTagResult(row)
		if err != nil {
			return PublicTagPage{}, stable(err)
		}
	}
	return PublicTagPage{Items: items, Number: query.Page, Size: query.PageSize, TotalItems: total, TotalPages: pageCount(total, query.PageSize)}, nil
}

func validPublicPage(page, size int) error {
	if page < 1 || size < 1 || size > 100 {
		return invalid("page")
	}
	return nil
}
