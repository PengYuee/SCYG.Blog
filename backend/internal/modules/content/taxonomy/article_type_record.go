package taxonomy

import "github.com/PengYuee/SCYG.Blog/backend/internal/platform/persistence"

// articleTypeModel is the GORM record for article_types.
type articleTypeModel struct {
	ID                      int64   `gorm:"column:id;primaryKey"`
	Name                    string  `gorm:"column:name"`
	Image                   *string `gorm:"column:image"`
	Meun                    int16   `gorm:"column:meun"`
	Version                 int64   `gorm:"column:version"`
	persistence.AuditFields `gorm:"embedded"`
}

func (articleTypeModel) TableName() string { return "article_types" }

// articleModel is a read-only record used by article type deletion checks.
type articleModel struct {
	ArticleTypeID int64 `gorm:"column:article_type_id"`
	IsDeleted     bool  `gorm:"column:is_deleted"`
}

func (articleModel) TableName() string { return "articles" }
