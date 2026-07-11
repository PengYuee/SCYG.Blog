package content

import (
	"slices"
	"strings"
)

func validTaxonomyPage(page, size int, sort string) error {
	if page < 1 || size < 1 || size > 100 {
		return invalidCommand("page")
	}
	switch sort {
	case "created_at", "-created_at", "title", "-title", "updated_at", "-updated_at":
		return nil
	default:
		return invalidCommand("sort")
	}
}
func pageRange(total, page, size int) (int, int) {
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return start, end
}
func pages(total, size int) int {
	if total == 0 {
		return 0
	}
	return (total + size - 1) / size
}
func sortArticleTypes(items []ArticleTypeResult, sort string) {
	slices.SortStableFunc(items, func(left, right ArticleTypeResult) int {
		switch sort {
		case "-title":
			return -strings.Compare(left.Name, right.Name)
		case "created_at":
			return left.CreatedAt.Compare(right.CreatedAt)
		case "-created_at":
			return right.CreatedAt.Compare(left.CreatedAt)
		case "updated_at":
			return left.ModifiedAt.Compare(right.ModifiedAt)
		case "-updated_at":
			return right.ModifiedAt.Compare(left.ModifiedAt)
		default:
			return strings.Compare(left.Name, right.Name)
		}
	})
}
func sortTags(items []TagResult, sort string) {
	slices.SortStableFunc(items, func(left, right TagResult) int {
		switch sort {
		case "-title":
			return -strings.Compare(left.Name, right.Name)
		case "created_at":
			return left.CreatedAt.Compare(right.CreatedAt)
		case "-created_at":
			return right.CreatedAt.Compare(left.CreatedAt)
		case "updated_at":
			return left.ModifiedAt.Compare(right.ModifiedAt)
		case "-updated_at":
			return right.ModifiedAt.Compare(left.ModifiedAt)
		default:
			return strings.Compare(left.Name, right.Name)
		}
	})
}
