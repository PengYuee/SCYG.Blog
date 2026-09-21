package image

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"

	"gorm.io/gorm"
)

// Upload stages, validates, persists, and commits an article image.
func (service *Service) Upload(ctx context.Context, command Upload) (Result, error) {
	if err := service.authorizer.Authorize(ctx, ActionUploadArticleImage, content.Resource{Kind: "article_image"}); err != nil {
		return Result{}, permission()
	}
	author, err := service.currentAuthor.CurrentAuthor(ctx)
	if err != nil {
		return Result{}, permission()
	}
	if command.Content == nil {
		return Result{}, validation(errors.New("缺少图片文件"))
	}
	validated, err := ValidateArticleImageWithLimits(command.Content, NewArticleImageValidationLimits(service.policy.MaxFileBytes(), service.policy.MaxPixels(), service.policy.MaxDimension()))
	if err != nil {
		return Result{}, validation(err)
	}
	rawID := make([]byte, 16)
	if _, err = rand.Read(rawID); err != nil {
		return Result{}, stable(err)
	}
	idText := hex.EncodeToString(rawID)
	id, _ := NewArticleImageID(idText)
	owner, _ := NewImageOwnerID(author.String())
	extension := ".jpg"
	if validated.MediaType == MediaTypePNG {
		extension = ".png"
	}
	key, _ := NewStorageKey(idText + extension)
	token, size, err := service.blob.Stage(ctx, idText, &bytesReader{content: validated.Bytes})
	if err != nil {
		return Result{}, stable(err)
	}
	cleanup := func() { _ = service.blob.Discard(context.WithoutCancel(ctx), token) }
	now := service.now()
	image, err := NewArticleImage(ArticleImageMetadata{ID: id, OwnerID: owner, StorageKey: key, MediaType: validated.MediaType, ByteSize: size, Width: validated.Width, Height: validated.Height, SHA256: validated.SHA256}, now, now.Add(service.policy.PendingTTL()))
	if err != nil {
		cleanup()
		return Result{}, stable(err)
	}
	if err = service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return (&Repository{db: tx}).Create(ctx, image)
	}); err != nil {
		cleanup()
		return Result{}, stable(err)
	}
	if err = service.blob.Commit(token, key.String()); err != nil {
		if committed(err) {
			cleanup()
			return result(image), nil
		}
		compensationCtx := context.WithoutCancel(ctx)
		compensateErr := service.db.WithContext(compensationCtx).Transaction(func(tx *gorm.DB) error {
			return (&Repository{db: tx}).DeleteMetadata(compensationCtx, id)
		})
		cleanup()
		if compensateErr != nil {
			return Result{}, stable(fmt.Errorf("提交图片失败且元数据补偿留待未来清理：%w", err))
		}
		return Result{}, stable(err)
	}

	return result(image), nil
}

// Cancel cancels a pending image owned by the current author.
func (service *Service) Cancel(ctx context.Context, command Delete) error {
	if err := service.authorizer.Authorize(ctx, ActionDeleteArticleImage, content.Resource{Kind: "article_image"}); err != nil {
		return permission()
	}
	author, err := service.currentAuthor.CurrentAuthor(ctx)
	if err != nil {
		return permission()
	}
	id, err := NewArticleImageID(command.ID)
	if err != nil {
		return validation(err)
	}
	return stable(service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := &Repository{db: tx}
		images, findErr := repo.FindForUpdate(ctx, []ArticleImageID{id})
		if findErr != nil {
			return findErr
		}
		if len(images) != 1 || images[0].Metadata().OwnerID.String() != author.String() {
			return ErrNotFound
		}
		if cancelErr := images[0].CancelWithGrace(service.now(), service.policy.OrphanGrace()); cancelErr != nil {
			return cancelErr
		}
		return repo.UpdateLifecycle(ctx, images[0])
	}))
}

type bytesReader struct {
	content []byte
	offset  int
}

func (reader *bytesReader) Read(buffer []byte) (int, error) {
	if reader.offset >= len(reader.content) {
		return 0, io.EOF
	}
	count := copy(buffer, reader.content[reader.offset:])
	reader.offset += count
	return count, nil
}

func committed(err error) bool {
	var marker interface{ Committed() bool }
	return errors.As(err, &marker) && marker.Committed()
}

func result(image *ArticleImage) Result {
	metadata := image.Metadata()
	return Result{ID: metadata.ID.String(), StorageKey: metadata.StorageKey.String(), URL: "/media/article-images/" + metadata.StorageKey.String(), MediaType: string(metadata.MediaType), ByteSize: metadata.ByteSize, Width: metadata.Width, Height: metadata.Height, Status: string(image.Status()), ExpiresAt: image.ExpiresAt()}
}
