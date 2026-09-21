package image

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/blobstorage"
)

// FilesystemBlob adapts the local filesystem to the image Blob contract.
type FilesystemBlob struct {
	filesystem   *blobstorage.Filesystem
	maxFileBytes int64
}

// NewFilesystemBlob constructs a filesystem-backed image blob.
func NewFilesystemBlob(filesystem *blobstorage.Filesystem, policy Policy) *FilesystemBlob {
	return &FilesystemBlob{filesystem: filesystem, maxFileBytes: policy.MaxFileBytes()}
}

// ErrStorageUnavailable indicates that image storage is not ready.
var ErrStorageUnavailable = errors.New("图片存储不可用")

func (blob *FilesystemBlob) available() error {
	if blob == nil || blob.filesystem == nil {
		return ErrStorageUnavailable
	}
	return nil
}

// Stage writes an upload to temporary storage.
func (blob *FilesystemBlob) Stage(ctx context.Context, id string, content io.Reader) (string, int64, error) {
	if err := blob.available(); err != nil {
		return "", 0, err
	}
	token, metadata, err := blob.filesystem.WriteTemp(ctx, id, content)
	return token.Name(), metadata.Size, err
}

// Commit promotes a staged upload to its final storage key.
func (blob *FilesystemBlob) Commit(token, key string) error {
	if err := blob.available(); err != nil {
		return err
	}
	return blob.filesystem.CommitTemp(blobstorage.NewTempToken(token), key)
}

// Discard removes a staged upload.
func (blob *FilesystemBlob) Discard(ctx context.Context, token string) error {
	if err := blob.available(); err != nil {
		return err
	}
	return blob.filesystem.DeleteTemp(ctx, token)
}

// Delete removes a committed image blob.
func (blob *FilesystemBlob) Delete(key string) error {
	if err := blob.available(); err != nil {
		return err
	}
	return blob.filesystem.Delete(key)
}

// ListExpiredTemps lists temporary uploads eligible for cleanup.
func (blob *FilesystemBlob) ListExpiredTemps(ctx context.Context, cutoff time.Time, limit int) ([]string, error) {
	if err := blob.available(); err != nil {
		return nil, err
	}
	entries, err := blob.filesystem.ListExpiredTemps(ctx, cutoff, limit)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names, nil
}

// DeleteTemp removes one temporary upload.
func (blob *FilesystemBlob) DeleteTemp(ctx context.Context, name string) error {
	if err := blob.available(); err != nil {
		return err
	}
	return blob.filesystem.DeleteTemp(ctx, name)
}

// Load reads and size-checks one committed image blob.
func (blob *FilesystemBlob) Load(key string) (content []byte, err error) {
	if err := blob.available(); err != nil {
		return nil, err
	}
	file, info, err := blob.filesystem.Open(key)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if info.Size() < 1 || info.Size() > blob.maxFileBytes {
		return nil, errors.New("图片文件大小不合法")
	}
	content, err = io.ReadAll(io.LimitReader(file, blob.maxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) != info.Size() {
		return nil, errors.New("图片文件读取不完整")
	}
	return content, nil
}

var _ Blob = (*FilesystemBlob)(nil)
