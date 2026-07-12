package content

import (
	"math"
	"testing"
)

func Test_ContentREST_page_mapping_validates_metadata_consistency(t *testing.T) {
	tests := []struct {
		name         string
		number, size int
		total        int64
		pages, items int
		valid        bool
	}{
		{"zero number", 0, 20, 0, 0, 0, false}, {"zero size", 1, 0, 0, 0, 0, false}, {"oversized size", 1, 101, 0, 0, 0, false},
		{"negative total", 1, 20, -1, 0, 0, false}, {"negative pages", 1, 20, 0, -1, 0, false}, {"inconsistent pages", 1, 20, 21, 1, 20, false},
		{"too many items", 1, 20, 21, 2, 21, false}, {"number overflow", math.MaxInt32 + 1, 20, 0, 0, 0, false},
		{"empty", 1, 20, 0, 0, 0, true}, {"full page", 1, 100, 100, 1, 100, true}, {"last partial", 3, 20, 45, 3, 5, true}, {"beyond end empty", 4, 20, 45, 3, 0, true},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := pageInfo(testCase.number, testCase.size, testCase.total, testCase.pages, testCase.items)
			if (err == nil) != testCase.valid {
				t.Fatalf("pageInfo() error = %v, valid=%v", err, testCase.valid)
			}
		})
	}
}
