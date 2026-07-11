package domain

import "time"

// ArticleType classifies articles and owns naming, deletion, and version rules.
type ArticleType struct {
	id         ArticleTypeID
	name       Name
	version    Version
	createdAt  time.Time
	modifiedAt time.Time
	deletedAt  time.Time
}

// NewArticleType creates a version-one article type.
func NewArticleType(id ArticleTypeID, name Name, clock Clock) *ArticleType {
	now := clock.Now().UTC()
	return &ArticleType{id: id, name: name, version: initialVersion(), createdAt: now, modifiedAt: now}
}

// Rename changes the name exactly once when the version matches.
func (item *ArticleType) Rename(expected Version, name Name, clock Clock) error {
	if item.version != expected {
		return &VersionConflict{Expected: expected, Actual: item.version}
	}
	item.name = name
	item.modifiedAt = clock.Now().UTC()
	item.version = item.version.next()
	return nil
}

// Delete soft-deletes the article type when the version matches.
func (item *ArticleType) Delete(expected Version, clock Clock) error {
	if item.version != expected {
		return &VersionConflict{Expected: expected, Actual: item.version}
	}
	item.deletedAt = clock.Now().UTC()
	item.version = item.version.next()
	return nil
}
func (item *ArticleType) ID() ArticleTypeID    { return item.id }
func (item *ArticleType) Name() Name           { return item.name }
func (item *ArticleType) Version() Version     { return item.version }
func (item *ArticleType) DeletedAt() time.Time { return item.deletedAt }

// Tag labels articles and owns naming, deletion, and version rules.
type Tag struct {
	id         TagID
	name       Name
	version    Version
	createdAt  time.Time
	modifiedAt time.Time
	deletedAt  time.Time
}

// NewTag creates a version-one tag.
func NewTag(id TagID, name Name, clock Clock) *Tag {
	now := clock.Now().UTC()
	return &Tag{id: id, name: name, version: initialVersion(), createdAt: now, modifiedAt: now}
}

// Rename changes the name exactly once when the version matches.
func (tag *Tag) Rename(expected Version, name Name, clock Clock) error {
	if tag.version != expected {
		return &VersionConflict{Expected: expected, Actual: tag.version}
	}
	tag.name = name
	tag.modifiedAt = clock.Now().UTC()
	tag.version = tag.version.next()
	return nil
}

// Delete soft-deletes the tag when the version matches.
func (tag *Tag) Delete(expected Version, clock Clock) error {
	if tag.version != expected {
		return &VersionConflict{Expected: expected, Actual: tag.version}
	}
	tag.deletedAt = clock.Now().UTC()
	tag.version = tag.version.next()
	return nil
}
func (tag *Tag) ID() TagID            { return tag.id }
func (tag *Tag) Name() Name           { return tag.name }
func (tag *Tag) Version() Version     { return tag.version }
func (tag *Tag) DeletedAt() time.Time { return tag.deletedAt }

// TagArticle represents only the business association between article and tag.
type TagArticle struct {
	articleID ArticleID
	tagID     TagID
}

// NewTagArticle creates an association from parsed identifiers.
func NewTagArticle(articleID ArticleID, tagID TagID) TagArticle {
	return TagArticle{articleID: articleID, tagID: tagID}
}
func (link TagArticle) ArticleID() ArticleID { return link.articleID }
func (link TagArticle) TagID() TagID         { return link.tagID }
