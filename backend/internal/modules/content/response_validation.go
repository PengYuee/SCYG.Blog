package content

import (
	"fmt"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// ValidateArticleResponseText 对读取结果应用权威领域文本解析，阻止非法数据进入 HTTP 响应。
func ValidateArticleResponseText(result ArticleResult) error {
	title, err := domain.NewTitle(result.Title)
	if err != nil {
		return err
	}
	if title.String() != result.Title {
		return fmt.Errorf("文章响应标题尚未规范化")
	}
	slug, err := domain.NewSlug(result.Slug)
	if err != nil {
		return err
	}
	// 响应值必须已经由领域层规范化；传输层不得静默修正内部数据后继续返回成功。
	if slug.String() != result.Slug {
		return fmt.Errorf("文章响应 slug 尚未规范化")
	}
	digest, err := domain.NewDigest(result.Digest)
	if err != nil {
		return err
	}
	if digest.String() != result.Digest {
		return fmt.Errorf("文章响应摘要尚未规范化")
	}
	content, err := domain.NewContent(result.Content)
	if err != nil {
		return err
	}
	if content.String() != result.Content {
		return fmt.Errorf("文章响应正文尚未规范化")
	}
	return nil
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
