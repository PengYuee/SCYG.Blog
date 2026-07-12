package domain

import (
	"math"
	"time"
	"unicode"
	"unicode/utf8"
)

// ArticleType 对文章分类，并维护命名、删除与版本规则。
type ArticleType struct {
	id         ArticleTypeID
	name       Name
	image      *string
	meun       int32
	version    Version
	createdAt  time.Time
	modifiedAt time.Time
	deletedAt  time.Time
}

// NewArticleType 创建已校验且初始版本为一的文章分类。
func NewArticleType(id ArticleTypeID, name Name, clock Clock) (*ArticleType, error) {
	return NewArticleTypeWithDetails(id, name, nil, 0, clock)
}

// NewArticleTypeWithDetails 创建包含展示元数据的已校验文章分类。
func NewArticleTypeWithDetails(id ArticleTypeID, name Name, image *string, meun int32, clock Clock) (*ArticleType, error) {
	if !id.valid() {
		return nil, invalid("article_type_id")
	}
	if !name.valid() {
		return nil, invalid("name")
	}
	imageValue, err := parseImage(image)
	if err != nil || meun < 0 || meun > math.MaxInt16 {
		return nil, invalid("article_type_details")
	}
	now, err := clockTime(clock, time.Time{})
	if err != nil {
		return nil, err
	}
	return &ArticleType{id: id, name: name, image: imageValue, meun: meun, version: initialVersion(), createdAt: now, modifiedAt: now}, nil
}

// ArticleTypePatch 包含调用方明确提供的可变文章分类值。
type ArticleTypePatch struct {
	Name          *Name
	ImageProvided bool
	Image         *string
	Meun          *int32
}

// Patch 原子更新调用方提供的文章分类值。
func (item *ArticleType) Patch(expected Version, patch ArticleTypePatch, clock Clock) error {
	if err := taxonomyCurrent(item.version, expected, item.deletedAt); err != nil {
		return err
	}
	if patch.Name == nil && !patch.ImageProvided && patch.Meun == nil {
		return ErrNoChange
	}
	name, image, meun := item.name, copyString(item.image), item.meun
	if patch.Name != nil {
		if !patch.Name.valid() {
			return invalid("name")
		}
		name = *patch.Name
	}
	if patch.ImageProvided {
		parsed, err := parseImage(patch.Image)
		if err != nil {
			return err
		}
		image = parsed
	}
	if patch.Meun != nil {
		if *patch.Meun < 0 || *patch.Meun > math.MaxInt16 {
			return invalid("meun")
		}
		meun = *patch.Meun
	}
	if item.name == name && equalString(item.image, image) && item.meun == meun {
		return ErrNoChange
	}
	next, err := item.version.next()
	if err != nil {
		return err
	}
	now, err := clockTime(clock, item.modifiedAt)
	if err != nil {
		return err
	}
	item.name, item.image, item.meun, item.modifiedAt, item.version = name, image, meun, now, next
	return nil
}

// Rename 在实体有效且版本匹配时原子修改名称。
func (item *ArticleType) Rename(expected Version, name Name, clock Clock) error {
	return item.Patch(expected, ArticleTypePatch{Name: &name}, clock)
}

// Delete 原子软删除有效文章分类。
func (item *ArticleType) Delete(expected Version, clock Clock) error {
	if err := taxonomyCurrent(item.version, expected, item.deletedAt); err != nil {
		return err
	}
	next, err := item.version.next()
	if err != nil {
		return err
	}
	now, err := clockTime(clock, item.modifiedAt)
	if err != nil {
		return err
	}
	item.deletedAt, item.modifiedAt, item.version = now, now, next
	return nil
}
func (item *ArticleType) ID() ArticleTypeID     { return item.id }
func (item *ArticleType) Name() Name            { return item.name }
func (item *ArticleType) Image() *string        { return copyString(item.image) }
func (item *ArticleType) Meun() int32           { return item.meun }
func (item *ArticleType) Version() Version      { return item.version }
func (item *ArticleType) CreatedAt() time.Time  { return item.createdAt }
func (item *ArticleType) ModifiedAt() time.Time { return item.modifiedAt }
func (item *ArticleType) DeletedAt() time.Time  { return item.deletedAt }
func parseImage(image *string) (*string, error) {
	if image == nil {
		return nil, nil
	}
	if !utf8.ValidString(*image) || len([]rune(*image)) > 512 {
		return nil, invalid("image")
	}
	for _, character := range *image {
		if unicode.IsControl(character) {
			return nil, invalid("image")
		}
	}
	value := *image
	return &value, nil
}

// ValidateImage 校验可选文章分类图片文本且不修改输入。
func ValidateImage(image *string) error { _, err := parseImage(image); return err }
func copyString(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
func equalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
