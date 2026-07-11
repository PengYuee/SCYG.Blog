package content

import "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"

// ValidateArticleResponseText 对读取结果应用权威领域文本解析，阻止非法数据进入 HTTP 响应。
func ValidateArticleResponseText(result ArticleResult) error {
	if _, err := domain.NewTitle(result.Title); err != nil {
		return err
	}
	if _, err := domain.NewSlug(result.Slug); err != nil {
		return err
	}
	if _, err := domain.NewDigest(result.Digest); err != nil {
		return err
	}
	_, err := domain.NewContent(result.Content)
	return err
}

// ValidateArticleTypeResponseText 对分类读取结果应用权威领域文本解析。
func ValidateArticleTypeResponseText(result ArticleTypeResult) error {
	if _, err := domain.NewName(result.Name); err != nil {
		return err
	}
	return domain.ValidateImage(result.Image)
}

// ValidateTagResponseText 对标签读取结果应用权威领域名称解析。
func ValidateTagResponseText(result TagResult) error {
	_, err := domain.NewName(result.Name)
	return err
}
