package content

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// ActionCommitArticleImages 授权作者在文章事务中确认受控正文图片。
const ActionCommitArticleImages Action = "content.article_images.commit"

// managedImageKeys 使用领域 Markdown AST 解析器提取受控图片键。
func managedImageKeys(markdown string) ([]domain.StorageKey, error) {
	return domain.ParseArticleImageReferences([]byte(markdown))
}

type articleImageIdentity struct {
	owner domain.ImageOwnerID
}

// validateManagedImageFiles 在数据库写事务外确认受控图片物理文件可完整读取。
func (module *Module) validateManagedImageFiles(keys []domain.StorageKey) error {
	for _, key := range keys {
		if _, err := module.imageLoader.LoadArticleImage(key.String()); err != nil {
			return validation(errors.New("正文图片不存在或不可用"))
		}
	}
	return nil
}

func (module *Module) imageIdentity(ctx context.Context, keys []domain.StorageKey, articleID int64) (articleImageIdentity, error) {
	if len(keys) == 0 {
		return articleImageIdentity{}, nil
	}
	if err := module.authorizer.Authorize(ctx, ActionCommitArticleImages, Resource{Kind: "article", ID: articleID}); err != nil {
		return articleImageIdentity{}, permission(err)
	}
	author, err := module.currentAuthor.CurrentAuthor(ctx)
	if err != nil {
		return articleImageIdentity{}, permission(err)
	}
	owner, err := domain.NewImageOwnerID(author.String())
	if err != nil {
		return articleImageIdentity{}, validation(err)
	}
	return articleImageIdentity{owner: owner}, nil
}

func (module *Module) bindArticleImages(ctx context.Context, repo application.ArticleImageRepository, articleID domain.ArticleID, keys []domain.StorageKey, identity articleImageIdentity, now time.Time) error {
	oldIDs, err := repo.FindArticleReferences(ctx, articleID)
	if err != nil {
		return err
	}
	lockedImages, err := repo.LockReferenceTransition(ctx, oldIDs, keys)
	if err != nil {
		return err
	}
	byID := make(map[string]*domain.ArticleImage, len(lockedImages))
	byKey := make(map[string]*domain.ArticleImage, len(lockedImages))
	for _, image := range lockedImages {
		metadata := image.Metadata()
		byID[metadata.ID.String()] = image
		byKey[metadata.StorageKey.String()] = image
	}
	newIDs := make([]domain.ArticleImageID, 0, len(keys))
	newSet := make(map[string]struct{}, len(keys))
	newImages := make([]*domain.ArticleImage, 0, len(keys))
	for _, key := range keys {
		image, exists := byKey[key.String()]
		if !exists || image.Metadata().StorageKey != key || image.Metadata().OwnerID != identity.owner {
			return validation(errors.New("正文图片不存在或不可用"))
		}
		if image.Status() != domain.ArticleImageStatusCommitted {
			if commitErr := image.Commit(now); commitErr != nil {
				return validation(fmt.Errorf("确认正文图片：%w", commitErr))
			}
		}
		id := image.Metadata().ID
		newIDs = append(newIDs, id)
		newSet[id.String()] = struct{}{}
		newImages = append(newImages, image)
	}
	removed := make([]domain.ArticleImageID, 0)
	for _, id := range oldIDs {
		if _, retained := newSet[id.String()]; !retained {
			if _, locked := byID[id.String()]; !locked {
				return validation(errors.New("正文图片不存在或不可用"))
			}
			removed = append(removed, id)
		}
	}
	if err := repo.ReplaceArticleReferences(ctx, articleID, newIDs, now); err != nil {
		return err
	}
	for _, image := range newImages {
		if err := repo.Save(ctx, image); err != nil {
			return err
		}
	}
	for _, id := range removed {
		count, countErr := repo.CountReferencesForLockedImage(ctx, id)
		if countErr != nil {
			return countErr
		}
		if count != 0 {
			continue
		}
		removedImage := byID[id.String()]
		if removedImage.Status() == domain.ArticleImageStatusCommitted {
			if orphanErr := removedImage.OrphanWithGrace(now, module.imagePolicy.orDefault().OrphanGrace()); orphanErr != nil {
				return orphanErr
			}
			if saveErr := repo.Save(ctx, removedImage); saveErr != nil {
				return saveErr
			}
		}
	}
	return nil
}
