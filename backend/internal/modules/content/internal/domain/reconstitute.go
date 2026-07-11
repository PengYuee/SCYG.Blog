package domain

import "time"

// ArticleState contains trusted persisted state used only by internal adapters.
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

// ReconstituteArticle restores a persisted aggregate through domain invariants.
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

// TaxonomyState contains trusted persisted taxonomy state used by internal adapters.
type TaxonomyState[ID ArticleTypeID | TagID] struct {
	ID         ID
	Name       Name
	Version    Version
	CreatedAt  time.Time
	ModifiedAt time.Time
	DeletedAt  time.Time
}

// ReconstituteArticleType restores a persisted article type through domain invariants.
func ReconstituteArticleType(state TaxonomyState[ArticleTypeID]) (*ArticleType, error) {
	if !state.ID.valid() || !state.Name.valid() || !state.Version.valid() || state.CreatedAt.IsZero() || state.ModifiedAt.Before(state.CreatedAt) {
		return nil, ErrInvalidValue
	}
	return &ArticleType{id: state.ID, name: state.Name, version: state.Version, createdAt: state.CreatedAt, modifiedAt: state.ModifiedAt, deletedAt: state.DeletedAt}, nil
}

// ReconstituteTag restores a persisted tag through domain invariants.
func ReconstituteTag(state TaxonomyState[TagID]) (*Tag, error) {
	if !state.ID.valid() || !state.Name.valid() || !state.Version.valid() || state.CreatedAt.IsZero() || state.ModifiedAt.Before(state.CreatedAt) {
		return nil, ErrInvalidValue
	}
	return &Tag{id: state.ID, name: state.Name, version: state.Version, createdAt: state.CreatedAt, modifiedAt: state.ModifiedAt, deletedAt: state.DeletedAt}, nil
}
