package domain

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

// NewArticle creates a valid version-one draft using the injected clock.
func NewArticle(draft ArticleDraft, clock Clock) (*Article, error) {
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
	return &Article{id: draft.ID, articleTypeID: draft.ArticleTypeID, title: draft.Title, slug: draft.Slug, digest: draft.Digest, content: draft.Content, status: StatusDraft, tagIDs: tags, version: initialVersion(), createdAt: now, modifiedAt: now}, nil
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

func validateDraft(draft ArticleDraft) error {
	if !draft.ID.valid() {
		return invalid("article_id")
	}
	if !draft.ArticleTypeID.valid() {
		return ErrArticleTypeRequired
	}
	return validateText(draft.Title, draft.Slug, draft.Digest, draft.Content)
}
func validateRevision(revision ArticleRevision) error {
	if !revision.ArticleTypeID.valid() {
		return ErrArticleTypeRequired
	}
	return validateText(revision.Title, revision.Slug, revision.Digest, revision.Content)
}
func validateText(title Title, slug Slug, digest Digest, content Content) error {
	switch {
	case !title.valid():
		return invalid("title")
	case !slug.valid():
		return invalid("slug")
	case !digest.valid():
		return invalid("digest")
	case !content.valid():
		return ErrContentRequired
	default:
		return nil
	}
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
func uniqueTags(input []TagID) ([]TagID, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("tags: %w", ErrInvalidValue)
	}
	seen := make(map[TagID]struct{}, len(input))
	result := make([]TagID, 0, len(input))
	for _, id := range input {
		if !id.valid() {
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
