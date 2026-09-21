package image

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

// PrepareReferences validates image availability before the article transaction.
func (service *Service) PrepareReferences(ctx context.Context, author content.AuthorID, keys ReferenceKeys) (PreparedReferences, error) {
	if keys.empty() {
		return PreparedReferences{}, nil
	}
	owner, err := NewImageOwnerID(author.String())
	if err != nil {
		return PreparedReferences{}, validation(err)
	}
	for _, key := range keys.keys {
		if _, err := service.blob.Load(key.String()); err != nil {
			return PreparedReferences{}, validation(errors.New("正文图片不存在或不可用"))
		}
	}
	return PreparedReferences{keys: keys, owner: owner}, nil
}

// ReplaceReferencesInTx commits prepared image references in the caller-owned transaction.
func (service *Service) ReplaceReferencesInTx(ctx context.Context, db *gorm.DB, articleID int64, prepared PreparedReferences, now time.Time) error {
	if db == nil {
		return internal(errors.New("image transaction is nil"))
	}
	return replaceReferences(ctx, &Repository{db: db}, articleID, prepared, now, service.policy.OrphanGrace())
}

func replaceReferences(ctx context.Context, repo *Repository, articleID int64, prepared PreparedReferences, now time.Time, orphanGrace time.Duration) error {
	if err := repo.LockArticleReferences(ctx, articleID); err != nil {
		return err
	}
	if prepared.empty() {
		return replaceEmptyLocked(ctx, repo, articleID, now, orphanGrace)
	}
	oldIDs, err := repo.FindArticleReferences(ctx, articleID)
	if err != nil {
		return err
	}
	locked, err := repo.LockReferenceTransition(ctx, oldIDs, prepared.keys.keys)
	if err != nil {
		return err
	}
	byID := make(map[string]*ArticleImage, len(locked))
	byKey := make(map[string]*ArticleImage, len(locked))
	for _, value := range locked {
		metadata := value.Metadata()
		byID[metadata.ID.String()] = value
		byKey[metadata.StorageKey.String()] = value
	}
	newIDs := make([]ArticleImageID, 0, len(prepared.keys.keys))
	newSet := make(map[string]struct{}, len(prepared.keys.keys))
	newImages := make([]*ArticleImage, 0, len(prepared.keys.keys))
	for _, key := range prepared.keys.keys {
		value, ok := byKey[key.String()]
		if !ok || value.Metadata().OwnerID != prepared.owner {
			return validation(errors.New("正文图片不存在或不可用"))
		}
		if value.Status() != ArticleImageStatusCommitted {
			if err := value.Commit(now); err != nil {
				return validation(fmt.Errorf("确认正文图片：%w", err))
			}
		}
		id := value.Metadata().ID
		newIDs = append(newIDs, id)
		newSet[id.String()] = struct{}{}
		newImages = append(newImages, value)
	}
	removed := make([]ArticleImageID, 0)
	for _, id := range oldIDs {
		if _, keep := newSet[id.String()]; !keep {
			if _, ok := byID[id.String()]; !ok {
				return validation(errors.New("正文图片不存在或不可用"))
			}
			removed = append(removed, id)
		}
	}
	if err := repo.ReplaceArticleReferences(ctx, articleID, newIDs, now); err != nil {
		return err
	}
	for _, value := range newImages {
		if err := repo.UpdateLifecycle(ctx, value); err != nil {
			return err
		}
	}
	for _, id := range removed {
		count, err := repo.CountReferencesForLockedImage(ctx, id)
		if err != nil {
			return err
		}
		if count != 0 {
			continue
		}
		value := byID[id.String()]
		if value.Status() == ArticleImageStatusCommitted {
			if err := value.OrphanWithGrace(now, orphanGrace); err != nil {
				return err
			}
			if err := repo.UpdateLifecycle(ctx, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func replaceEmptyLocked(ctx context.Context, repo *Repository, articleID int64, now time.Time, orphanGrace time.Duration) error {
	oldIDs, err := repo.FindArticleReferences(ctx, articleID)
	if err != nil {
		return err
	}
	if len(oldIDs) == 0 {
		return nil
	}
	locked, err := repo.LockReferenceTransition(ctx, oldIDs, nil)
	if err != nil {
		return err
	}
	if err := repo.ReplaceArticleReferences(ctx, articleID, nil, now); err != nil {
		return err
	}
	for _, value := range locked {
		count, err := repo.CountReferencesForLockedImage(ctx, value.Metadata().ID)
		if err != nil {
			return err
		}
		if count == 0 && value.Status() == ArticleImageStatusCommitted {
			if err := value.OrphanWithGrace(now, orphanGrace); err != nil {
				return err
			}
			if err := repo.UpdateLifecycle(ctx, value); err != nil {
				return err
			}
		}
	}
	return nil
}
