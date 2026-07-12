package content

import (
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func articleTypeResult(item *domain.ArticleType) ArticleTypeResult {
	return ArticleTypeResult{ID: item.ID().Int64(), Name: item.Name().String(), Image: item.Image(), Meun: item.Meun(), Version: item.Version().Uint64(), CreatedAt: item.CreatedAt(), ModifiedAt: item.ModifiedAt()}
}

func articleTypeViewResult(view application.ArticleTypeView) ArticleTypeResult {
	return ArticleTypeResult{ID: view.ID.Int64(), Name: view.Name.String(), Image: view.Image, Meun: view.Meun, Version: view.Version.Uint64(), CreatedAt: view.CreatedAt, ModifiedAt: view.ModifiedAt}
}
