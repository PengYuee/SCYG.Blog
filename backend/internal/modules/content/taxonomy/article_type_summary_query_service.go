package taxonomy

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ArticleTypeSummary is the read-only category data used in article responses.
type ArticleTypeSummary struct {
	// ID identifies the article category.
	ID int64
	// Name is the category's validated display name.
	Name string
	// Image is the optional category image.
	Image *string
}

// GetArticleTypeSummariesInTx reads all requested live categories on the caller's
// snapshot. lock holds shared category locks until a write transaction completes;
// read-only snapshot transactions must pass false.
func (service *Service) GetArticleTypeSummariesInTx(ctx context.Context, tx *gorm.DB, ids []int64, lock bool) ([]ArticleTypeSummary, error) {
	if tx == nil {
		return nil, stable(errors.New("article type summary transaction is nil"))
	}
	unique := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if err := parseID(id, "article_type_id"); err != nil {
			return nil, err
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}
	if len(unique) == 0 {
		return []ArticleTypeSummary{}, nil
	}
	items, err := (&repository{db: tx}).articleTypeSummaries(ctx, unique, lock)
	return items, stable(err)
}
