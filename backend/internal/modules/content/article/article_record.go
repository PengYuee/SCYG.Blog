package article

import "github.com/PengYuee/SCYG.Blog/backend/internal/platform/persistence"

type articleModel struct {
	ID                      int64  `gorm:"column:id;primaryKey"`
	ArticleTypeID           int64  `gorm:"column:article_type_id"`
	Title                   string `gorm:"column:title"`
	Slug                    string `gorm:"column:slug"`
	Digest                  string `gorm:"column:digest"`
	Content                 string `gorm:"column:content"`
	Status                  int16  `gorm:"column:status"`
	Support                 int64  `gorm:"column:support"`
	Comment                 int64  `gorm:"column:comment"`
	Visited                 int64  `gorm:"column:visited"`
	Version                 int64  `gorm:"column:version"`
	persistence.AuditFields `gorm:"embedded"`
}

func (articleModel) TableName() string { return "articles" }

type tagArticleModel struct {
	ArticleID int64 `gorm:"column:article_id;primaryKey"`
	TagID     int64 `gorm:"column:tag_id;primaryKey"`
}

func (tagArticleModel) TableName() string { return "article_tags" }
