package postgres

import (
	"context"
	"errors"
	"io"
	"time"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/blobstorage"
)

// ImageStorage 将固定根文件系统适配为内容模块的领域图片能力。
type ImageStorage struct {
	filesystem   *blobstorage.Filesystem
	maxFileBytes int64
}

// NewImageStorage 构造图片存储适配器。
func NewImageStorage(filesystem *blobstorage.Filesystem, policy module.ArticleImagePolicy) *ImageStorage {
	return &ImageStorage{filesystem: filesystem, maxFileBytes: policy.MaxFileBytes()}
}

// DeleteArticleImage 幂等删除最终图片。
func (storage *ImageStorage) DeleteArticleImage(key string) error {
	return storage.filesystem.Delete(key)
}

// ListExpiredArticleImageTemps 返回达到截止时间的临时图片名称。
func (storage *ImageStorage) ListExpiredArticleImageTemps(ctx context.Context, cutoff time.Time, limit int) ([]string, error) {
	entries, err := storage.filesystem.ListExpiredTemps(ctx, cutoff, limit)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	return names, nil
}

// DeleteArticleImageTemp 幂等删除临时图片。
func (storage *ImageStorage) DeleteArticleImageTemp(ctx context.Context, name string) error {
	return storage.filesystem.DeleteTemp(ctx, name)
}

type imageContentReader struct{ content module.ArticleImageContent }

func (reader imageContentReader) Read(buffer []byte) (int, error) {
	return reader.content.ReadArticleImage(buffer)
}

// StageArticleImage 写入已验证图片的同目录临时对象。
func (storage *ImageStorage) StageArticleImage(ctx context.Context, id string, content module.ArticleImageContent) (string, int64, error) {
	token, metadata, err := storage.filesystem.WriteTemp(ctx, id, imageContentReader{content: content})
	return token.Name(), metadata.Size, err
}

// CommitArticleImage 原子提交临时对象。
func (storage *ImageStorage) CommitArticleImage(token, key string) error {
	return storage.filesystem.CommitTemp(blobstorage.NewTempToken(token), key)
}

// DiscardArticleImage 幂等删除受控临时图片。
func (storage *ImageStorage) DiscardArticleImage(ctx context.Context, token string) error {
	return storage.filesystem.DeleteTemp(ctx, token)
}

// LoadArticleImage 使用注入上限打开、读取并关闭最终图片。
func (storage *ImageStorage) LoadArticleImage(key string) (content []byte, err error) {
	file, info, err := storage.filesystem.Open(key)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if info.Size() < 1 || info.Size() > storage.maxFileBytes {
		return nil, errors.New("图片文件大小不合法")
	}
	content, err = io.ReadAll(io.LimitReader(file, storage.maxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) != info.Size() {
		return nil, errors.New("图片文件读取不完整")
	}
	return content, nil
}

var (
	_ module.ArticleImageStager       = (*ImageStorage)(nil)
	_ module.ArticleImagePublisher    = (*ImageStorage)(nil)
	_ module.ArticleImageDiscarder    = (*ImageStorage)(nil)
	_ module.ArticleImageLoader       = (*ImageStorage)(nil)
	_ module.ArticleImageFinalDeleter = (*ImageStorage)(nil)
	_ module.ArticleImageTempLister   = (*ImageStorage)(nil)
	_ module.ArticleImageTempDeleter  = (*ImageStorage)(nil)
)
