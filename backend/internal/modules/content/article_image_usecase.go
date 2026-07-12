package content

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

type unavailableArticleImageAssets struct{}

func (unavailableArticleImageAssets) StageArticleImage(context.Context, string, ArticleImageContent) (string, int64, error) {
	return "", 0, errors.New("图片存储不可用")
}

func (unavailableArticleImageAssets) CommitArticleImage(string, string) error {
	return errors.New("图片存储不可用")
}

func (unavailableArticleImageAssets) DiscardArticleImage(context.Context, string) error {
	return errors.New("图片存储不可用")
}

func (unavailableArticleImageAssets) LoadArticleImage(string) ([]byte, error) {
	return nil, errors.New("图片存储不可用")
}

func articleImageStagerOrUnavailable(candidate ArticleImageStager) ArticleImageStager {
	if nilLike(candidate) {
		return unavailableArticleImageAssets{}
	}
	return candidate
}

func articleImagePublisherOrUnavailable(candidate ArticleImagePublisher) ArticleImagePublisher {
	if nilLike(candidate) {
		return unavailableArticleImageAssets{}
	}
	return candidate
}

func articleImageDiscarderOrUnavailable(candidate ArticleImageDiscarder) ArticleImageDiscarder {
	if nilLike(candidate) {
		return unavailableArticleImageAssets{}
	}
	return candidate
}

func articleImageLoaderOrUnavailable(candidate ArticleImageLoader) ArticleImageLoader {
	if nilLike(candidate) {
		return unavailableArticleImageAssets{}
	}
	return candidate
}

type articleImageContentReader struct{ source ArticleImageContent }

func (reader articleImageContentReader) Read(buffer []byte) (int, error) {
	return reader.source.ReadArticleImage(buffer)
}

type articleImageBytes struct {
	content []byte
	offset  int
}

func (source *articleImageBytes) ReadArticleImage(buffer []byte) (int, error) {
	if source.offset >= len(source.content) {
		return 0, io.EOF
	}
	count := copy(buffer, source.content[source.offset:])
	source.offset += count
	return count, nil
}

// UploadArticleImage 校验并重新编码图片，再以 temp、数据库、最终文件顺序安全提交。
func (module *Module) UploadArticleImage(ctx context.Context, command UploadArticleImage) (ArticleImageResult, error) {
	if err := module.authorizer.Authorize(ctx, ActionUploadArticleImage, Resource{Kind: "article_image"}); err != nil {
		return ArticleImageResult{}, permission(err)
	}
	author, err := module.currentAuthor.CurrentAuthor(ctx)
	if err != nil {
		return ArticleImageResult{}, permission(err)
	}
	if command.Content == nil {
		return ArticleImageResult{}, validation(errors.New("缺少图片文件"))
	}
	validated, err := domain.ValidateArticleImage(articleImageContentReader{source: command.Content})
	if err != nil {
		return ArticleImageResult{}, validation(err)
	}
	rawID := make([]byte, 16)
	if _, err = rand.Read(rawID); err != nil {
		return ArticleImageResult{}, stable(err)
	}
	idText := hex.EncodeToString(rawID)
	id, _ := domain.NewArticleImageID(idText)
	owner, _ := domain.NewImageOwnerID(author.String())
	extension := ".jpg"
	if validated.MediaType == domain.MediaTypePNG {
		extension = ".png"
	}
	key, _ := domain.NewStorageKey(idText + extension)
	token, size, err := module.imageStager.StageArticleImage(ctx, idText, &articleImageBytes{content: validated.Bytes})
	if err != nil {
		return ArticleImageResult{}, stable(err)
	}
	cleanupTemp := func() { _ = module.imageDiscarder.DiscardArticleImage(context.WithoutCancel(ctx), token) }
	now := module.clock.Now().UTC()
	image, err := domain.NewArticleImage(domain.ArticleImageMetadata{ID: id, OwnerID: owner, StorageKey: key, MediaType: validated.MediaType, ByteSize: size, Width: validated.Width, Height: validated.Height, SHA256: validated.SHA256}, now, now.Add(module.imagePendingTTL))
	if err != nil {
		cleanupTemp()
		return ArticleImageResult{}, stable(err)
	}
	if err = module.unit.Within(ctx, func(txctx context.Context, tx application.Transaction) error {
		return tx.ArticleImages().Save(txctx, image)
	}); err != nil {
		cleanupTemp()
		return ArticleImageResult{}, stable(err)
	}
	if err = module.imagePublisher.CommitArticleImage(token, key.String()); err != nil {
		if application.StorageCommitSucceeded(err) {
			cleanupTemp()
			return imageResult(image), nil
		}
		compensateErr := module.unit.Within(context.WithoutCancel(ctx), func(txctx context.Context, tx application.Transaction) error {
			return tx.ArticleImages().DeleteMetadata(txctx, id)
		})
		cleanupTemp()
		if compensateErr != nil {
			return ArticleImageResult{}, stable(fmt.Errorf("提交图片失败且元数据补偿留待未来清理：%w", err))
		}
		return ArticleImageResult{}, stable(err)
	}
	return imageResult(image), nil
}

// CancelArticleImage 仅允许所有者把 pending 图片置为 orphaned，重复取消幂等。
func (module *Module) CancelArticleImage(ctx context.Context, command DeleteArticleImage) error {
	if err := module.authorizer.Authorize(ctx, ActionDeleteArticleImage, Resource{Kind: "article_image"}); err != nil {
		return permission(err)
	}
	author, err := module.currentAuthor.CurrentAuthor(ctx)
	if err != nil {
		return permission(err)
	}
	id, err := domain.NewArticleImageID(command.ID)
	if err != nil {
		return validation(err)
	}
	return stable(module.unit.Within(ctx, func(txctx context.Context, tx application.Transaction) error {
		images, findErr := tx.ArticleImages().FindForUpdate(txctx, []domain.ArticleImageID{id})
		if findErr != nil {
			return findErr
		}
		if len(images) != 1 || images[0].Metadata().OwnerID.String() != author.String() {
			return stable(ErrNotFound)
		}
		if cancelErr := images[0].Cancel(module.clock.Now().UTC()); cancelErr != nil {
			return stable(cancelErr)
		}
		return tx.ArticleImages().Save(txctx, images[0])
	}))
}

// GetArticleImageMedia 公开 committed 图片，并仅向所有者返回 pending 图片。
func (module *Module) GetArticleImageMedia(ctx context.Context, query GetArticleImage) (ArticleImageMedia, error) {
	key, err := domain.NewStorageKey(query.StorageKey)
	if err != nil {
		return ArticleImageMedia{}, validation(err)
	}
	var image *domain.ArticleImage
	err = module.unit.Within(ctx, func(txctx context.Context, tx application.Transaction) error {
		var findErr error
		image, findErr = tx.ArticleImages().FindByStorageKey(txctx, key)
		return findErr
	})
	if err != nil {
		return ArticleImageMedia{}, stable(err)
	}
	if image.Status() == domain.ArticleImageStatusOrphaned {
		return ArticleImageMedia{}, stable(ErrNotFound)
	}
	pending := image.Status() == domain.ArticleImageStatusPending
	if pending {
		author, authorErr := module.currentAuthor.CurrentAuthor(ctx)
		if authorErr != nil || author.String() != image.Metadata().OwnerID.String() {
			return ArticleImageMedia{}, stable(ErrNotFound)
		}
	}
	content, err := module.imageLoader.LoadArticleImage(key.String())
	if err != nil {
		return ArticleImageMedia{}, stable(ErrNotFound)
	}
	metadata := image.Metadata()
	if int64(len(content)) != metadata.ByteSize {
		return ArticleImageMedia{}, stable(errors.New("图片文件大小与元数据不一致"))
	}
	return ArticleImageMedia{Content: content, MediaType: string(metadata.MediaType), ByteSize: metadata.ByteSize, SHA256: metadata.SHA256, Pending: pending}, nil
}

func imageResult(image *domain.ArticleImage) ArticleImageResult {
	metadata := image.Metadata()
	return ArticleImageResult{ID: metadata.ID.String(), StorageKey: metadata.StorageKey.String(), URL: "/media/article-images/" + metadata.StorageKey.String(), MediaType: string(metadata.MediaType), ByteSize: metadata.ByteSize, Width: metadata.Width, Height: metadata.Height, Status: string(image.Status()), ExpiresAt: image.ExpiresAt()}
}
