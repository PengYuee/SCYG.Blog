package content

import (
	"slices"
	"strings"
)

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
