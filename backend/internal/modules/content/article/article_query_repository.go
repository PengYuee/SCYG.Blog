package article

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// Query reads article projections without exposing persistence records.
type Query struct{ db *gorm.DB }

// NewQuery constructs an article projection query.
func NewQuery(db *gorm.DB) (*Query, error) {
	if db == nil {
		return nil, errors.New("article query database is nil")
	}
	return &Query{db: db}, nil
}

// Get reads one published article.
func (query *Query) Get(ctx context.Context, input Get) (Result, error) {
	return query.get(ctx, input, true)
}

// GetManage reads one non-deleted article for authorized management clients.
func (query *Query) GetManage(ctx context.Context, input Get) (Result, error) {
	return query.get(ctx, input, false)
}

func (query *Query) get(ctx context.Context, input Get, publishedOnly bool) (Result, error) {
	id, err := NewArticleID(input.ID)
	if err != nil {
		return Result{}, validation(err)
	}
	var row projectionRow
	// The correlated aggregate shares the article statement's MVCC snapshot and
	// returns the content only once, regardless of how many tags are attached.
	statement := query.db.WithContext(ctx).Table("articles AS a").Select("a.*, COALESCE((SELECT json_agg(at.tag_id ORDER BY at.tag_id) FROM article_tags AS at WHERE at.article_id = a.id), '[]'::json)::text AS tag_ids_json").Where("a.id = ? AND a.is_deleted = false", id.Int64())
	if publishedOnly {
		statement = statement.Where("a.status = 2")
	}
	r := statement.Take(&row)
	if errors.Is(r.Error, gorm.ErrRecordNotFound) {
		return Result{}, notFound()
	}
	if r.Error != nil {
		return Result{}, translate(r.Error)
	}
	var ids []int64
	if err := json.Unmarshal([]byte(row.TagIDsJSON), &ids); err != nil {
		return Result{}, translate(err)
	}
	tags := make([]TagID, len(ids))
	for index, value := range ids {
		tags[index], err = NewTagID(value)
		if err != nil {
			return Result{}, translate(err)
		}
	}
	return projectionResult(row, tags)
}

// ListManage reads non-deleted articles for authorized management clients.
func (query *Query) ListManage(ctx context.Context, input List) (Page, error) {
	page, size := input.Page, input.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	q := query.db.WithContext(ctx).Table("articles AS a").Where("a.is_deleted = false")
	if input.Status != "" {
		status, err := statusToDB(Status(input.Status))
		if err != nil {
			return Page{}, validation(err)
		}
		q = q.Where("a.status = ?", status)
	}
	if input.ArticleTypeID > 0 {
		q = q.Where("a.article_type_id = ?", input.ArticleTypeID)
	}
	if input.TagID > 0 {
		q = q.Where("EXISTS (SELECT 1 FROM article_tags at WHERE at.article_id = a.id AND at.tag_id = ?)", input.TagID)
	}
	if text := strings.TrimSpace(input.Query); text != "" {
		q = q.Where("(a.title ILIKE ? OR a.digest ILIKE ?)", "%"+text+"%", "%"+text+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return Page{}, translate(err)
	}
	order, err := projectionOrder(input.Sort)
	if err != nil {
		return Page{}, validation(err)
	}
	var rows []projectionRow
	columns := "a.id, a.article_type_id, a.title, a.slug, a.digest, a.content, a.status, a.support, a.comment, a.visited, a.version, a.created_at, a.updated_at"
	if err = q.Select(columns).Order(order).Limit(size).Offset((page - 1) * size).Scan(&rows).Error; err != nil {
		return Page{}, translate(err)
	}
	ids := make([]int64, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	tags, err := query.tags(ctx, ids)
	if err != nil {
		return Page{}, err
	}
	items := make([]Result, 0, len(rows))
	for _, row := range rows {
		value, err := projectionResult(row, tags[row.ID])
		if err != nil {
			return Page{}, err
		}
		items = append(items, value)
	}
	pages := 0
	if total > 0 {
		pages = int((total + int64(size) - 1) / int64(size))
	}
	return Page{Items: items, Number: page, Size: size, TotalItems: total, TotalPages: pages}, nil
}

// List reads published articles with filtering and pagination.
func (query *Query) List(ctx context.Context, input List) (Page, error) {
	page, size := input.Page, input.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	q := query.db.WithContext(ctx).Table("articles AS a").Where("a.is_deleted = false AND a.status = 2")
	if input.ArticleTypeID > 0 {
		q = q.Where("a.article_type_id = ?", input.ArticleTypeID)
	}
	if input.TagID > 0 {
		q = q.Where("EXISTS (SELECT 1 FROM article_tags at WHERE at.article_id = a.id AND at.tag_id = ?)", input.TagID)
	}
	if text := strings.TrimSpace(input.Query); text != "" {
		q = q.Where("(a.title ILIKE ? OR a.digest ILIKE ?)", "%"+text+"%", "%"+text+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return Page{}, translate(err)
	}
	order, err := projectionOrder(input.Sort)
	if err != nil {
		return Page{}, validation(err)
	}
	var rows []projectionRow
	columns := "a.id, a.article_type_id, a.title, a.slug, a.digest, a.content, a.status, a.support, a.comment, a.visited, a.version, a.created_at, a.updated_at"
	if err = q.Select(columns).Order(order).Limit(size).Offset((page - 1) * size).Scan(&rows).Error; err != nil {
		return Page{}, translate(err)
	}
	ids := make([]int64, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	tags, err := query.tags(ctx, ids)
	if err != nil {
		return Page{}, err
	}
	items := make([]Result, 0, len(rows))
	for _, row := range rows {
		value, err := projectionResult(row, tags[row.ID])
		if err != nil {
			return Page{}, err
		}
		items = append(items, value)
	}
	pages := 0
	if total > 0 {
		pages = int((total + int64(size) - 1) / int64(size))
	}
	return Page{Items: items, Number: page, Size: size, TotalItems: total, TotalPages: pages}, nil
}

func projectionOrder(sort string) (string, error) {
	switch sort {
	case "", "-createdAt":
		return "a.created_at DESC, a.id DESC", nil
	case "createdAt":
		return "a.created_at ASC, a.id ASC", nil
	case "updatedAt":
		return "COALESCE(a.updated_at, a.created_at) ASC, a.id ASC", nil
	case "-updatedAt":
		return "COALESCE(a.updated_at, a.created_at) DESC, a.id DESC", nil
	case "title":
		return "a.title ASC, a.id ASC", nil
	case "-title":
		return "a.title DESC, a.id DESC", nil
	default:
		return "", fmt.Errorf("sort: %w", ErrInvalidValue)
	}
}

func (query *Query) tags(ctx context.Context, ids []int64) (map[int64][]TagID, error) {
	result := make(map[int64][]TagID, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var rows []struct {
		ArticleID int64 `gorm:"column:article_id"`
		TagID     int64 `gorm:"column:tag_id"`
	}
	if err := query.db.WithContext(ctx).Table("article_tags").Select("article_id, tag_id").Where("article_id IN ?", ids).Order("article_id, tag_id").Scan(&rows).Error; err != nil {
		return nil, translate(err)
	}
	for _, row := range rows {
		id, err := NewTagID(row.TagID)
		if err != nil {
			return nil, err
		}
		result[row.ArticleID] = append(result[row.ArticleID], id)
	}
	return result, nil
}
