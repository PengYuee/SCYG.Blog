package domain

import "time"

// Tag 标记文章，并维护命名、删除与版本规则。
type Tag struct {
	id         TagID
	name       Name
	version    Version
	createdAt  time.Time
	modifiedAt time.Time
	deletedAt  time.Time
}

// NewTag 创建已校验且初始版本为一的标签。
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

// Rename 在标签有效且版本匹配时原子修改名称。
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

// Delete 原子软删除有效标签。
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
