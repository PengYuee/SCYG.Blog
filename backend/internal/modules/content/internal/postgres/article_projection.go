package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
	"gorm.io/gorm"
)

type articleProjectionRow struct {
	ID                   int64      `gorm:"column:Id"`
	ArticleTypeID        int64      `gorm:"column:ArticleTypeId"`
	Title                string     `gorm:"column:Title"`
	Slug                 string     `gorm:"column:Slug"`
	Digest               string     `gorm:"column:Digest"`
	Content              string     `gorm:"column:Content"`
	Support              int64      `gorm:"column:Support"`
	Comment              int64      `gorm:"column:Comment"`
	Visited              int64      `gorm:"column:Visited"`
	Status               int16      `gorm:"column:Status"`
	Version              int64      `gorm:"column:Version"`
	CreationTime         time.Time  `gorm:"column:CreationTime"`
	LastModificationTime *time.Time `gorm:"column:LastModificationTime"`
}
type projectionTagRow struct {
	ArticleID int64 `gorm:"column:ArticleId"`
	TagID     int64 `gorm:"column:TagId"`
}

// ReadModel executes dedicated projection queries on a root read handle.
type ReadModel struct{ db *gorm.DB }

// NewReadModel constructs public, admin, and taxonomy projections.
func NewReadModel(db *gorm.DB) (*ReadModel, error) {
	if db == nil {
		return nil, errors.New("content read database is nil")
	}
	return &ReadModel{db: db}, nil
}
func (read *ReadModel) FindPublished(ctx context.Context, id domain.ArticleID) (application.ArticleView, error) {
	var row articleProjectionRow
	result := read.db.WithContext(ctx).Table(`"Article"`).Where(`"Id" = ? AND "Status" = 2 AND "IsDeleted" = false`, id.Int64()).Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return application.ArticleView{}, notFound("article")
	}
	if result.Error != nil {
		return application.ArticleView{}, translate(result.Error)
	}
	tags, err := read.tags(ctx, []int64{row.ID})
	if err != nil {
		return application.ArticleView{}, err
	}
	return projectionView(row, tags[row.ID])
}
func (read *ReadModel) ListPublished(ctx context.Context, filter application.ArticleFilter) (application.ArticlePage, error) {
	return read.list(ctx, filter, true)
}
func (read *ReadModel) ListAll(ctx context.Context, filter application.ArticleFilter) (application.ArticlePage, error) {
	return read.list(ctx, filter, false)
}
func (read *ReadModel) list(ctx context.Context, filter application.ArticleFilter, public bool) (application.ArticlePage, error) {
	page, size := filter.Page, filter.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	query := read.db.WithContext(ctx).Table(`"Article" AS a`).Where(`a."IsDeleted" = false`)
	if public {
		query = query.Where(`a."Status" = 2`)
	} else if filter.Status != "" {
		status, err := articleStatusToDB(filter.Status)
		if err != nil {
			return application.ArticlePage{}, err
		}
		query = query.Where(`a."Status" = ?`, status)
	}
	if filter.ArticleTypeID.Int64() > 0 {
		query = query.Where(`a."ArticleTypeId" = ?`, filter.ArticleTypeID.Int64())
	}
	if filter.TagID.Int64() > 0 {
		query = query.Where(`EXISTS (SELECT 1 FROM "TagArticle" ta WHERE ta."ArticleId" = a."Id" AND ta."TagId" = ?)`, filter.TagID.Int64())
	}
	if text := strings.TrimSpace(filter.Query); text != "" {
		query = query.Where(`(a."Title" ILIKE ? OR a."Digest" ILIKE ?)`, "%"+text+"%", "%"+text+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return application.ArticlePage{}, translate(err)
	}
	order, err := projectionOrder(filter.Sort)
	if err != nil {
		return application.ArticlePage{}, err
	}
	var rows []articleProjectionRow
	columns := `a."Id", a."ArticleTypeId", a."Title", a."Slug", a."Digest", a."Content", a."Status", a."Support", a."Comment", a."Visited", a."Version", a."CreationTime", a."LastModificationTime"`
	if err = query.Select(columns).Order(order).Limit(size).Offset((page - 1) * size).Scan(&rows).Error; err != nil {
		return application.ArticlePage{}, translate(err)
	}
	ids := make([]int64, len(rows))
	for index := range rows {
		ids[index] = rows[index].ID
	}
	tags, err := read.tags(ctx, ids)
	if err != nil {
		return application.ArticlePage{}, err
	}
	items := make([]application.ArticleView, 0, len(rows))
	for _, row := range rows {
		view, mapErr := projectionView(row, tags[row.ID])
		if mapErr != nil {
			return application.ArticlePage{}, mapErr
		}
		items = append(items, view)
	}
	pages := 0
	if total > 0 {
		pages = int((total + int64(size) - 1) / int64(size))
	}
	return application.ArticlePage{Items: items, Number: page, Size: size, TotalItems: total, TotalPages: pages}, nil
}
func projectionOrder(sort string) (string, error) {
	switch sort {
	case "", "newest":
		return `a."CreationTime" DESC, a."Id" DESC`, nil
	case "oldest":
		return `a."CreationTime" ASC, a."Id" ASC`, nil
	case "title":
		return `a."Title" ASC, a."Id" ASC`, nil
	case "title_desc":
		return `a."Title" DESC, a."Id" DESC`, nil
	default:
		return "", fmt.Errorf("sort: %w", domain.ErrInvalidValue)
	}
}
func (read *ReadModel) tags(ctx context.Context, ids []int64) (map[int64][]domain.TagID, error) {
	result := make(map[int64][]domain.TagID, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var rows []projectionTagRow
	if err := read.db.WithContext(ctx).Table(`"TagArticle"`).Select(`"ArticleId", "TagId"`).Where(`"ArticleId" IN ?`, ids).Order(`"ArticleId", "TagId"`).Scan(&rows).Error; err != nil {
		return nil, translate(err)
	}
	for _, row := range rows {
		id, err := domain.NewTagID(row.TagID)
		if err != nil {
			return nil, err
		}
		result[row.ArticleID] = append(result[row.ArticleID], id)
	}
	return result, nil
}
func projectionView(row articleProjectionRow, tags []domain.TagID) (application.ArticleView, error) {
	id, err := domain.NewArticleID(row.ID)
	if err != nil {
		return application.ArticleView{}, err
	}
	typeID, err := domain.NewArticleTypeID(row.ArticleTypeID)
	if err != nil {
		return application.ArticleView{}, err
	}
	title, err := domain.NewTitle(row.Title)
	if err != nil {
		return application.ArticleView{}, err
	}
	slug, err := domain.NewSlug(row.Slug)
	if err != nil {
		return application.ArticleView{}, err
	}
	digest, err := domain.NewDigest(row.Digest)
	if err != nil {
		return application.ArticleView{}, err
	}
	contentValue, err := domain.NewContent(row.Content)
	if err != nil {
		return application.ArticleView{}, err
	}
	status, err := articleStatusFromDB(row.Status)
	if err != nil {
		return application.ArticleView{}, err
	}
	version, err := domain.NewVersion(uint64(row.Version))
	if err != nil {
		return application.ArticleView{}, err
	}
	return application.ArticleView{ID: id, ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, Content: contentValue, Status: status, TagIDs: append([]domain.TagID(nil), tags...), Support: row.Support, Comment: row.Comment, Visited: row.Visited, Version: version, CreatedAt: row.CreationTime.UTC(), ModifiedAt: timeValue(row.LastModificationTime, row.CreationTime)}, nil
}

var _ application.ArticleReadModel = (*ReadModel)(nil)
var _ application.ArticleAdminReadModel = (*ReadModel)(nil)
