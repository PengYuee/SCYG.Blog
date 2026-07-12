package content

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
