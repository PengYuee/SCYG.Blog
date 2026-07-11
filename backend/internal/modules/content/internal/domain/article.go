package domain

import (
	"fmt"
	"time"
)

// Clock supplies deterministic UTC domain time.
type Clock interface{ Now() time.Time }

// ArticleDraft contains parsed values needed to create an article draft.
type ArticleDraft struct {
	ID            ArticleID
	ArticleTypeID ArticleTypeID
	Title         Title
	Slug          Slug
	Digest        Digest
	Content       Content
	TagIDs        []TagID
}

// ArticleRevision contains parsed mutable article values.
type ArticleRevision struct {
	ArticleTypeID ArticleTypeID
	Title         Title
	Slug          Slug
	Digest        Digest
	Content       Content
	TagIDs        []TagID
}

// Article is the consistency boundary for content lifecycle and tag membership.
type Article struct {
	id            ArticleID
	articleTypeID ArticleTypeID
	title         Title
	slug          Slug
	digest        Digest
	content       Content
	status        Status
	tagIDs        []TagID
	version       Version
	createdAt     time.Time
	modifiedAt    time.Time
	deletedAt     time.Time
}

// NewArticle creates a version-one draft using the injected clock.
func NewArticle(draft ArticleDraft, clock Clock) (*Article, error) {
	tags, err := uniqueTags(draft.TagIDs)
	if err != nil {
		return nil, err
	}
	if draft.ArticleTypeID.Int64() == 0 {
		return nil, ErrArticleTypeRequired
	}
	now := clock.Now().UTC()
	return &Article{id: draft.ID, articleTypeID: draft.ArticleTypeID, title: draft.Title, slug: draft.Slug, digest: draft.Digest, content: draft.Content, status: StatusDraft, tagIDs: tags, version: initialVersion(), createdAt: now, modifiedAt: now}, nil
}

// Revise atomically updates editable values when expected version is current.
func (article *Article) Revise(expected Version, revision ArticleRevision, clock Clock) error {
	if err := article.mutable(expected); err != nil {
		return err
	}
	if revision.ArticleTypeID.Int64() == 0 {
		return ErrArticleTypeRequired
	}
	tags, err := uniqueTags(revision.TagIDs)
	if err != nil {
		return err
	}
	article.articleTypeID = revision.ArticleTypeID
	article.title = revision.Title
	article.slug = revision.Slug
	article.digest = revision.Digest
	article.content = revision.Content
	article.tagIDs = tags
	article.touch(clock)
	return nil
}

// Publish transitions a complete draft to published.
func (article *Article) Publish(expected Version, clock Clock) error {
	if err := article.current(expected); err != nil {
		return err
	}
	if article.status != StatusDraft {
		return fmt.Errorf("publish %s: %w", article.status, ErrInvalidTransition)
	}
	if article.articleTypeID.Int64() == 0 {
		return ErrArticleTypeRequired
	}
	if article.content.String() == "" {
		return ErrContentRequired
	}
	article.status = StatusPublished
	article.touch(clock)
	return nil
}

// Archive transitions a published article to archived.
func (article *Article) Archive(expected Version, clock Clock) error {
	if err := article.current(expected); err != nil {
		return err
	}
	if article.status != StatusPublished {
		return fmt.Errorf("archive %s: %w", article.status, ErrInvalidTransition)
	}
	article.status = StatusArchived
	article.touch(clock)
	return nil
}

// Delete soft-deletes the article at the supplied domain time.
func (article *Article) Delete(expected Version, clock Clock) error {
	if err := article.current(expected); err != nil {
		return err
	}
	article.deletedAt = clock.Now().UTC()
	article.version = article.version.next()
	return nil
}
func (article *Article) mutable(expected Version) error {
	if err := article.current(expected); err != nil {
		return err
	}
	if article.status == StatusArchived {
		return fmt.Errorf("revise archived: %w", ErrInvalidTransition)
	}
	return nil
}
func (article *Article) current(expected Version) error {
	if article.version != expected {
		return &VersionConflict{Expected: expected, Actual: article.version}
	}
	return nil
}
func (article *Article) touch(clock Clock) {
	article.modifiedAt = clock.Now().UTC()
	article.version = article.version.next()
}
func uniqueTags(input []TagID) ([]TagID, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("tags: %w", ErrInvalidValue)
	}
	seen := make(map[TagID]struct{}, len(input))
	result := make([]TagID, 0, len(input))
	for _, id := range input {
		if id.Int64() == 0 {
			return nil, invalid("tag_id")
		}
		if _, exists := seen[id]; exists {
			return nil, ErrDuplicateTag
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}
func (article *Article) ID() ArticleID                { return article.id }
func (article *Article) ArticleTypeID() ArticleTypeID { return article.articleTypeID }
func (article *Article) Title() Title                 { return article.title }
func (article *Article) Slug() Slug                   { return article.slug }
func (article *Article) Digest() Digest               { return article.digest }
func (article *Article) Content() Content             { return article.content }
func (article *Article) Status() Status               { return article.status }
func (article *Article) TagIDs() []TagID              { return append([]TagID(nil), article.tagIDs...) }
func (article *Article) Version() Version             { return article.version }
func (article *Article) CreatedAt() time.Time         { return article.createdAt }
func (article *Article) ModifiedAt() time.Time        { return article.modifiedAt }
func (article *Article) DeletedAt() time.Time         { return article.deletedAt }
