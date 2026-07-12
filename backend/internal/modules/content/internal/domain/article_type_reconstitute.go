package domain

import "time"

// TaxonomyState 包含仅供内部适配器使用的可信分类持久化状态。
type TaxonomyState[ID ArticleTypeID | TagID] struct {
	ID         ID
	Name       Name
	Version    Version
	CreatedAt  time.Time
	ModifiedAt time.Time
	DeletedAt  time.Time
}

// ArticleTypeState 包含可信文章分类持久化状态。
type ArticleTypeState struct {
	TaxonomyState[ArticleTypeID]
	Image *string
	Meun  int32
}

// ReconstituteArticleType 通过领域不变量恢复持久化文章分类。
func ReconstituteArticleType(state ArticleTypeState) (*ArticleType, error) {
	if !state.ID.valid() || !state.Name.valid() || !state.Version.valid() || state.CreatedAt.IsZero() || state.ModifiedAt.Before(state.CreatedAt) {
		return nil, ErrInvalidValue
	}
	image, err := parseImage(state.Image)
	if err != nil || state.Meun < 0 {
		return nil, ErrInvalidValue
	}
	return &ArticleType{id: state.ID, name: state.Name, image: image, meun: state.Meun, version: state.Version, createdAt: state.CreatedAt, modifiedAt: state.ModifiedAt, deletedAt: state.DeletedAt}, nil
}
