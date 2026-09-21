package taxonomy

import (
	"context"
	"errors"
	"math"
	"time"

	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

// CreateArticleType creates an article type.
func (service *Service) CreateArticleType(ctx context.Context, command CreateArticleType) (ArticleTypeResult, error) {
	name, err := parseName(command.Name)
	if err != nil {
		return ArticleTypeResult{}, err
	}
	if err = service.authorizer.Authorize(ctx, ActionManageArticleType, content.Resource{Kind: "article_type"}); err != nil {
		return ArticleTypeResult{}, permission()
	}
	var image *string
	var result ArticleTypeResult
	err = service.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := newRepository(tx)
		id, nextErr := repo.nextArticleTypeID(ctx)
		if nextErr != nil {
			return nextErr
		}
		if err := parseMeun(command.Meun); err != nil {
			return err
		}
		var parseErr error
		image, parseErr = parseImage(command.Image)
		if parseErr != nil {
			return parseErr
		}
		now, clockErr := service.now(time.Time{})
		if clockErr != nil {
			return clockErr
		}
		entity := articleTypeEntity{ID: id, Name: name, Image: image, Meun: command.Meun, Version: 1, CreatedAt: now, Modified: now}
		if saveErr := repo.createArticleType(ctx, entity); saveErr != nil {
			return saveErr
		}
		result = articleTypeResult(entity)
		return nil
	})
	return result, stable(err)
}

// PatchArticleType applies partial changes to an article type.
func (service *Service) PatchArticleType(ctx context.Context, command PatchArticleType) (ArticleTypeResult, error) {
	if err := parseID(command.ID, "article_type_id"); err != nil {
		return ArticleTypeResult{}, err
	}
	if err := parseVersion(command.Version); err != nil {
		return ArticleTypeResult{}, err
	}
	if command.Name == nil && !command.Image.Provided && command.Meun == nil {
		return ArticleTypeResult{}, invalid("patch")
	}
	var name *string
	if command.Name != nil {
		parsed, err := parseName(*command.Name)
		if err != nil {
			return ArticleTypeResult{}, err
		}
		name = &parsed
	}
	var image *string
	if command.Image.Provided {
		image = command.Image.Value
	}
	if err := service.authorizer.Authorize(ctx, ActionManageArticleType, content.Resource{Kind: "article_type", ID: command.ID}); err != nil {
		return ArticleTypeResult{}, permission()
	}
	var result ArticleTypeResult
	err := service.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := newRepository(tx)
		entity, err := repo.findArticleType(ctx, command.ID, true)
		if err != nil {
			return err
		}
		if entity.Version != command.Version {
			return stale(command.Version, entity.Version)
		}
		if command.Meun != nil {
			if err := parseMeun(*command.Meun); err != nil {
				return err
			}
		}
		if command.Image.Provided {
			var parseErr error
			image, parseErr = parseImage(command.Image.Value)
			if parseErr != nil {
				return parseErr
			}
		}
		oldName, oldImage, oldMeun := entity.Name, copyImage(entity.Image), entity.Meun
		if name != nil {
			entity.Name = *name
		}
		if command.Image.Provided {
			entity.Image = image
		}
		if command.Meun != nil {
			entity.Meun = *command.Meun
		}
		if oldName == entity.Name && equalImage(oldImage, entity.Image) && oldMeun == entity.Meun {
			return ErrNoChange
		}
		if entity.Version == math.MaxUint64 {
			return internalWithCause(errors.New("taxonomy version exhausted"))
		}
		now, clockErr := service.now(entity.Modified)
		if clockErr != nil {
			return clockErr
		}
		entity.Modified = now
		entity.Version++
		if err := repo.updateArticleType(ctx, entity, command.Version); err != nil {
			return err
		}
		result = articleTypeResult(entity)
		return nil
	})
	return result, stable(err)
}

// RenameArticleType changes an article type name.
func (service *Service) RenameArticleType(ctx context.Context, command RenameArticleType) (ArticleTypeResult, error) {
	return service.PatchArticleType(ctx, PatchArticleType{ID: command.ID, Version: command.Version, Name: &command.Name})
}

// DeleteArticleType soft-deletes an article type when its version matches.
func (service *Service) DeleteArticleType(ctx context.Context, command DeleteArticleType) error {
	if err := parseID(command.ID, "article_type_id"); err != nil {
		return err
	}
	if err := parseVersion(command.Version); err != nil {
		return err
	}
	if err := service.authorizer.Authorize(ctx, ActionManageArticleType, content.Resource{Kind: "article_type", ID: command.ID}); err != nil {
		return permission()
	}
	err := service.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := newRepository(tx)
		entity, err := repo.findArticleType(ctx, command.ID, true)
		if err != nil {
			return err
		}
		if entity.Version != command.Version {
			return stale(command.Version, entity.Version)
		}
		now, err := service.now(entity.Modified)
		if err != nil {
			return err
		}
		entity.DeletedAt, entity.Modified = now, now
		entity.Version++
		return repo.updateArticleType(ctx, entity, command.Version)
	})
	return stable(err)
}

func equalImage(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
