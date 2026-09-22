package taxonomy

func validPage(page, size int, sortKey string) error {
	if page < 1 || size < 1 || size > 100 {
		return invalid("page")
	}
	if _, err := taxonomyOrder(sortKey); err != nil {
		return err
	}
	return nil
}

func taxonomyOrder(sortKey string) (string, error) {
	switch sortKey {
	case "createdAt":
		return "created_at ASC, id ASC", nil
	case "-createdAt":
		return "created_at DESC, id DESC", nil
	case "title":
		return "name ASC, id ASC", nil
	case "-title":
		return "name DESC, id DESC", nil
	case "updatedAt":
		return "COALESCE(updated_at, created_at) ASC, id ASC", nil
	case "-updatedAt":
		return "COALESCE(updated_at, created_at) DESC, id DESC", nil
	default:
		return "", invalid("sort")
	}
}

func pageCount(total int64, size int) int {
	if total == 0 {
		return 0
	}
	return int((total + int64(size) - 1) / int64(size))
}
