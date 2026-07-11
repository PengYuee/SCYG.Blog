package content

import "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"

// ValidateArticleResponseText applies authoritative domain text parsing to a read result.
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

// ValidateArticleTypeResponseText applies authoritative taxonomy text parsing to a read result.
func ValidateArticleTypeResponseText(result ArticleTypeResult) error {
	if _, err := domain.NewName(result.Name); err != nil {
		return err
	}
	return domain.ValidateImage(result.Image)
}

// ValidateTagResponseText applies authoritative taxonomy name parsing to a read result.
func ValidateTagResponseText(result TagResult) error {
	_, err := domain.NewName(result.Name)
	return err
}
