package content_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/image"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
	restcontent "github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest/content"
)

type testFailure string

func (failure testFailure) Error() string      { return string(failure) }
func (failure testFailure) StableCode() string { return string(failure) }

type testLoginHandler struct{}

func (testLoginHandler) Login(context.Context, generated.LoginRequestObject) (generated.LoginResponseObject, error) {
	return generated.Login200JSONResponse{AccessToken: "test-token", TokenType: generated.Bearer}, nil
}

const permissionDenied testFailure = "permission_denied"

type testService struct {
	writeCalls            int
	allowWrites           bool
	articlePage           article.Page
	article               article.Result
	articleType           taxonomy.ArticleTypeResult
	publicArticleTypePage taxonomy.PublicArticleTypePage
	publicTagPage         taxonomy.PublicTagPage
	lastArticleCreate     article.Create
	lastTypeCreate        taxonomy.CreateArticleType
	lastTypePatch         taxonomy.PatchArticleType
}

func (*testService) Get(context.Context, article.Get) (article.Result, error) {
	return article.Result{}, errors.New("not found")
}

func (service *testService) List(context.Context, article.List) (article.Page, error) {
	return service.articlePage, nil
}

func (service *testService) GetManage(context.Context, article.Get) (article.Result, error) {
	return service.article, nil
}

func (service *testService) ListManage(context.Context, article.List) (article.Page, error) {
	return service.articlePage, nil
}

func (*testService) GetArticleType(context.Context, taxonomy.GetArticleType) (taxonomy.ArticleTypeResult, error) {
	return taxonomy.ArticleTypeResult{}, errors.New("not found")
}

func (*testService) ListArticleTypes(context.Context, taxonomy.ListArticleTypes) (taxonomy.ArticleTypePage, error) {
	return taxonomy.ArticleTypePage{}, nil
}

func (*testService) GetPublicArticleType(context.Context, taxonomy.GetPublicArticleType) (taxonomy.PublicArticleTypeResult, error) {
	return taxonomy.PublicArticleTypeResult{ID: 1, Name: "News", ArticleCount: 1, Version: 1}, nil
}

func (service *testService) ListPublicArticleTypes(context.Context, taxonomy.ListPublicArticleTypes) (taxonomy.PublicArticleTypePage, error) {
	return service.publicArticleTypePage, nil
}

func (*testService) GetTag(context.Context, taxonomy.GetTag) (taxonomy.TagResult, error) {
	return taxonomy.TagResult{}, errors.New("not found")
}

func (*testService) ListTags(context.Context, taxonomy.ListTags) (taxonomy.TagPage, error) {
	return taxonomy.TagPage{}, nil
}

func (*testService) GetPublicTag(context.Context, taxonomy.GetPublicTag) (taxonomy.PublicTagResult, error) {
	return taxonomy.PublicTagResult{ID: 1, Name: "Go", ArticleCount: 1, Version: 1}, nil
}

func (service *testService) ListPublicTags(context.Context, taxonomy.ListPublicTags) (taxonomy.PublicTagPage, error) {
	return service.publicTagPage, nil
}

func (service *testService) denied() error {
	service.writeCalls++
	return permissionDenied
}

func (service *testService) Create(_ context.Context, command article.Create) (article.Result, error) {
	service.writeCalls++
	service.lastArticleCreate = command
	if service.allowWrites {
		return service.article, nil
	}
	return article.Result{}, permissionDenied
}

func (service *testService) Patch(context.Context, article.Patch) (article.Result, error) {
	return article.Result{}, service.denied()
}

func (service *testService) Publish(context.Context, article.Publish) (article.Result, error) {
	if err := service.denied(); err != nil {
		return article.Result{}, err
	}
	return service.article, nil
}

func (service *testService) Archive(context.Context, article.Archive) (article.Result, error) {
	if err := service.denied(); err != nil {
		return article.Result{}, err
	}
	return service.article, nil
}

func (service *testService) Delete(context.Context, article.Delete) error { return service.denied() }

func (service *testService) CreateArticleType(_ context.Context, command taxonomy.CreateArticleType) (taxonomy.ArticleTypeResult, error) {
	service.writeCalls++
	service.lastTypeCreate = command
	if service.allowWrites {
		return service.articleType, nil
	}
	return taxonomy.ArticleTypeResult{}, permissionDenied
}

func (service *testService) PatchArticleType(_ context.Context, command taxonomy.PatchArticleType) (taxonomy.ArticleTypeResult, error) {
	service.writeCalls++
	service.lastTypePatch = command
	if service.allowWrites {
		return service.articleType, nil
	}
	return taxonomy.ArticleTypeResult{}, permissionDenied
}

func (service *testService) DeleteArticleType(context.Context, taxonomy.DeleteArticleType) error {
	return service.denied()
}

func (service *testService) CreateTag(context.Context, taxonomy.CreateTag) (taxonomy.TagResult, error) {
	return taxonomy.TagResult{}, service.denied()
}

func (service *testService) RenameTag(context.Context, taxonomy.RenameTag) (taxonomy.TagResult, error) {
	return taxonomy.TagResult{}, service.denied()
}

func (service *testService) DeleteTag(context.Context, taxonomy.DeleteTag) error {
	return service.denied()
}

func (*testService) Upload(context.Context, image.Upload) (image.Result, error) {
	return image.Result{}, nil
}
func (*testService) Cancel(context.Context, image.Delete) error { return nil }
func (*testService) GetMedia(context.Context, image.Get) (image.Media, error) {
	return image.Media{}, nil
}

func Test_ContentREST_missing_If_Match_returns_RFC9457_428(t *testing.T) {
	// Given
	router, service := testRouter(t)
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/manage/articles/1?secret=hidden", nil)
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
		RequestID string              `json:"requestId"`
		Errors    map[string][]string `json:"errors"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Instance != "/api/v1/manage/articles/1" || strings.Contains(problem.Instance, "secret") {
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
	body := `{"articleTypeId":1,"title":"Title","slug":"title","digest":"Digest","content":"Body","tagIds":[1],"status":1}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/manage/articles", strings.NewReader(body))
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

func Test_ContentREST_NewHandler_rejects_typed_nil_services(t *testing.T) {
	// Given
	var typedNil *testService
	nonNil := &testService{}

	// When
	_, queryErr := restcontent.NewHandler(typedNil, nonNil, nonNil, nonNil, nonNil, image.DefaultPolicy())
	_, commandErr := restcontent.NewHandler(nonNil, typedNil, nonNil, nonNil, nonNil, image.DefaultPolicy())

	// Then
	if queryErr == nil || commandErr == nil {
		t.Fatalf("typed nil errors = (%v, %v), want both non-nil", queryErr, commandErr)
	}
}

func Test_ContentREST_registersInjectedLoginHandler(t *testing.T) {
	router, _ := testRouter(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body generated.LoginResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.AccessToken != "test-token" || body.TokenType != generated.Bearer {
		t.Fatalf("login response=%#v", body)
	}
}

func testRouter(t *testing.T) (*gin.Engine, *testService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service := &testService{}
	return routerForService(t, service), service
}

func routerForService(t *testing.T, service *testService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler, err := restcontent.NewHandler(service, service, service, service, service, image.DefaultPolicy())
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	router := gin.New()
	if err = handler.Register(router, testLoginHandler{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return router
}
