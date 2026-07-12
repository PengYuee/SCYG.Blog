package content

import (
	"strings"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func parseFilter(query ListArticles) (application.ArticleFilter, error) {
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 100 {
		return application.ArticleFilter{}, invalidCommand("page")
	}
	filter := application.ArticleFilter{Page: query.Page, PageSize: query.PageSize, Query: strings.TrimSpace(query.Query), Sort: query.Sort}
	var err error
	if query.ArticleTypeID != 0 {
		filter.ArticleTypeID, err = domain.NewArticleTypeID(query.ArticleTypeID)
		if err != nil {
			return application.ArticleFilter{}, err
		}
	}
	if query.TagID != 0 {
		filter.TagID, err = domain.NewTagID(query.TagID)
		if err != nil {
			return application.ArticleFilter{}, err
		}
	}
	switch query.Sort {
	case "", "newest", "oldest", "title", "title_desc":
	default:
		return application.ArticleFilter{}, invalidCommand("sort")
	}
	return filter, nil
}
