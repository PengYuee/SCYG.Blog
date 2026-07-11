package content

import (
	"math"
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

func validArticleResult() module.ArticleResult {
	now := time.Unix(1, 0).UTC()
	return module.ArticleResult{ID: 1, ArticleTypeID: 2, Title: "Title", Slug: "title", Digest: "Digest", Content: "Body", Status: "published", TagIDs: []int64{3}, Version: 1, CreatedAt: now, ModifiedAt: now}
}
