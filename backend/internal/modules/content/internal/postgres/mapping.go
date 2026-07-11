package postgres

import (
	"fmt"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func articleStatusToDB(status domain.Status) (int16, error) {
	switch status {
	case domain.StatusDraft:
		return 1, nil
	case domain.StatusPublished:
		return 2, nil
	case domain.StatusArchived:
		return 3, nil
	default:
		return 0, fmt.Errorf("map article status: %w", domain.ErrInvalidValue)
	}
}
func articleStatusFromDB(status int16) (domain.Status, error) {
	switch status {
	case 1:
		return domain.StatusDraft, nil
	case 2:
		return domain.StatusPublished, nil
	case 3:
		return domain.StatusArchived, nil
	default:
		return "", fmt.Errorf("map article status %d: %w", status, domain.ErrInvalidValue)
	}
}
func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copied := value.UTC()
	return &copied
}
func timeValue(value *time.Time, fallback time.Time) time.Time {
	if value == nil {
		return fallback
	}
	return value.UTC()
}

func articleToModel(article *domain.Article) (articleModel, error) {
	status, err := articleStatusToDB(article.Status())
	if err != nil {
		return articleModel{}, err
	}
	return articleModel{ID: article.ID().Int64(), ArticleTypeID: article.ArticleTypeID().Int64(), Title: article.Title().String(), Slug: article.Slug().String(), Digest: article.Digest().String(), Content: article.Content().String(), Status: status, Version: int64(article.Version().Uint64()), CreationTime: article.CreatedAt().UTC(), LastModificationTime: nullableTime(article.ModifiedAt()), DeletionTime: nullableTime(article.DeletedAt()), IsDeleted: !article.DeletedAt().IsZero()}, nil
}
func articleFromModel(row articleModel, tags []tagArticleModel) (*domain.Article, error) {
	id, err := domain.NewArticleID(row.ID)
	if err != nil {
		return nil, err
	}
	typeID, err := domain.NewArticleTypeID(row.ArticleTypeID)
	if err != nil {
		return nil, err
	}
	title, err := domain.NewTitle(row.Title)
	if err != nil {
		return nil, err
	}
	slug, err := domain.NewSlug(row.Slug)
	if err != nil {
		return nil, err
	}
	digest, err := domain.NewDigest(row.Digest)
	if err != nil {
		return nil, err
	}
	body, err := domain.NewContent(row.Content)
	if err != nil {
		return nil, err
	}
	status, err := articleStatusFromDB(row.Status)
	if err != nil {
		return nil, err
	}
	version, err := domain.NewVersion(uint64(row.Version))
	if err != nil {
		return nil, err
	}
	tagIDs := make([]domain.TagID, 0, len(tags))
	for _, link := range tags {
		tagID, parseErr := domain.NewTagID(link.TagID)
		if parseErr != nil {
			return nil, parseErr
		}
		tagIDs = append(tagIDs, tagID)
	}
	return domain.ReconstituteArticle(domain.ArticleState{ID: id, ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, Content: body, Status: status, TagIDs: tagIDs, Version: version, CreatedAt: row.CreationTime.UTC(), ModifiedAt: timeValue(row.LastModificationTime, row.CreationTime), DeletedAt: timeValue(row.DeletionTime, time.Time{})})
}
func articleTypeFromModel(row articleTypeModel) (*domain.ArticleType, error) {
	id, err := domain.NewArticleTypeID(row.ID)
	if err != nil {
		return nil, err
	}
	name, err := domain.NewName(row.Name)
	if err != nil {
		return nil, err
	}
	version, err := domain.NewVersion(uint64(row.Version))
	if err != nil {
		return nil, err
	}
	return domain.ReconstituteArticleType(domain.ArticleTypeState{TaxonomyState: domain.TaxonomyState[domain.ArticleTypeID]{ID: id, Name: name, Version: version, CreatedAt: row.CreationTime.UTC(), ModifiedAt: timeValue(row.LastModificationTime, row.CreationTime), DeletedAt: timeValue(row.DeletionTime, time.Time{})}, Image: row.Image, Meun: int32(row.Meun)})
}
func tagFromModel(row tagModel) (*domain.Tag, error) {
	id, err := domain.NewTagID(row.ID)
	if err != nil {
		return nil, err
	}
	name, err := domain.NewName(row.Name)
	if err != nil {
		return nil, err
	}
	version, err := domain.NewVersion(uint64(row.Version))
	if err != nil {
		return nil, err
	}
	return domain.ReconstituteTag(domain.TaxonomyState[domain.TagID]{ID: id, Name: name, Version: version, CreatedAt: row.CreationTime.UTC(), ModifiedAt: timeValue(row.LastModificationTime, row.CreationTime), DeletedAt: timeValue(row.DeletionTime, time.Time{})})
}
