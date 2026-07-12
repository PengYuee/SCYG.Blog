package application

import (
	"context"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// TaxonomyReadModel 提供文章分类和标签投影。
type TaxonomyReadModel interface {
	FindArticleType(context.Context, domain.ArticleTypeID) (ArticleTypeView, error)
	ListArticleTypes(context.Context, string) ([]ArticleTypeView, error)
	FindTag(context.Context, domain.TagID) (TagView, error)
	ListTags(context.Context, string) ([]TagView, error)
}
