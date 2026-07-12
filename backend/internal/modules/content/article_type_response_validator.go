package content

import "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"

// ValidateArticleTypeResponseText 对分类读取结果应用权威领域文本解析。
func ValidateArticleTypeResponseText(result ArticleTypeResult) error {
	if _, err := domain.NewName(result.Name); err != nil {
		return err
	}
	return domain.ValidateImage(result.Image)
}
