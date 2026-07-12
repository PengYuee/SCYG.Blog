package content

import (
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func articleResult(article *domain.Article) ArticleResult {
	tags := article.TagIDs()
	tagIDs := make([]int64, len(tags))
	for index, id := range tags {
		tagIDs[index] = id.Int64()
	}
	return ArticleResult{ID: article.ID().Int64(), ArticleTypeID: article.ArticleTypeID().Int64(), Title: article.Title().String(), Slug: article.Slug().String(), Digest: article.Digest().String(), Content: article.Content().String(), Status: string(article.Status()), TagIDs: tagIDs, Version: article.Version().Uint64(), CreatedAt: article.CreatedAt(), ModifiedAt: article.ModifiedAt()}
}

func articleViewResult(view application.ArticleView) ArticleResult {
	tagIDs := make([]int64, len(view.TagIDs))
	for index, id := range view.TagIDs {
		tagIDs[index] = id.Int64()
	}
	return ArticleResult{ID: view.ID.Int64(), ArticleTypeID: view.ArticleTypeID.Int64(), Title: view.Title.String(), Slug: view.Slug.String(), Digest: view.Digest.String(), Content: view.Content.String(), Status: string(view.Status), TagIDs: tagIDs, Support: view.Support, Comment: view.Comment, Visited: view.Visited, Version: view.Version.Uint64(), CreatedAt: view.CreatedAt, ModifiedAt: view.ModifiedAt}
}
