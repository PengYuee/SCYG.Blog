package article

import "time"

// projectionRow is the read-only Article projection used by article queries.
type projectionRow struct {
	ID            int64      `gorm:"column:id"`
	ArticleTypeID int64      `gorm:"column:article_type_id"`
	Title         string     `gorm:"column:title"`
	Slug          string     `gorm:"column:slug"`
	Digest        string     `gorm:"column:digest"`
	Content       string     `gorm:"column:content"`
	Support       int64      `gorm:"column:support"`
	Comment       int64      `gorm:"column:comment"`
	Visited       int64      `gorm:"column:visited"`
	Status        int16      `gorm:"column:status"`
	Version       int64      `gorm:"column:version"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     *time.Time `gorm:"column:updated_at"`
}
