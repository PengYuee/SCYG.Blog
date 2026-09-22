package article

import "testing"

func TestProjectionOrder_usesOpenAPISortValues(t *testing.T) {
	tests := map[string]string{
		"-createdAt": "a.created_at DESC, a.id DESC",
		"createdAt":  "a.created_at ASC, a.id ASC",
		"updatedAt":  "COALESCE(a.updated_at, a.created_at) ASC, a.id ASC",
		"-updatedAt": "COALESCE(a.updated_at, a.created_at) DESC, a.id DESC",
		"title":      "a.title ASC, a.id ASC",
		"-title":     "a.title DESC, a.id DESC",
	}
	for sortKey, want := range tests {
		t.Run(sortKey, func(t *testing.T) {
			got, err := projectionOrder(sortKey)
			if err != nil || got != want {
				t.Fatalf("projectionOrder(%q) = %q, %v; want %q", sortKey, got, err, want)
			}
		})
	}
	for _, sortKey := range []string{"newest", "oldest", "created_at", "-created_at", "updated_at", "-updated_at", "title_desc"} {
		if _, err := projectionOrder(sortKey); err == nil {
			t.Fatalf("projectionOrder(%q) accepted a non-contract sort key", sortKey)
		}
	}
}
