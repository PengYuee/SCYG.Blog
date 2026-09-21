package article

import (
	"fmt"
	"math"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/persistence"
)

func statusToDB(status Status) (int16, error) {
	switch status {
	case StatusDraft:
		return 1, nil
	case StatusPublished:
		return 2, nil
	case StatusArchived:
		return 3, nil
	default:
		return 0, fmt.Errorf("map article status: %w", ErrInvalidValue)
	}
}

func statusFromDB(status int16) (Status, error) {
	switch status {
	case 1:
		return StatusDraft, nil
	case 2:
		return StatusPublished, nil
	case 3:
		return StatusArchived, nil
	default:
		return "", fmt.Errorf("map article status %d: %w", status, ErrInvalidValue)
	}
}

func articleToModel(value *Article) (articleModel, error) {
	status, err := statusToDB(value.Status())
	if err != nil {
		return articleModel{}, err
	}
	version, err := versionToDB(value.Version().Uint64())
	if err != nil {
		return articleModel{}, err
	}
	return articleModel{ID: value.ID().Int64(), ArticleTypeID: value.ArticleTypeID().Int64(), Title: value.Title().String(), Slug: value.Slug().String(), Digest: value.Digest().String(), Content: value.Content().String(), Status: status, Version: version, AuditFields: persistence.NewAuditFields(value.CreatedAt(), value.ModifiedAt(), value.DeletedAt())}, nil
}

func versionToDB(value uint64) (int64, error) {
	if value == 0 || value > math.MaxInt64 {
		return 0, invalid("version")
	}
	return int64(value), nil
}

func versionFromDB(value int64) (Version, error) {
	if value <= 0 {
		return Version{}, invalid("version")
	}
	return NewVersion(uint64(value))
}

func articleFromModel(row articleModel, tags []tagArticleModel) (*Article, error) {
	if err := row.Validate(); err != nil {
		return nil, fmt.Errorf("map article audit fields: %w", err)
	}
	id, err := NewArticleID(row.ID)
	if err != nil {
		return nil, err
	}
	typeID, err := NewArticleTypeID(row.ArticleTypeID)
	if err != nil {
		return nil, err
	}
	title, err := NewTitle(row.Title)
	if err != nil {
		return nil, err
	}
	slug, err := NewSlug(row.Slug)
	if err != nil {
		return nil, err
	}
	digest, err := NewDigest(row.Digest)
	if err != nil {
		return nil, err
	}
	body, err := NewContent(row.Content)
	if err != nil {
		return nil, err
	}
	status, err := statusFromDB(row.Status)
	if err != nil {
		return nil, err
	}
	version, err := versionFromDB(row.Version)
	if err != nil {
		return nil, err
	}
	tagIDs := make([]TagID, 0, len(tags))
	for _, link := range tags {
		tagID, parseErr := NewTagID(link.TagID)
		if parseErr != nil {
			return nil, parseErr
		}
		tagIDs = append(tagIDs, tagID)
	}
	return ReconstituteArticle(ArticleState{ID: id, ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, Content: body, Status: status, TagIDs: tagIDs, Version: version, CreatedAt: row.CreatedAt.UTC(), ModifiedAt: persistence.EffectiveUpdatedAt(row.CreatedAt, row.UpdatedAt), DeletedAt: timeValue(row.DeletedAt, time.Time{})})
}

func timeValue(value *time.Time, fallback time.Time) time.Time {
	if value == nil {
		return fallback
	}
	return value.UTC()
}

func result(value *Article) Result {
	tags := value.TagIDs()
	ids := make([]int64, len(tags))
	for i, id := range tags {
		ids[i] = id.Int64()
	}
	return Result{ID: value.ID().Int64(), ArticleTypeID: value.ArticleTypeID().Int64(), Title: value.Title().String(), Slug: value.Slug().String(), Digest: value.Digest().String(), Content: value.Content().String(), Status: string(value.Status()), TagIDs: ids, Version: value.Version().Uint64(), CreatedAt: value.CreatedAt(), ModifiedAt: value.ModifiedAt()}
}

func projectionResult(row projectionRow, tags []TagID) (Result, error) {
	id, err := NewArticleID(row.ID)
	if err != nil {
		return Result{}, err
	}
	typeID, err := NewArticleTypeID(row.ArticleTypeID)
	if err != nil {
		return Result{}, err
	}
	title, err := NewTitle(row.Title)
	if err != nil {
		return Result{}, err
	}
	slug, err := NewSlug(row.Slug)
	if err != nil {
		return Result{}, err
	}
	digest, err := NewDigest(row.Digest)
	if err != nil {
		return Result{}, err
	}
	body, err := NewContent(row.Content)
	if err != nil {
		return Result{}, err
	}
	status, err := statusFromDB(row.Status)
	if err != nil {
		return Result{}, err
	}
	version, err := versionFromDB(row.Version)
	if err != nil {
		return Result{}, err
	}
	ids := make([]int64, len(tags))
	for i, v := range tags {
		ids[i] = v.Int64()
	}
	return Result{ID: id.Int64(), ArticleTypeID: typeID.Int64(), Title: title.String(), Slug: slug.String(), Digest: digest.String(), Content: body.String(), Status: string(status), TagIDs: ids, Support: row.Support, Comment: row.Comment, Visited: row.Visited, Version: version.Uint64(), CreatedAt: row.CreatedAt.UTC(), ModifiedAt: persistence.EffectiveUpdatedAt(row.CreatedAt, row.UpdatedAt)}, nil
}
