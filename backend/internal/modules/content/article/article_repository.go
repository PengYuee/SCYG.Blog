package article

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository persists article aggregates and their tag links.
type Repository struct{ db *gorm.DB }

// NewRepository constructs an article repository.
func NewRepository(db *gorm.DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("article database is nil")
	}
	return &Repository{db: db}, nil
}

// NextID allocates the next article identifier.
func (repo *Repository) NextID(ctx context.Context) (ArticleID, error) {
	var value int64
	if err := repo.db.WithContext(ctx).Raw(`SELECT nextval(pg_get_serial_sequence('articles', 'id'))`).Scan(&value).Error; err != nil {
		return ArticleID{}, translate(err)
	}
	return NewArticleID(value)
}

// Find loads one non-deleted article aggregate.
func (repo *Repository) Find(ctx context.Context, id ArticleID) (*Article, error) {
	var row articleModel
	r := repo.db.WithContext(ctx).Where("id = ? AND is_deleted = false", id.Int64()).First(&row)
	if errors.Is(r.Error, gorm.ErrRecordNotFound) {
		return nil, notFound()
	}
	if r.Error != nil {
		return nil, translate(r.Error)
	}
	var links []tagArticleModel
	if err := repo.db.WithContext(ctx).Where("article_id = ?", id.Int64()).Order("tag_id ASC").Find(&links).Error; err != nil {
		return nil, translate(err)
	}
	value, err := articleFromModel(row, links)
	if err != nil {
		return nil, fmt.Errorf("map article %d: %w", id.Int64(), err)
	}
	return value, nil
}

// Save persists an article and synchronizes its tag links.
func (repo *Repository) Save(ctx context.Context, value *Article) error {
	row, err := articleToModel(value)
	if err != nil {
		return err
	}
	articleTypeID, err := NewArticleTypeID(row.ArticleTypeID)
	if err != nil {
		return err
	}
	if err := repo.lockArticleTypes(ctx, value.ArticleTypeID(), articleTypeID); err != nil {
		return err
	}
	if err := repo.lockTags(ctx, value.TagIDs()); err != nil {
		return err
	}
	if row.Version == 1 {
		return repo.create(ctx, row, value.TagIDs())
	}
	return repo.update(ctx, row, value.TagIDs())
}

func (repo *Repository) lockArticleTypes(ctx context.Context, ids ...ArticleTypeID) error {
	values := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if !id.valid() {
			continue
		}
		value := id.Int64()
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return nil
	}
	sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
	var rows []struct {
		ID int64 `gorm:"column:id"`
	}
	result := repo.db.WithContext(ctx).Table("article_types").Select("id").Where("id IN ? AND is_deleted = false", values).Order("id ASC").Clauses(clause.Locking{Strength: "SHARE"}).Find(&rows)
	if result.Error != nil {
		return translate(result.Error)
	}
	if len(rows) != len(values) {
		return precondition()
	}
	return nil
}

func (repo *Repository) create(ctx context.Context, row articleModel, tags []TagID) error {
	if err := repo.db.WithContext(ctx).Select("id", "article_type_id", "title", "slug", "digest", "content", "status", "support", "comment", "visited", "version", "created_at", "updated_at", "deleted_at", "is_deleted").Create(&row).Error; err != nil {
		return translate(err)
	}
	return repo.replaceTags(ctx, row.ID, tags)
}

func (repo *Repository) update(ctx context.Context, row articleModel, tags []TagID) error {
	expected := row.Version - 1
	updates := map[string]any{"article_type_id": row.ArticleTypeID, "title": row.Title, "slug": row.Slug, "digest": row.Digest, "content": row.Content, "status": row.Status, "updated_at": row.UpdatedAt, "deleted_at": row.DeletedAt, "is_deleted": row.IsDeleted, "version": gorm.Expr("version + 1")}
	r := repo.db.WithContext(ctx).Model(&articleModel{}).Where("id = ? AND version = ? AND is_deleted = false", row.ID, expected).Updates(updates)
	if r.Error != nil {
		return translate(r.Error)
	}
	if r.RowsAffected == 0 {
		return repo.classifyMiss(ctx, row.ID, expected)
	}
	if row.IsDeleted {
		return nil
	}
	return repo.replaceTags(ctx, row.ID, tags)
}

func (repo *Repository) classifyMiss(ctx context.Context, id int64, expected int64) error {
	var row struct {
		Version int64 `gorm:"column:version"`
	}
	r := repo.db.WithContext(ctx).Table("articles").Select("version").Where("id = ?", id).Take(&row)
	if errors.Is(r.Error, gorm.ErrRecordNotFound) {
		return notFound()
	}
	if r.Error != nil {
		return translate(r.Error)
	}
	if expected <= 0 || row.Version <= 0 {
		return internal(errors.New("文章版本数据不合法"))
	}
	return stale(uint64(expected), uint64(row.Version))
}

func (repo *Repository) replaceTags(ctx context.Context, articleID int64, tags []TagID) error {
	var existing []tagArticleModel
	if err := repo.db.WithContext(ctx).Where("article_id = ?", articleID).Find(&existing).Error; err != nil {
		return translate(err)
	}
	if err := repo.lockTags(ctx, tags); err != nil {
		return err
	}
	wanted := make(map[int64]struct{}, len(tags))
	for _, id := range tags {
		wanted[id.Int64()] = struct{}{}
	}
	for _, link := range existing {
		if _, keep := wanted[link.TagID]; keep {
			delete(wanted, link.TagID)
			continue
		}
		if err := repo.db.WithContext(ctx).Where("article_id = ? AND tag_id = ?", articleID, link.TagID).Delete(&tagArticleModel{}).Error; err != nil {
			return translate(err)
		}
	}
	for id := range wanted {
		if err := repo.db.WithContext(ctx).Select("article_id", "tag_id").Create(&tagArticleModel{ArticleID: articleID, TagID: id}).Error; err != nil {
			return translate(err)
		}
	}
	return nil
}

func (repo *Repository) lockTags(ctx context.Context, tags []TagID) error {
	if len(tags) == 0 {
		return nil
	}
	ids := make([]int64, len(tags))
	for index, id := range tags {
		ids[index] = id.Int64()
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	var rows []struct {
		ID int64 `gorm:"column:id"`
	}
	result := repo.db.WithContext(ctx).Table("tags").Select("id").Where("id IN ? AND is_deleted = false", ids).Order("id ASC").Clauses(clause.Locking{Strength: "SHARE"}).Find(&rows)
	if result.Error != nil {
		return translate(result.Error)
	}
	if len(rows) != len(ids) {
		return precondition()
	}
	return nil
}
