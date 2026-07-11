package domain

import (
	"math"
	"time"
)

// ArticleType classifies articles and owns naming, deletion, and version rules.
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

// NewArticleType creates a validated version-one article type.
func NewArticleType(id ArticleTypeID, name Name, clock Clock) (*ArticleType, error) {
	return NewArticleTypeWithDetails(id, name, nil, 0, clock)
}

// NewArticleTypeWithDetails creates a validated article type with presentation metadata.
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

// ArticleTypePatch contains explicitly provided mutable article-type values.
type ArticleTypePatch struct {
	Name          *Name
	ImageProvided bool
	Image         *string
	Meun          *int32
}

// Patch atomically updates any provided article-type values.
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

// Rename atomically changes the name when the entity is active and version matches.
func (item *ArticleType) Rename(expected Version, name Name, clock Clock) error {
	return item.Patch(expected, ArticleTypePatch{Name: &name}, clock)
}

// Delete atomically soft-deletes the active article type.
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

// Tag labels articles and owns naming, deletion, and version rules.
type Tag struct {
	id         TagID
	name       Name
	version    Version
	createdAt  time.Time
	modifiedAt time.Time
	deletedAt  time.Time
}

// NewTag creates a validated version-one tag.
func NewTag(id TagID, name Name, clock Clock) (*Tag, error) {
	if !id.valid() {
		return nil, invalid("tag_id")
	}
	if !name.valid() {
		return nil, invalid("name")
	}
	now, err := clockTime(clock, time.Time{})
	if err != nil {
		return nil, err
	}
	return &Tag{id: id, name: name, version: initialVersion(), createdAt: now, modifiedAt: now}, nil
}

// Rename atomically changes the name when the tag is active and version matches.
func (tag *Tag) Rename(expected Version, name Name, clock Clock) error {
	if err := taxonomyCurrent(tag.version, expected, tag.deletedAt); err != nil {
		return err
	}
	if !name.valid() {
		return invalid("name")
	}
	if tag.name == name {
		return ErrNoChange
	}
	next, err := tag.version.next()
	if err != nil {
		return err
	}
	now, err := clockTime(clock, tag.modifiedAt)
	if err != nil {
		return err
	}
	tag.name, tag.modifiedAt, tag.version = name, now, next
	return nil
}

// Delete atomically soft-deletes the active tag.
func (tag *Tag) Delete(expected Version, clock Clock) error {
	if err := taxonomyCurrent(tag.version, expected, tag.deletedAt); err != nil {
		return err
	}
	next, err := tag.version.next()
	if err != nil {
		return err
	}
	now, err := clockTime(clock, tag.modifiedAt)
	if err != nil {
		return err
	}
	tag.deletedAt, tag.modifiedAt, tag.version = now, now, next
	return nil
}
func (tag *Tag) ID() TagID             { return tag.id }
func (tag *Tag) Name() Name            { return tag.name }
func (tag *Tag) Version() Version      { return tag.version }
func (tag *Tag) CreatedAt() time.Time  { return tag.createdAt }
func (tag *Tag) ModifiedAt() time.Time { return tag.modifiedAt }
func (tag *Tag) DeletedAt() time.Time  { return tag.deletedAt }

func taxonomyCurrent(actual, expected Version, deletedAt time.Time) error {
	if !expected.valid() || actual != expected {
		return &VersionConflict{Expected: expected, Actual: actual}
	}
	if !deletedAt.IsZero() {
		return ErrDeleted
	}
	return nil
}

// TagArticle represents only the business association between article and tag.
type TagArticle struct {
	articleID ArticleID
	tagID     TagID
}

// NewTagArticle creates a validated association from parsed identifiers.
func NewTagArticle(articleID ArticleID, tagID TagID) (TagArticle, error) {
	if !articleID.valid() {
		return TagArticle{}, invalid("article_id")
	}
	if !tagID.valid() {
		return TagArticle{}, invalid("tag_id")
	}
	return TagArticle{articleID: articleID, tagID: tagID}, nil
}
func (link TagArticle) ArticleID() ArticleID { return link.articleID }
func (link TagArticle) TagID() TagID         { return link.tagID }

func parseImage(image *string) (*string, error) {
	if image == nil {
		return nil, nil
	}
	if len([]rune(*image)) > 512 {
		return nil, invalid("image")
	}
	value := *image
	return &value, nil
}
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
