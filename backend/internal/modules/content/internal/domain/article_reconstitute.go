package domain

import "time"

// ArticleState 包含仅供内部适配器使用的可信持久化状态。
type ArticleState struct {
	ID            ArticleID
	ArticleTypeID ArticleTypeID
	Title         Title
	Slug          Slug
	Digest        Digest
	Content       Content
	Status        Status
	TagIDs        []TagID
	Version       Version
	CreatedAt     time.Time
	ModifiedAt    time.Time
	DeletedAt     time.Time
}

// ReconstituteArticle 通过领域不变量恢复持久化聚合。
func ReconstituteArticle(state ArticleState) (*Article, error) {
	if err := validateDraft(ArticleDraft{ID: state.ID, ArticleTypeID: state.ArticleTypeID, Title: state.Title, Slug: state.Slug, Digest: state.Digest, Content: state.Content, TagIDs: state.TagIDs}); err != nil {
		return nil, err
	}
	tags, err := uniqueTags(state.TagIDs)
	if err != nil {
		return nil, err
	}
	if !state.Version.valid() || state.CreatedAt.IsZero() || state.ModifiedAt.Before(state.CreatedAt) {
		return nil, ErrInvalidValue
	}
	if _, err = ParseStatus(string(state.Status)); err != nil {
		return nil, err
	}
	if !state.DeletedAt.IsZero() && state.DeletedAt.Before(state.ModifiedAt) {
		return nil, ErrTimeRegression
	}
	return &Article{id: state.ID, articleTypeID: state.ArticleTypeID, title: state.Title, slug: state.Slug, digest: state.Digest, content: state.Content, status: state.Status, tagIDs: tags, version: state.Version, createdAt: state.CreatedAt, modifiedAt: state.ModifiedAt, deletedAt: state.DeletedAt}, nil
}
