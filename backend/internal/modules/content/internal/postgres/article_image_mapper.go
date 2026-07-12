package postgres

import (
	"fmt"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
	"time"
)

func articleImageToModel(image *domain.ArticleImage) articleImageModel {
	metadata := image.Metadata()
	return articleImageModel{ID: metadata.ID.String(), OwnerID: metadata.OwnerID.String(), StorageKey: metadata.StorageKey.String(), MediaType: string(metadata.MediaType), ByteSize: metadata.ByteSize, Width: metadata.Width, Height: metadata.Height, SHA256: metadata.SHA256, Status: string(image.Status()), CreatedAt: image.CreatedAt().UTC(), CommittedAt: nullableTime(image.CommittedAt()), OrphanedAt: nullableTime(image.OrphanedAt()), ExpiresAt: image.ExpiresAt().UTC()}
}
func articleImageFromModel(row articleImageModel) (*domain.ArticleImage, error) {
	id, err := domain.NewArticleImageID(row.ID)
	if err != nil {
		return nil, err
	}
	owner, err := domain.NewImageOwnerID(row.OwnerID)
	if err != nil {
		return nil, err
	}
	key, err := domain.NewStorageKey(row.StorageKey)
	if err != nil {
		return nil, err
	}
	status, err := domain.NewArticleImageStatus(row.Status)
	if err != nil {
		return nil, err
	}
	media := domain.MediaType(row.MediaType)
	if media != domain.MediaTypeJPEG && media != domain.MediaTypePNG {
		return nil, fmt.Errorf("map image media type: %w", domain.ErrInvalidValue)
	}
	return domain.ReconstituteArticleImage(domain.ArticleImageState{Metadata: domain.ArticleImageMetadata{ID: id, OwnerID: owner, StorageKey: key, MediaType: media, ByteSize: row.ByteSize, Width: row.Width, Height: row.Height, SHA256: row.SHA256}, Status: status, CreatedAt: row.CreatedAt.UTC(), CommittedAt: timeValue(row.CommittedAt, time.Time{}), OrphanedAt: timeValue(row.OrphanedAt, time.Time{}), ExpiresAt: row.ExpiresAt.UTC()})
}
