package content_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
	restcontent "github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest/content"
)

type testService struct{ writeCalls int }

func (*testService) GetArticle(context.Context, module.GetArticle) (module.ArticleResult, error) {
	return module.ArticleResult{}, module.ErrNotFound
}
func (*testService) ListArticles(context.Context, module.ListArticles) (module.ArticlePage, error) {
	return module.ArticlePage{}, nil
}
func (*testService) GetArticleType(context.Context, module.GetArticleType) (module.ArticleTypeResult, error) {
	return module.ArticleTypeResult{}, module.ErrNotFound
}
func (*testService) ListArticleTypes(context.Context, module.ListArticleTypes) (module.ArticleTypePage, error) {
	return module.ArticleTypePage{}, nil
}
func (*testService) GetTag(context.Context, module.GetTag) (module.TagResult, error) {
	return module.TagResult{}, module.ErrNotFound
}
func (*testService) ListTags(context.Context, module.ListTags) (module.TagPage, error) {
	return module.TagPage{}, nil
}
func (service *testService) denied() error {
	service.writeCalls++
	return &module.ApplicationError{Code: module.CodePermissionDenied, Kind: module.KindPermission, Cause: module.ErrPermissionDenied}
}
func (service *testService) CreateArticle(context.Context, module.CreateArticle) (module.ArticleResult, error) {
	return module.ArticleResult{}, service.denied()
}
func (service *testService) PatchArticle(context.Context, module.PatchArticle) (module.ArticleResult, error) {
	return module.ArticleResult{}, service.denied()
}
func (service *testService) DeleteArticle(context.Context, module.DeleteArticle) error {
	return service.denied()
}
func (service *testService) CreateArticleType(context.Context, module.CreateArticleType) (module.ArticleTypeResult, error) {
	return module.ArticleTypeResult{}, service.denied()
}
func (service *testService) RenameArticleType(context.Context, module.RenameArticleType) (module.ArticleTypeResult, error) {
	return module.ArticleTypeResult{}, service.denied()
}
func (service *testService) DeleteArticleType(context.Context, module.DeleteArticleType) error {
	return service.denied()
}
func (service *testService) CreateTag(context.Context, module.CreateTag) (module.TagResult, error) {
	return module.TagResult{}, service.denied()
}
func (service *testService) RenameTag(context.Context, module.RenameTag) (module.TagResult, error) {
	return module.TagResult{}, service.denied()
}
func (service *testService) DeleteTag(context.Context, module.DeleteTag) error {
	return service.denied()
}

func Test_ContentREST_missing_If_Match_returns_RFC9457_428(t *testing.T) {
	// Given
	router, service := testRouter(t)
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/articles/1?secret=hidden", nil)
	request = request.WithContext(observability.WithRequestFields(request.Context(), observability.RequestFields{RequestID: "req-10"}))
	response := httptest.NewRecorder()

	// When
	router.ServeHTTP(response, request)

	// Then
	if response.Code != http.StatusPreconditionRequired {
		t.Fatalf("status = %d, want 428", response.Code)
	}
	if response.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("content type = %q", response.Header().Get("Content-Type"))
	}
	var problem struct {
		Instance  string              `json:"instance"`
		RequestID string              `json:"request_id"`
		Errors    map[string][]string `json:"errors"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Instance != "/api/v1/articles/1" || strings.Contains(problem.Instance, "secret") {
		t.Fatalf("instance = %q", problem.Instance)
	}
	if problem.RequestID != "req-10" || problem.Errors == nil {
		t.Fatalf("problem = %#v", problem)
	}
	if service.writeCalls != 0 {
		t.Fatalf("command calls = %d, want 0", service.writeCalls)
	}
}

func Test_ContentREST_default_DenyAll_returns_403_without_persistence(t *testing.T) {
	// Given
	router, service := testRouter(t)
	body := `{"article_type_id":1,"title":"Title","slug":"title","digest":"Digest","content":"Body","tag_ids":[1],"status":1}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/articles", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	// When
	router.ServeHTTP(response, request)

	// Then
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if service.writeCalls != 1 {
		t.Fatalf("command calls = %d, want 1", service.writeCalls)
	}
}

func testRouter(t *testing.T) (*gin.Engine, *testService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service := &testService{}
	handler, err := restcontent.NewHandler(service, service)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	router := gin.New()
	if err = handler.Register(router); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return router, service
}
