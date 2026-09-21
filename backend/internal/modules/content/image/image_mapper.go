package image

import (
	"fmt"
	"time"
)

func imageToModel(value *ArticleImage) imageModel {
	metadata := value.Metadata()
	return imageModel{ID: metadata.ID.String(), OwnerID: metadata.OwnerID.String(), StorageKey: metadata.StorageKey.String(), MediaType: string(metadata.MediaType), ByteSize: metadata.ByteSize, Width: metadata.Width, Height: metadata.Height, SHA256: metadata.SHA256, Status: string(value.Status()), CreatedAt: value.CreatedAt().UTC(), CommittedAt: nullableTime(value.CommittedAt()), OrphanedAt: nullableTime(value.OrphanedAt()), ExpiresAt: value.ExpiresAt().UTC()}
}

func imageFromModel(row imageModel) (*ArticleImage, error) {
	id, err := NewArticleImageID(row.ID)
	if err != nil {
		return nil, err
	}
	owner, err := NewImageOwnerID(row.OwnerID)
	if err != nil {
		return nil, err
	}
	key, err := NewStorageKey(row.StorageKey)
	if err != nil {
		return nil, err
	}
	status, err := NewArticleImageStatus(row.Status)
	if err != nil {
		return nil, err
	}
	media := MediaType(row.MediaType)
	if media != MediaTypeJPEG && media != MediaTypePNG {
		return nil, fmt.Errorf("map image media type: %w", ErrInvalidValue)
	}
	return ReconstituteArticleImage(ArticleImageState{Metadata: ArticleImageMetadata{ID: id, OwnerID: owner, StorageKey: key, MediaType: media, ByteSize: row.ByteSize, Width: row.Width, Height: row.Height, SHA256: row.SHA256}, Status: status, CreatedAt: row.CreatedAt.UTC(), CommittedAt: timeValue(row.CommittedAt, time.Time{}), OrphanedAt: timeValue(row.OrphanedAt, time.Time{}), ExpiresAt: row.ExpiresAt.UTC()})
}

func mapRows(rows []imageModel) ([]*ArticleImage, error) {
	result := make([]*ArticleImage, 0, len(rows))
	for _, row := range rows {
		value, err := imageFromModel(row)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}
