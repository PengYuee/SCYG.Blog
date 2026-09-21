package taxonomy

import (
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/persistence"
)

type tagEntity struct {
	ID        int64
	Name      string
	Version   uint64
	CreatedAt time.Time
	Modified  time.Time
	DeletedAt time.Time
}

func entityFromTag(row tagModel) (tagEntity, error) {
	if err := row.Validate(); err != nil {
		return tagEntity{}, ErrInvalidData
	}
	if err := parseID(row.ID, "tag_id"); err != nil || row.Version <= 0 || row.CreatedAt.IsZero() {
		return tagEntity{}, ErrInvalidData
	}
	name, err := parseName(row.Name)
	if err != nil {
		return tagEntity{}, ErrInvalidData
	}
	return tagEntity{ID: row.ID, Name: name, Version: uint64(row.Version), CreatedAt: row.CreatedAt.UTC(), Modified: row.EffectiveUpdatedAt(), DeletedAt: nullableTimeValue(row.DeletedAt)}, nil
}

func entityFromTagProjection(row tagProjectionRow) (tagEntity, error) {
	if err := parseID(row.ID, "tag_id"); err != nil || row.Version <= 0 || row.CreatedAt.IsZero() {
		return tagEntity{}, ErrInvalidData
	}
	name, err := parseName(row.Name)
	if err != nil {
		return tagEntity{}, ErrInvalidData
	}
	return tagEntity{ID: row.ID, Name: name, Version: uint64(row.Version), CreatedAt: row.CreatedAt.UTC(), Modified: persistence.EffectiveUpdatedAt(row.CreatedAt, row.UpdatedAt)}, nil
}

func tagResult(entity tagEntity) TagResult {
	return TagResult{ID: entity.ID, Name: entity.Name, Version: entity.Version, CreatedAt: entity.CreatedAt, ModifiedAt: entity.Modified}
}
