package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
	"gorm.io/gorm"
)

type articleRepository struct{ db *gorm.DB }

func (repo *articleRepository) NextID(ctx context.Context) (domain.ArticleID, error) {
	var value int64
	if err := repo.db.WithContext(ctx).Raw(`SELECT nextval(pg_get_serial_sequence('"Article"', 'Id'))`).Scan(&value).Error; err != nil {
		return domain.ArticleID{}, translate(err)
	}
	return domain.NewArticleID(value)
}

func (repo *articleRepository) Find(ctx context.Context, id domain.ArticleID) (*domain.Article, error) {
	var row articleModel
	result := repo.db.WithContext(ctx).Where(`"Id" = ? AND "IsDeleted" = false`, id.Int64()).First(&row)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, notFound("article")
		}
		return nil, translate(result.Error)
	}
	var links []tagArticleModel
	if err := repo.db.WithContext(ctx).Where(`"ArticleId" = ?`, id.Int64()).Order(`"TagId" ASC`).Find(&links).Error; err != nil {
		return nil, translate(err)
	}
	article, err := articleFromModel(row, links)
	if err != nil {
		return nil, fmt.Errorf("map article %d: %w", id.Int64(), err)
	}
	return article, nil
}
func (repo *articleRepository) Save(ctx context.Context, article *domain.Article) error {
	row, err := articleToModel(article)
	if err != nil {
		return err
	}
	if row.Version == 1 {
		return repo.create(ctx, row, article.TagIDs())
	}
	return repo.update(ctx, row, article.TagIDs())
}
func (repo *articleRepository) create(ctx context.Context, row articleModel, tags []domain.TagID) error {
	result := repo.db.WithContext(ctx).Select("Id", "ArticleTypeId", "Title", "Slug", "Digest", "Content", "Status", "Support", "Comment", "Visited", "Version", "CreationTime", "LastModificationTime", "DeletionTime", "IsDeleted").Create(&row)
	if result.Error != nil {
		return translate(result.Error)
	}
	return repo.replaceTags(ctx, row.ID, tags)
}
func (repo *articleRepository) update(ctx context.Context, row articleModel, tags []domain.TagID) error {
	expected := row.Version - 1
	updates := map[string]any{"ArticleTypeId": row.ArticleTypeID, "Title": row.Title, "Slug": row.Slug, "Digest": row.Digest, "Content": row.Content, "Status": row.Status, "LastModificationTime": row.LastModificationTime, "DeletionTime": row.DeletionTime, "IsDeleted": row.IsDeleted, "Version": gorm.Expr(`"Version" + 1`)}
	result := repo.db.WithContext(ctx).Model(&articleModel{}).Where(`"Id" = ? AND "Version" = ? AND "IsDeleted" = false`, row.ID, expected).Updates(updates)
	if result.Error != nil {
		return translate(result.Error)
	}
	if result.RowsAffected == 0 {
		return classifyMiss(ctx, repo.db, "Article", row.ID, uint64(expected))
	}
	if row.IsDeleted {
		return nil
	}
	return repo.replaceTags(ctx, row.ID, tags)
}
func (repo *articleRepository) replaceTags(ctx context.Context, articleID int64, tags []domain.TagID) error {
	var existing []tagArticleModel
	if err := repo.db.WithContext(ctx).Where(`"ArticleId" = ?`, articleID).Find(&existing).Error; err != nil {
		return translate(err)
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
		if err := repo.db.WithContext(ctx).Where(`"ArticleId" = ? AND "TagId" = ?`, articleID, link.TagID).Delete(&tagArticleModel{}).Error; err != nil {
			return translate(err)
		}
	}
	for tagID := range wanted {
		link := tagArticleModel{ArticleID: articleID, TagID: tagID}
		if err := repo.db.WithContext(ctx).Select("ArticleId", "TagId").Create(&link).Error; err != nil {
			return translate(err)
		}
	}
	return nil
}

type versionRow struct {
	Version   int64 `gorm:"column:Version"`
	IsDeleted bool  `gorm:"column:IsDeleted"`
}

func classifyMiss(ctx context.Context, db *gorm.DB, table string, id int64, expected uint64) error {
	var row versionRow
	result := db.WithContext(ctx).Table(table).Select(`"Version", "IsDeleted"`).Where(`"Id" = ?`, id).Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return notFound(table)
	}
	if result.Error != nil {
		return translate(result.Error)
	}
	return stale(expected, uint64(row.Version))
}
