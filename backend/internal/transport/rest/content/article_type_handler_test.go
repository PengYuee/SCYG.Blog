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

func validArticleTypeResult(image string, meun int32) module.ArticleTypeResult {
	now := time.Unix(1, 0).UTC()
	var value *string
	if image != "" {
		value = &image
	}
	return module.ArticleTypeResult{ID: 1, Name: "News", Image: value, Meun: meun, Version: 1, CreatedAt: now, ModifiedAt: now}
}
