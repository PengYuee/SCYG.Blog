package content

import (
	"strings"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func parseDraft(command CreateArticle) (domain.ArticleDraft, error) {
	typeID, err := domain.NewArticleTypeID(command.ArticleTypeID)
	if err != nil {
		return domain.ArticleDraft{}, err
	}
	title, err := domain.NewTitle(command.Title)
	if err != nil {
		return domain.ArticleDraft{}, err
	}
	slug, err := domain.NewSlug(command.Slug)
	if err != nil {
		return domain.ArticleDraft{}, err
	}
	digest, err := domain.NewDigest(command.Digest)
	if err != nil {
		return domain.ArticleDraft{}, err
	}
	body, err := domain.NewContent(command.Content)
	if err != nil {
		return domain.ArticleDraft{}, err
	}
	tags, err := parseTags(command.TagIDs)
	if err != nil {
		return domain.ArticleDraft{}, err
	}
	return domain.ArticleDraft{ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, Content: body, TagIDs: tags}, nil
}

func parseRevision(command ReviseArticle) (domain.ArticleID, domain.Version, domain.ArticleRevision, error) {
	id, version, err := parseIdentity(command.ID, command.Version)
	if err != nil {
		return domain.ArticleID{}, domain.Version{}, domain.ArticleRevision{}, err
	}
	draft, err := parseDraft(CreateArticle{ArticleTypeID: command.ArticleTypeID, Title: command.Title, Slug: command.Slug, Digest: command.Digest, Content: command.Content, TagIDs: command.TagIDs})
	if err != nil {
		return domain.ArticleID{}, domain.Version{}, domain.ArticleRevision{}, err
	}
	return id, version, domain.ArticleRevision{ArticleTypeID: draft.ArticleTypeID, Title: draft.Title, Slug: draft.Slug, Digest: draft.Digest, Content: draft.Content, TagIDs: draft.TagIDs}, nil
}

func parseIdentity(rawID int64, rawVersion uint64) (domain.ArticleID, domain.Version, error) {
	id, err := domain.NewArticleID(rawID)
	if err != nil {
		return domain.ArticleID{}, domain.Version{}, err
	}
	version, err := domain.NewVersion(rawVersion)
	if err != nil {
		return domain.ArticleID{}, domain.Version{}, err
	}
	return id, version, nil
}

func parseTags(raw []int64) ([]domain.TagID, error) {
	tags := make([]domain.TagID, len(raw))
	for index, value := range raw {
		parsed, err := domain.NewTagID(value)
		if err != nil {
			return nil, err
		}
		tags[index] = parsed
	}
	return tags, nil
}

func parseFilter(query ListArticles) (application.ArticleFilter, error) {
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 100 {
		return application.ArticleFilter{}, invalidCommand("page")
	}
	filter := application.ArticleFilter{Page: query.Page, PageSize: query.PageSize, Query: strings.TrimSpace(query.Query), Sort: query.Sort}
	var err error
	if query.ArticleTypeID != 0 {
		filter.ArticleTypeID, err = domain.NewArticleTypeID(query.ArticleTypeID)
		if err != nil {
			return application.ArticleFilter{}, err
		}
	}
	if query.TagID != 0 {
		filter.TagID, err = domain.NewTagID(query.TagID)
		if err != nil {
			return application.ArticleFilter{}, err
		}
	}
	switch query.Sort {
	case "", "newest", "oldest", "title", "title_desc":
	default:
		return application.ArticleFilter{}, invalidCommand("sort")
	}
	return filter, nil
}
