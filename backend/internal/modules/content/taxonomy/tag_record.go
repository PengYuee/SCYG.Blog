package taxonomy

import "github.com/PengYuee/SCYG.Blog/backend/internal/platform/persistence"

// tagModel is the GORM record for tags.
type tagModel struct {
	ID                      int64  `gorm:"column:id;primaryKey"`
	Name                    string `gorm:"column:name"`
	Version                 int64  `gorm:"column:version"`
	persistence.AuditFields `gorm:"embedded"`
}

func (tagModel) TableName() string { return "tags" }

// tagArticleModel is a read-only record used by tag deletion checks.
type tagArticleModel struct {
	ArticleID int64 `gorm:"column:article_id"`
	TagID     int64 `gorm:"column:tag_id"`
}

func (tagArticleModel) TableName() string { return "article_tags" }
