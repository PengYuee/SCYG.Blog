package content_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

func Test_ContentREST_list_with_nested_invalid_item_returns_safe_500(t *testing.T) {
	// Given
	valid := validArticleResultForHTTP()
	invalid := valid
	invalid.TagIDs = []int64{0}
	service := &testService{articlePage: module.ArticlePage{Items: []module.ArticleResult{valid, invalid}, Number: 1, Size: 20, TotalItems: 2, TotalPages: 1}}
	router := routerForService(t, service)
	response := httptest.NewRecorder()
	// When
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/articles", nil))
	// Then
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Header().Get("Content-Type"), "application/problem+json") {
		t.Fatalf("status/type/body = %d/%s/%s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"items"`) {
		t.Fatalf("partial list leaked: %s", response.Body.String())
	}
}

func Test_ContentREST_Article_list_preserves_nonzero_counters(t *testing.T) {
	// Given
	item := validArticleResultForHTTP()
	service := &testService{articlePage: module.ArticlePage{Items: []module.ArticleResult{item}, Number: 1, Size: 20, TotalItems: 1, TotalPages: 1}}
	router := routerForService(t, service)
	response := httptest.NewRecorder()

	// When
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/articles", nil))

	// Then
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Items []struct {
			Support int64 `json:"support"`
			Comment int64 `json:"comment"`
			Visited int64 `json:"visited"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Support != 7 || body.Items[0].Comment != 8 || body.Items[0].Visited != 9 {
		t.Fatalf("items = %#v", body.Items)
	}
}

func Test_ContentREST_list_with_invalid_page_returns_safe_500(t *testing.T) {
	// Given
	service := &testService{articlePage: module.ArticlePage{Number: 0, Size: 20, TotalItems: -1, TotalPages: 0}}
	router := routerForService(t, service)
	response := httptest.NewRecorder()

	// When
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/articles", nil))

	// Then
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), `"items"`) {
		t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
	}
}

func Test_ContentREST_list_with_noncanonical_slug_returns_safe_500(t *testing.T) {
	// Given
	item := validArticleResultForHTTP()
	item.Slug = "UPPER"
	service := &testService{articlePage: module.ArticlePage{Items: []module.ArticleResult{item}, Number: 1, Size: 20, TotalItems: 1, TotalPages: 1}}
	router := routerForService(t, service)
	response := httptest.NewRecorder()

	// When
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/articles", nil))

	// Then
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "UPPER") || strings.Contains(response.Body.String(), `"items"`) {
		t.Fatalf("状态码/响应体 = %d/%s", response.Code, response.Body.String())
	}
}

func validArticleResultForHTTP() module.ArticleResult {
	now := time.Unix(1, 0).UTC()
	return module.ArticleResult{ID: 1, ArticleTypeID: 1, Title: "Title", Slug: "title", Digest: "Digest", Content: "Body", Status: "published", TagIDs: []int64{1}, Support: 7, Comment: 8, Visited: 9, Version: 1, CreatedAt: now, ModifiedAt: now}
}
