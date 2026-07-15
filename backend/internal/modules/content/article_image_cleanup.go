package content

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

const articleImageCleanupBatchSize = 100

// ArticleImageFinalDeleter 删除最终图片。
type ArticleImageFinalDeleter interface {
	// DeleteArticleImage 幂等删除受控最终图片。
	DeleteArticleImage(string) error
}

// ArticleImageTempLister 枚举过期临时图片。
type ArticleImageTempLister interface {
	// ListExpiredArticleImageTemps 返回截止时间前的受控临时图片。
	ListExpiredArticleImageTemps(context.Context, time.Time, int) ([]string, error)
}

// ArticleImageTempDeleter 删除临时图片。
type ArticleImageTempDeleter interface {
	// DeleteArticleImageTemp 幂等删除受控临时图片。
	DeleteArticleImageTemp(context.Context, string) error
}

// unavailableArticleImageCleanupStorage 为未装配存储的旧测试提供显式中文错误。
type unavailableArticleImageCleanupStorage struct{}

func (unavailableArticleImageCleanupStorage) DeleteArticleImage(string) error {
	return errors.New("图片清理存储不可用")
}

func (unavailableArticleImageCleanupStorage) ListExpiredArticleImageTemps(context.Context, time.Time, int) ([]string, error) {
	return nil, errors.New("图片清理存储不可用")
}

func (unavailableArticleImageCleanupStorage) DeleteArticleImageTemp(context.Context, string) error {
	return errors.New("图片清理存储不可用")
}

// articleImageFinalDeleterOrUnavailable 将可选最终删除端口规范化为非空实现。
func articleImageFinalDeleterOrUnavailable(storage ArticleImageFinalDeleter) ArticleImageFinalDeleter {
	if nilLike(storage) {
		return unavailableArticleImageCleanupStorage{}
	}
	return storage
}

// articleImageTempListerOrUnavailable 将可选临时枚举端口规范化为非空实现。
func articleImageTempListerOrUnavailable(storage ArticleImageTempLister) ArticleImageTempLister {
	if nilLike(storage) {
		return unavailableArticleImageCleanupStorage{}
	}
	return storage
}

// articleImageTempDeleterOrUnavailable 将可选临时删除端口规范化为非空实现。
func articleImageTempDeleterOrUnavailable(storage ArticleImageTempDeleter) ArticleImageTempDeleter {
	if nilLike(storage) {
		return unavailableArticleImageCleanupStorage{}
	}
	return storage
}

// CleanupArticleImages 清理已过期元数据和临时文件；单条失败不阻断同轮其他候选。
func (module *Module) CleanupArticleImages(ctx context.Context) error {
	now := module.clock.Now()
	var failures []error
	for _, status := range []domain.ArticleImageStatus{domain.ArticleImageStatusPending, domain.ArticleImageStatusOrphaned} {
		candidates, err := module.cleanupCandidates(ctx, status, now)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for _, candidate := range candidates {
			if err = module.cleanupCandidate(ctx, candidate, status, now); err != nil {
				failures = append(failures, fmt.Errorf("清理图片 %s：%w", candidate.Metadata().ID.String(), err))
			}
		}
	}
	temps, err := module.imageTempLister.ListExpiredArticleImageTemps(ctx, now.Add(-module.imagePendingTTL), articleImageCleanupBatchSize)
	if err != nil {
		failures = append(failures, fmt.Errorf("枚举过期临时图片：%w", err))
	} else {
		for _, name := range temps {
			if deleteErr := module.imageTempDeleter.DeleteArticleImageTemp(ctx, name); deleteErr != nil {
				failures = append(failures, fmt.Errorf("清理临时图片 %s：%w", name, deleteErr))
			}
		}
	}
	return errors.Join(failures...)
}

// cleanupCandidates 在短事务中取得一批确定性候选。
func (module *Module) cleanupCandidates(ctx context.Context, status domain.ArticleImageStatus, now time.Time) ([]*domain.ArticleImage, error) {
	var candidates []*domain.ArticleImage
	err := module.unit.Within(ctx, func(txctx context.Context, transaction application.Transaction) error {
		var listErr error
		switch status {
		case domain.ArticleImageStatusPending:
			candidates, listErr = transaction.ArticleImages().ListExpiredPending(txctx, now, articleImageCleanupBatchSize)
		case domain.ArticleImageStatusOrphaned:
			candidates, listErr = transaction.ArticleImages().ListExpiredOrphaned(txctx, now, articleImageCleanupBatchSize)
		default:
			return errors.New("不支持的图片清理状态")
		}
		return listErr
	})
	return candidates, err
}

// cleanupCandidate 重新锁定并复核单条候选，隔离失败且阻止并发重新引用竞态。
func (module *Module) cleanupCandidate(ctx context.Context, candidate *domain.ArticleImage, expected domain.ArticleImageStatus, now time.Time) error {
	return module.unit.Within(ctx, func(txctx context.Context, transaction application.Transaction) error {
		locked, err := transaction.ArticleImages().FindForUpdate(txctx, []domain.ArticleImageID{candidate.Metadata().ID})
		if err != nil || len(locked) == 0 {
			return err
		}
		image := locked[0]
		if image.Status() != expected || image.ExpiresAt().After(now) {
			return nil
		}
		if expected == domain.ArticleImageStatusOrphaned {
			references, countErr := transaction.ArticleImages().CountReferencesForLockedImage(txctx, image.Metadata().ID)
			if countErr != nil {
				return countErr
			}
			if references > 0 {
				return nil
			}
		}
		// 文件删除成功（含不存在）后才删除元数据；DB 失败时保留行供下一轮幂等重试。
		if err = module.imageFinalDeleter.DeleteArticleImage(image.Metadata().StorageKey.String()); err != nil {
			return err
		}
		return transaction.ArticleImages().DeleteMetadata(txctx, image.Metadata().ID)
	})
}
