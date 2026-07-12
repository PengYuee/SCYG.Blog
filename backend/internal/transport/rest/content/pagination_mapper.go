package content

import (
	"math"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
)

func pageInfo(number, size int, totalItems int64, totalPages, itemCount int) (generated.PageInfo, error) {
	if number < 1 || number > math.MaxInt32 || size < 1 || size > 100 || totalItems < 0 || totalPages < 0 || itemCount < 0 || itemCount > size {
		return generated.PageInfo{}, responseMappingError()
	}
	expectedPages := int64(0)
	if totalItems > 0 {
		expectedPages = totalItems / int64(size)
		if totalItems%int64(size) != 0 {
			expectedPages++
		}
	}
	if int64(totalPages) != expectedPages {
		return generated.PageInfo{}, responseMappingError()
	}
	expectedItems := int64(0)
	start := int64(number-1) * int64(size)
	if start < totalItems {
		expectedItems = totalItems - start
		if expectedItems > int64(size) {
			expectedItems = int64(size)
		}
	}
	if int64(itemCount) != expectedItems {
		return generated.PageInfo{}, responseMappingError()
	}
	return generated.PageInfo{Number: int32(number), Size: int32(size), TotalItems: totalItems, TotalPages: int64(totalPages)}, nil
}

func pageValues(page *generated.Page, size *generated.PageSize) (int, int) {
	number, pageSize := 1, 20
	if page != nil {
		number = int(*page)
	}
	if size != nil {
		pageSize = int(*size)
	}
	return number, pageSize
}
