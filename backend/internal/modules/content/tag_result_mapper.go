package content

import (
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func tagResult(item *domain.Tag) TagResult {
	return TagResult{ID: item.ID().Int64(), Name: item.Name().String(), Version: item.Version().Uint64(), CreatedAt: item.CreatedAt(), ModifiedAt: item.ModifiedAt()}
}

func tagViewResult(view application.TagView) TagResult {
	return TagResult{ID: view.ID.Int64(), Name: view.Name.String(), Version: view.Version.Uint64(), CreatedAt: view.CreatedAt, ModifiedAt: view.ModifiedAt}
}
