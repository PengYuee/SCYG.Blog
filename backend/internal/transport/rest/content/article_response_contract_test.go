package content_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
)

func Test_ContentREST_create_draft_response_satisfies_article_contract_without_public_category(t *testing.T) {
	item := validArticleResultForHTTP()
	item.ArticleTypeID, item.Status = 2, "draft"
	image := "https://example.test/category.png"
	item.ArticleTypeName, item.ArticleTypeImage = "Draft category", &image
	service := &testService{allowWrites: true, article: item}
	router := routerForService(t, service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/manage/articles", strings.NewReader(`{"articleTypeId":2,"title":"Draft","slug":"draft","digest":"Draft summary","content":"# Draft\n\nBody","tagIds":[1],"status":1}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("create status/body = %d/%s", response.Code, response.Body.String())
	}
	var body any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	document, err := openapi3.NewLoader().LoadFromFile("../../../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("load authoritative contract: %v", err)
	}
	if err := document.Components.Schemas["Article"].Value.VisitJSON(body); err != nil {
		t.Fatalf("created draft violates Article response contract: %v", err)
	}
}

func Test_ContentREST_rejects_article_with_invalid_category_summary(t *testing.T) {
	item := validArticleResultForHTTP()
	item.ArticleTypeName = ""
	service := &testService{article: item}
	router := routerForService(t, service)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/manage/articles/1", nil))

	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), `"articleType"`) {
		t.Fatalf("invalid category leaked: status/body = %d/%s", response.Code, response.Body.String())
	}
}

func Test_ContentREST_invalid_article_category_returns_safe_500(t *testing.T) {
	for _, path := range []string{"/api/v1/manage/articles/1", "/api/v1/manage/articles", "/api/v1/articles"} {
		t.Run(path, func(t *testing.T) {
			item := validArticleResultForHTTP()
			item.ArticleTypeName = ""
			service := &testService{
				article:     item,
				articlePage: article.Page{Items: []article.Result{item}, Number: 1, Size: 20, TotalItems: 1, TotalPages: 1},
			}
			response := httptest.NewRecorder()

			routerForService(t, service).ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))

			if response.Code != http.StatusInternalServerError || response.Header().Get("Content-Type") != "application/problem+json" {
				t.Fatalf("status/type/body = %d/%s/%s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
			}
			var problem generated.Problem
			if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if problem.Status != http.StatusInternalServerError || problem.Type != "https://scyg.blog/problems/internal" || problem.Detail != "服务器处理请求时发生内部错误" {
				t.Fatalf("unexpected problem: %#v", problem)
			}
			for _, forbidden := range []string{`"items"`, `"articleType"`, item.Title, item.Content} {
				if strings.Contains(response.Body.String(), forbidden) {
					t.Fatalf("internal cause or partial article leaked: %s", response.Body.String())
				}
			}
		})
	}
}
