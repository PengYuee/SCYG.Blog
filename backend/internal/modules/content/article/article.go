package article

import (
	"fmt"
	"slices"
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

// NewArticle 使用注入时钟创建版本为一的文章。
func NewArticle(draft ArticleDraft, status Status, clock Clock) (*Article, error) {
	switch status {
	case StatusDraft, StatusPublished:
	case StatusArchived:
		return nil, fmt.Errorf("文章创建状态 %q 不合法：%w", status, ErrInvalidValue)
	default:
		return nil, fmt.Errorf("文章创建状态 %q 不合法：%w", status, ErrInvalidValue)
	}
	if err := validateDraft(draft); err != nil {
		return nil, err
	}
	tags, err := uniqueTags(draft.TagIDs)
	if err != nil {
		return nil, err
	}
	now, err := clockTime(clock, time.Time{})
	if err != nil {
		return nil, err
	}
	return &Article{id: draft.ID, articleTypeID: draft.ArticleTypeID, title: draft.Title, slug: draft.Slug, digest: draft.Digest, content: draft.Content, status: status, tagIDs: tags, version: initialVersion(), createdAt: now, modifiedAt: now}, nil
}

// Revise atomically updates editable values when expected version is current.
func (article *Article) Revise(expected Version, revision ArticleRevision, clock Clock) error {
	if err := article.changeable(expected); err != nil {
		return err
	}
	if err := validateRevision(revision); err != nil {
		return err
	}
	tags, err := uniqueTags(revision.TagIDs)
	if err != nil {
		return err
	}
	if article.sameRevision(revision, tags) {
		return ErrNoChange
	}
	next, err := article.version.next()
	if err != nil {
		return err
	}
	now, err := clockTime(clock, article.modifiedAt)
	if err != nil {
		return err
	}
	article.articleTypeID, article.title, article.slug = revision.ArticleTypeID, revision.Title, revision.Slug
	article.digest, article.content, article.tagIDs = revision.Digest, revision.Content, tags
	article.modifiedAt, article.version = now, next
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
	next, err := article.version.next()
	if err != nil {
		return err
	}
	now, err := clockTime(clock, article.modifiedAt)
	if err != nil {
		return err
	}
	article.status, article.modifiedAt, article.version = StatusPublished, now, next
	return nil
}

// Archive transitions a published article to a terminal archived state.
func (article *Article) Archive(expected Version, clock Clock) error {
	if err := article.current(expected); err != nil {
		return err
	}
	if article.status != StatusPublished {
		return fmt.Errorf("archive %s: %w", article.status, ErrInvalidTransition)
	}
	next, err := article.version.next()
	if err != nil {
		return err
	}
	now, err := clockTime(clock, article.modifiedAt)
	if err != nil {
		return err
	}
	article.status, article.modifiedAt, article.version = StatusArchived, now, next
	return nil
}

// Delete soft-deletes a non-archived article at monotonic domain time.
func (article *Article) Delete(expected Version, clock Clock) error {
	if err := article.current(expected); err != nil {
		return err
	}
	if article.status == StatusArchived {
		return fmt.Errorf("delete archived: %w", ErrInvalidTransition)
	}
	next, err := article.version.next()
	if err != nil {
		return err
	}
	now, err := clockTime(clock, article.modifiedAt)
	if err != nil {
		return err
	}
	article.deletedAt, article.modifiedAt, article.version = now, now, next
	return nil
}

func (article *Article) current(expected Version) error {
	if !expected.valid() || article.version != expected {
		return &VersionConflict{Expected: expected, Actual: article.version}
	}
	if !article.deletedAt.IsZero() {
		return ErrDeleted
	}
	return nil
}

func (article *Article) changeable(expected Version) error {
	if err := article.current(expected); err != nil {
		return err
	}
	if article.status == StatusArchived {
		return fmt.Errorf("revise archived: %w", ErrInvalidTransition)
	}
	return nil
}

func (article *Article) sameRevision(revision ArticleRevision, tags []TagID) bool {
	return article.articleTypeID == revision.ArticleTypeID && article.title == revision.Title && article.slug == revision.Slug && article.digest == revision.Digest && article.content == revision.Content && slices.Equal(article.tagIDs, tags)
}

// ID returns the article identifier.
func (article *Article) ID() ArticleID { return article.id }

// ArticleTypeID returns the associated article type identifier.
func (article *Article) ArticleTypeID() ArticleTypeID { return article.articleTypeID }

// Title returns the article title.
func (article *Article) Title() Title { return article.title }

// Slug returns the URL slug.
func (article *Article) Slug() Slug { return article.slug }

// Digest returns the article summary.
func (article *Article) Digest() Digest { return article.digest }

// Content returns the article body.
func (article *Article) Content() Content { return article.content }

// Status returns the current lifecycle status.
func (article *Article) Status() Status { return article.status }

// TagIDs returns a copy of the article tag identifiers.
func (article *Article) TagIDs() []TagID { return append([]TagID(nil), article.tagIDs...) }

// Version returns the optimistic-concurrency version.
func (article *Article) Version() Version { return article.version }

// CreatedAt returns the creation timestamp.
func (article *Article) CreatedAt() time.Time { return article.createdAt }

// ModifiedAt returns the last modification timestamp.
func (article *Article) ModifiedAt() time.Time { return article.modifiedAt }

// DeletedAt returns the soft-deletion timestamp.
func (article *Article) DeletedAt() time.Time { return article.deletedAt }
