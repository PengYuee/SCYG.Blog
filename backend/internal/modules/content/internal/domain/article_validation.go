package domain

import "fmt"

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
