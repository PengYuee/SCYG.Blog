package article

import (
	"fmt"
)

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

func parseCreationStatus(value ArticleCreationStatus) (Status, error) {
	switch value {
	case ArticleCreationStatusDraft:
		return StatusDraft, nil
	case ArticleCreationStatusPublished:
		return StatusPublished, nil
	default:
		return "", fmt.Errorf("文章创建状态 %d 不合法：%w", value, ErrInvalidValue)
	}
}

func parseCreate(input Create) (ArticleDraft, error) {
	typeID, err := NewArticleTypeID(input.ArticleTypeID)
	if err != nil {
		return ArticleDraft{}, err
	}
	title, err := NewTitle(input.Title)
	if err != nil {
		return ArticleDraft{}, err
	}
	slug, err := NewSlug(input.Slug)
	if err != nil {
		return ArticleDraft{}, err
	}
	digest, err := NewDigest(input.Digest)
	if err != nil {
		return ArticleDraft{}, err
	}
	body, err := NewContent(input.Content)
	if err != nil {
		return ArticleDraft{}, err
	}
	tags, err := parseTags(input.TagIDs)
	if err != nil {
		return ArticleDraft{}, err
	}
	return ArticleDraft{ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, Content: body, TagIDs: tags}, nil
}

func parseTags(raw []int64) ([]TagID, error) {
	tags := make([]TagID, len(raw))
	for i, value := range raw {
		id, err := NewTagID(value)
		if err != nil {
			return nil, err
		}
		tags[i] = id
	}
	return tags, nil
}

func parseIdentity(idRaw int64, versionRaw uint64) (ArticleID, Version, error) {
	id, err := NewArticleID(idRaw)
	if err != nil {
		return ArticleID{}, Version{}, err
	}
	version, err := NewVersion(versionRaw)
	if err != nil {
		return ArticleID{}, Version{}, err
	}
	return id, version, nil
}
