package application

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

// ArticleImageStorage 是图片用例消费的持久 blob 端口。
type ArticleImageStorage interface {
	WriteTemp(context.Context, domain.ArticleImageID, io.Reader) (string, int64, error)
	CommitTemp(string, domain.StorageKey) error
	Open(domain.StorageKey) (io.ReadSeekCloser, int64, error)
	Delete(domain.StorageKey) error
	ListExpiredTemps(context.Context, time.Time, int) ([]string, error)
	DeleteTemp(context.Context, string) error
}

// ArticleImageRepository 是事务内图片元数据与引用端口。
type ArticleImageRepository interface {
	Save(context.Context, *domain.ArticleImage) error
	Find(context.Context, domain.ArticleImageID) (*domain.ArticleImage, error)
	FindByStorageKey(context.Context, domain.StorageKey) (*domain.ArticleImage, error)
	FindOwner(context.Context, domain.ArticleImageID) (domain.ImageOwnerID, error)
	FindForUpdate(context.Context, []domain.ArticleImageID) ([]*domain.ArticleImage, error)
	FindForUpdateByStorageKeys(context.Context, []domain.StorageKey) ([]*domain.ArticleImage, error)
	LockReferenceTransition(context.Context, []domain.ArticleImageID, []domain.StorageKey) ([]*domain.ArticleImage, error)
	FindArticleReferences(context.Context, domain.ArticleID) ([]domain.ArticleImageID, error)
	ReplaceArticleReferences(context.Context, domain.ArticleID, []domain.ArticleImageID, time.Time) error
	CountReferencesForUpdate(context.Context, domain.ArticleImageID) (int64, error)
	CountReferencesForLockedImage(context.Context, domain.ArticleImageID) (int64, error)
	ListExpiredPending(context.Context, time.Time, int) ([]*domain.ArticleImage, error)
	ListExpiredOrphaned(context.Context, time.Time, int) ([]*domain.ArticleImage, error)
	DeleteMetadata(context.Context, domain.ArticleImageID) error
}

// StorageCommitSucceeded 判断错误是否仅表示提交后的临时清理失败。
func StorageCommitSucceeded(err error) bool {
	var committed interface{ Committed() bool }
	return errors.As(err, &committed) && committed.Committed()
}
