package image

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// GetMedia reads image metadata and the committed Blob content.
func (service *Service) GetMedia(ctx context.Context, query Get) (Media, error) {
	key, err := NewStorageKey(query.StorageKey)
	if err != nil {
		return Media{}, validation(err)
	}
	return service.getMedia(ctx, key, service.repository.FindByStorageKey)
}

func (service *Service) getMedia(ctx context.Context, key StorageKey, find func(context.Context, StorageKey) (*ArticleImage, error)) (Media, error) {
	image, err := find(ctx, key)
	if err != nil {
		return Media{}, stable(err)
	}
	if image.Status() == ArticleImageStatusOrphaned {
		return Media{}, notFound()
	}
	pending := image.Status() == ArticleImageStatusPending
	if pending {
		author, authorErr := service.currentAuthor.CurrentAuthor(ctx)
		if authorErr != nil || author.String() != image.Metadata().OwnerID.String() {
			return Media{}, notFound()
		}
	}
	data, err := service.blob.Load(key.String())
	if err != nil {
		return Media{}, notFound()
	}
	metadata := image.Metadata()
	if int64(len(data)) != metadata.ByteSize {
		return Media{}, internal(errors.New("图片元数据与 Blob 大小不一致"))
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != metadata.SHA256 {
		return Media{}, internal(errors.New("图片元数据与 Blob 校验和不一致"))
	}
	return Media{Content: data, MediaType: string(metadata.MediaType), ByteSize: metadata.ByteSize, SHA256: metadata.SHA256, Pending: pending}, nil
}
