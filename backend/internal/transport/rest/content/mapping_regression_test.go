package content

import (
	"math"
	"strings"
	"testing"
	"time"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

func Test_ContentREST_response_mapping_rejects_invalid_article_identity(t *testing.T) {
	// Given
	item := validArticleResult()
	item.ID = 0

	// When
	_, err := articleDTO(item)

	// Then
	if err == nil {
		t.Fatal("articleDTO() error = nil")
	}
}

func Test_ContentREST_response_mapping_rejects_overflow_version(t *testing.T) {
	// Given
	item := validArticleResult()
	item.Version = math.MaxUint64

	// When
	_, err := articleDTO(item)

	// Then
	if err == nil {
		t.Fatal("articleDTO() error = nil")
	}
}

func Test_ContentREST_response_mapping_preserves_article_counters(t *testing.T) {
	// Given
	item := validArticleResult()
	item.Support, item.Comment, item.Visited = 7, 8, 9

	// When
	dto, err := articleDTO(item)

	// Then
	if err != nil {
		t.Fatalf("articleDTO() error = %v", err)
	}
	if dto.Support != 7 || dto.Comment != 8 || dto.Visited != 9 {
		t.Fatalf("counters = %d/%d/%d", dto.Support, dto.Comment, dto.Visited)
	}
}

func Test_ContentREST_response_mapping_accepts_max_contract_identity_and_version(t *testing.T) {
	// Given
	item := validArticleResult()
	item.ID, item.ArticleTypeID, item.TagIDs, item.Version = math.MaxInt64, math.MaxInt64, []int64{math.MaxInt64}, math.MaxInt64

	// When
	dto, err := articleDTO(item)
	etag, tagErr := entityTag(item.Version)

	// Then
	if err != nil || tagErr != nil {
		t.Fatalf("mapping errors = %v / %v", err, tagErr)
	}
	if dto.ID != math.MaxInt64 || dto.Version != math.MaxInt64 || etag != `"9223372036854775807"` {
		t.Fatalf("dto/etag = %#v / %q", dto, etag)
	}
}

func Test_ContentREST_constructor_nil_detection_covers_nilable_kinds(t *testing.T) {
	// Given
	var pointer *testNilValue
	var mapping map[string]string
	var values []string
	var function func()
	var channel chan string
	tests := []any{pointer, mapping, values, function, channel}

	// When / Then
	for _, value := range tests {
		if !nilService(value) {
			t.Fatalf("nilService(%T) = false", value)
		}
	}
	if nilService(testNilValue{}) {
		t.Fatal("nilService(non-nil value) = true")
	}
}

type testNilValue struct{}

func Test_ContentREST_response_mapping_rejects_invalid_article_text(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*module.ArticleResult)
	}{
		{"empty title", func(item *module.ArticleResult) { item.Title = "" }},
		{"whitespace title", func(item *module.ArticleResult) { item.Title = " \t" }},
		{"long title", func(item *module.ArticleResult) { item.Title = strings.Repeat("a", 121) }},
		{"control title", func(item *module.ArticleResult) { item.Title = "bad\x00title" }},
		{"invalid utf8 title", func(item *module.ArticleResult) { item.Title = string([]byte{0xff}) }},
		{"illegal slug", func(item *module.ArticleResult) { item.Slug = "Bad Slug" }},
		{"empty digest", func(item *module.ArticleResult) { item.Digest = "" }},
		{"empty content", func(item *module.ArticleResult) { item.Content = "" }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			item := validArticleResult()
			testCase.mutate(&item)
			if _, err := articleDTO(item); err == nil {
				t.Fatalf("articleDTO(%s) error = nil", testCase.name)
			}
		})
	}
}

func Test_ContentREST_response_mapping_rejects_noncanonical_slug(t *testing.T) {
	tests := []string{"UPPER", " Mixed ", " leading", "trailing ", "Go-Lang", "ＦＯＯ"}
	for _, slug := range tests {
		t.Run(slug, func(t *testing.T) {
			// Given
			item := validArticleResult()
			item.Slug = slug

			// When
			_, err := articleDTO(item)

			// Then
			if err == nil {
				t.Fatalf("articleDTO(%q) 未拒绝非规范 slug", slug)
			}
		})
	}
}

func Test_ContentREST_response_mapping_accepts_canonical_slug_boundaries(t *testing.T) {
	tests := []string{"a", strings.Repeat("a", 160), "a-b-0"}
	for _, slug := range tests {
		t.Run(slug[:1], func(t *testing.T) {
			item := validArticleResult()
			item.Slug = slug
			if _, err := articleDTO(item); err != nil {
				t.Fatalf("articleDTO(%q) 错误 = %v", slug, err)
			}
		})
	}
}

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

func validArticleResult() module.ArticleResult {
	now := time.Unix(1, 0).UTC()
	return module.ArticleResult{ID: 1, ArticleTypeID: 2, Title: "Title", Slug: "title", Digest: "Digest", Content: "Body", Status: "published", TagIDs: []int64{3}, Version: 1, CreatedAt: now, ModifiedAt: now}
}
