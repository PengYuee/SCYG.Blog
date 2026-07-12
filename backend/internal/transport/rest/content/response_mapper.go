package content

import (
	"math"
	"time"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

func generatedVersion(version uint64) (generated.Version, error) {
	if version == 0 || version > math.MaxInt64 {
		return 0, responseMappingError()
	}
	return int64(version), nil
}

func responseMappingError() error {
	return &module.ApplicationError{Code: module.CodeInternal, Kind: module.KindInternal, Cause: module.ErrPersistence}
}

func invalidTimes(createdAt, modifiedAt time.Time) bool {
	return createdAt.IsZero() || (!modifiedAt.IsZero() && modifiedAt.Before(createdAt))
}
