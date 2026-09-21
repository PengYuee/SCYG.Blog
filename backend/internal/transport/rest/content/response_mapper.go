package content

import (
	"math"
	"time"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
)

func generatedVersion(version uint64) (generated.Version, error) {
	if version == 0 || version > math.MaxInt64 {
		return 0, responseMappingError()
	}
	return int64(version), nil
}

func responseMappingError() error {
	return newRESTError(codeInternal, nil)
}

func invalidTimes(createdAt, modifiedAt time.Time) bool {
	return createdAt.IsZero() || (!modifiedAt.IsZero() && modifiedAt.Before(createdAt))
}
