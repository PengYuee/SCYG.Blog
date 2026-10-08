package article

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// SavePatchPreviewInTx persists a preview exactly once without replaying its patch.
func (service *Service) SavePatchPreviewInTx(ctx context.Context, tx *gorm.DB, preview PatchPreview) (Result, error) {
	if tx == nil || preview.Article == nil {
		return Result{}, internal(errors.New("article patch preview is nil"))
	}
	if err := (&Repository{db: tx}).Save(ctx, preview.Article); err != nil {
		return Result{}, err
	}
	return result(preview.Article), nil
}

// PublishInTx authorizes and publishes on the caller-owned connection.
func (service *Service) PublishInTx(ctx context.Context, tx *gorm.DB, input Publish) (Result, error) {
	if err := authorize(service.authorizer, ctx, ActionPublishArticle, "article", input.ID); err != nil {
		return Result{}, err
	}
	return service.transitionInTx(ctx, tx, input.ID, input.Version, true)
}

// ArchiveInTx authorizes and archives on the caller-owned connection.
func (service *Service) ArchiveInTx(ctx context.Context, tx *gorm.DB, input Archive) (Result, error) {
	if err := authorize(service.authorizer, ctx, ActionArchiveArticle, "article", input.ID); err != nil {
		return Result{}, err
	}
	return service.transitionInTx(ctx, tx, input.ID, input.Version, false)
}

func (service *Service) transitionInTx(ctx context.Context, tx *gorm.DB, rawID int64, rawVersion uint64, publish bool) (Result, error) {
	if tx == nil {
		return Result{}, internal(errors.New("article transaction is nil"))
	}
	id, version, err := parseIdentity(rawID, rawVersion)
	if err != nil {
		return Result{}, validation(err)
	}
	repo := &Repository{db: tx}
	value, err := repo.Find(ctx, id)
	if err != nil {
		return Result{}, err
	}
	if publish {
		err = value.Publish(version, service.clock)
	} else {
		err = value.Archive(version, service.clock)
	}
	if err != nil {
		return Result{}, stable(err)
	}
	if err := repo.Save(ctx, value); err != nil {
		return Result{}, err
	}
	return result(value), nil
}
