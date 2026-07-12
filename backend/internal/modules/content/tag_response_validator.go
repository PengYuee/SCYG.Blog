package content

import "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"

// ValidateTagResponseText 对标签读取结果应用权威领域名称解析。
func ValidateTagResponseText(result TagResult) error {
	_, err := domain.NewName(result.Name)
	return err
}
