package content

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

func entityTag(version uint64) (string, error) {
	if version == 0 || version > math.MaxInt64 {
		return "", responseMappingError()
	}
	return fmt.Sprintf("\"%d\"", version), nil
}

func parseEntityTag(value string) (uint64, error) {
	if strings.HasPrefix(value, "W/") || len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' {
		return 0, fmt.Errorf("强实体标签格式不合法")
	}
	version, err := strconv.ParseUint(value[1:len(value)-1], 10, 64)
	if err != nil || version == 0 {
		return 0, fmt.Errorf("强实体标签格式不合法")
	}
	return version, nil
}

func invalidETag(err error) error {
	return &module.ApplicationError{Code: module.CodeValidation, Kind: module.KindValidation, Cause: err}
}
