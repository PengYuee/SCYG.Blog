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

func Test_ContentREST_ArticleType_create_preserves_image_and_meun(t *testing.T) {
	// Given
	image := "hero.png"
	service := &testService{allowWrites: true, articleType: validArticleTypeResult(image, 7)}
	router := routerForService(t, service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/article-types", strings.NewReader(`{"name":"News","image":"hero.png","meun":7}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	// When
	router.ServeHTTP(response, request)

	// Then
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if service.lastTypeCreate.Image == nil || *service.lastTypeCreate.Image != image || service.lastTypeCreate.Meun != 7 {
		t.Fatalf("command = %#v", service.lastTypeCreate)
	}
	var body struct {
		Image *string `json:"image"`
		Meun  int32   `json:"meun"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Image == nil || *body.Image != image || body.Meun != 7 {
		t.Fatalf("body = %#v", body)
	}
}

func Test_ContentREST_ArticleType_patch_preserves_optional_fields(t *testing.T) {
	tests := []struct {
		name, body string
		assert     func(*testing.T, module.PatchArticleType)
	}{
		{"image only", `{"image":"next.png"}`, func(t *testing.T, command module.PatchArticleType) {
			if !command.Image.Provided || command.Image.Value == nil || *command.Image.Value != "next.png" || command.Name != nil || command.Meun != nil {
				t.Fatalf("command = %#v", command)
			}
		}},
		{"meun zero", `{"meun":0}`, func(t *testing.T, command module.PatchArticleType) {
			if command.Meun == nil || *command.Meun != 0 || command.Image.Provided {
				t.Fatalf("command = %#v", command)
			}
		}},
		{"both", `{"image":"next.png","meun":8}`, func(t *testing.T, command module.PatchArticleType) {
			if !command.Image.Provided || command.Image.Value == nil || command.Meun == nil || *command.Meun != 8 {
				t.Fatalf("command = %#v", command)
			}
		}},
		{"clear image", `{"image":null}`, func(t *testing.T, command module.PatchArticleType) {
			if !command.Image.Provided || command.Image.Value != nil {
				t.Fatalf("command = %#v", command)
			}
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// Given
			service := &testService{allowWrites: true, articleType: validArticleTypeResult("", 0)}
			router := routerForService(t, service)
			request := httptest.NewRequest(http.MethodPatch, "/api/v1/article-types/1", strings.NewReader(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("If-Match", `"1"`)
			response := httptest.NewRecorder()
			// When
			router.ServeHTTP(response, request)
			// Then
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			testCase.assert(t, service.lastTypePatch)
		})
	}
}

func Test_ContentREST_ArticleType_empty_patch_returns_400(t *testing.T) {
	// Given
	service := &testService{allowWrites: true}
	router := routerForService(t, service)
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/article-types/1", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", `"1"`)
	response := httptest.NewRecorder()
	// When
	router.ServeHTTP(response, request)
	// Then
	if response.Code != http.StatusBadRequest || service.writeCalls != 0 {
		t.Fatalf("status/calls = %d/%d", response.Code, service.writeCalls)
	}
}

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

func validArticleTypeResult(image string, meun int32) module.ArticleTypeResult {
	now := time.Unix(1, 0).UTC()
	var value *string
	if image != "" {
		value = &image
	}
	return module.ArticleTypeResult{ID: 1, Name: "News", Image: value, Meun: meun, Version: 1, CreatedAt: now, ModifiedAt: now}
}
func validArticleResultForHTTP() module.ArticleResult {
	now := time.Unix(1, 0).UTC()
	return module.ArticleResult{ID: 1, ArticleTypeID: 1, Title: "Title", Slug: "title", Digest: "Digest", Content: "Body", Status: "published", TagIDs: []int64{1}, Support: 7, Comment: 8, Visited: 9, Version: 1, CreatedAt: now, ModifiedAt: now}
}
