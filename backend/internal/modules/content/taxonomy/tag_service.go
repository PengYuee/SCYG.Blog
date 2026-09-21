package taxonomy

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

// CreateTag creates a tag.
func (service *Service) CreateTag(ctx context.Context, command CreateTag) (TagResult, error) {
	name, err := parseName(command.Name)
	if err != nil {
		return TagResult{}, err
	}
	if err = service.authorizer.Authorize(ctx, ActionManageTag, content.Resource{Kind: "tag"}); err != nil {
		return TagResult{}, permission()
	}
	var result TagResult
	err = service.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := newRepository(tx)
		id, nextErr := repo.nextTagID(ctx)
		if nextErr != nil {
			return nextErr
		}
		now, clockErr := service.now(time.Time{})
		if clockErr != nil {
			return clockErr
		}
		entity := tagEntity{ID: id, Name: name, Version: 1, CreatedAt: now, Modified: now}
		if saveErr := repo.createTag(ctx, entity); saveErr != nil {
			return saveErr
		}
		result = tagResult(entity)
		return nil
	})
	return result, stable(err)
}

// RenameTag changes a tag name.
func (service *Service) RenameTag(ctx context.Context, command RenameTag) (TagResult, error) {
	if err := parseID(command.ID, "tag_id"); err != nil {
		return TagResult{}, err
	}
	if err := parseVersion(command.Version); err != nil {
		return TagResult{}, err
	}
	name, err := parseName(command.Name)
	if err != nil {
		return TagResult{}, err
	}
	if err = service.authorizer.Authorize(ctx, ActionManageTag, content.Resource{Kind: "tag", ID: command.ID}); err != nil {
		return TagResult{}, permission()
	}
	var result TagResult
	err = service.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := newRepository(tx)
		entity, findErr := repo.findTag(ctx, command.ID, true)
		if findErr != nil {
			return findErr
		}
		if entity.Version != command.Version {
			return stale(command.Version, entity.Version)
		}
		if entity.Name == name {
			return ErrNoChange
		}
		now, clockErr := service.now(entity.Modified)
		if clockErr != nil {
			return clockErr
		}
		entity.Name, entity.Modified, entity.Version = name, now, entity.Version+1
		if err := repo.updateTag(ctx, entity, command.Version); err != nil {
			return err
		}
		result = tagResult(entity)
		return nil
	})
	return result, stable(err)
}

// DeleteTag soft-deletes a tag when its version matches.
func (service *Service) DeleteTag(ctx context.Context, command DeleteTag) error {
	if err := parseID(command.ID, "tag_id"); err != nil {
		return err
	}
	if err := parseVersion(command.Version); err != nil {
		return err
	}
	if err := service.authorizer.Authorize(ctx, ActionManageTag, content.Resource{Kind: "tag", ID: command.ID}); err != nil {
		return permission()
	}
	err := service.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := newRepository(tx)
		entity, findErr := repo.findTag(ctx, command.ID, true)
		if findErr != nil {
			return findErr
		}
		if entity.Version != command.Version {
			return stale(command.Version, entity.Version)
		}
		now, clockErr := service.now(entity.Modified)
		if clockErr != nil {
			return clockErr
		}
		entity.DeletedAt, entity.Modified, entity.Version = now, now, entity.Version+1
		return repo.updateTag(ctx, entity, command.Version)
	})
	return stable(err)
}
