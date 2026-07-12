package contracttest

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
)

const (
	articleImageIDFixture  = "0123456789abcdef0123456789abcdef"
	articleImageKeyFixture = articleImageIDFixture + ".jpg"
)

// Test_OpenAPI_article_image_contracts_when_spec_loaded 验证正文图片操作及 DTO 的稳定契约。
func Test_OpenAPI_article_image_contracts_when_spec_loaded(t *testing.T) {
	// Given
	document := loadAuthoritativeSpec(t)
	all := operations(document)

	// When / Then
	upload := all[http.MethodPost+" /api/v1/article-images"]
	if upload == nil || upload.OperationID != "createArticleImage" {
		t.Fatal("正文图片上传操作缺失或 operationId 不稳定")
	}
	remove := all[http.MethodDelete+" /api/v1/article-images/{image_id}"]
	if remove == nil || remove.OperationID != "deleteArticleImage" {
		t.Fatal("正文图片删除操作缺失或 operationId 不稳定")
	}
	media := all[http.MethodGet+" /media/article-images/{storage_key}"]
	if media == nil || media.OperationID != "getArticleImageMedia" {
		t.Fatal("正文图片读取操作缺失或 operationId 不稳定")
	}
	assertSchemaRequired(t, document, "ArticleImage", []string{"id", "storageKey", "url", "mediaType", "byteSize", "width", "height", "status", "expiresAt"})
	uploadSchema := upload.RequestBody.Value.Content.Get("multipart/form-data").Schema.Value
	if len(uploadSchema.Properties) != 1 || uploadSchema.Properties["file"] == nil || !slices.Equal(uploadSchema.Required, []string{"file"}) {
		t.Fatal("multipart schema 必须只包含一个必填 file")
	}
	fileSchema := uploadSchema.Properties["file"].Value
	if fileSchema.Format != "binary" || fileSchema.MaxLength == nil || *fileSchema.MaxLength != 5242880 {
		t.Fatal("multipart file 必须是最大 5 MiB 的 binary")
	}
	assertResponseHeader(t, upload, "201", "Location")
	for _, status := range []string{"400", "403", "404", "409"} {
		if response(t, remove, status).Content.Get("application/problem+json") == nil {
			t.Fatalf("删除正文图片响应 %s 不是 RFC 9457", status)
		}
	}
	for _, status := range []string{"200", "304", "404"} {
		response(t, media, status)
	}
	notModified := response(t, media, "304")
	if len(notModified.Content) != 0 {
		t.Fatal("正文图片 304 响应不得声明响应体")
	}
	entityTag := notModified.Headers["ETag"]
	if entityTag == nil || entityTag.Value == nil || !entityTag.Value.Required {
		t.Fatal("正文图片 304 响应必须声明 required ETag")
	}
}

// Test_OpenAPI_article_image_request_validation_when_input_is_invalid 验证缺文件及非法路径参数在 OpenAPI 边界被拒绝。
func Test_OpenAPI_article_image_request_validation_when_input_is_invalid(t *testing.T) {
	// Given
	document := loadAuthoritativeSpec(t)
	router, err := legacy.NewRouter(document)
	if err != nil {
		t.Fatalf("构造 OpenAPI 路由失败：%v", err)
	}
	fixtures := []struct {
		name    string
		request *http.Request
	}{
		{name: "缺少文件", request: multipartRequest(t, false)},
		{name: "非法图片标识", request: httptest.NewRequest(http.MethodDelete, "/api/v1/article-images/not-hex", nil)},
		{name: "非法存储键", request: httptest.NewRequest(http.MethodGet, "/media/article-images/../bad.jpg", nil)},
	}

	// When / Then
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			route, pathParams, routeErr := router.FindRoute(fixture.request)
			if routeErr != nil {
				return
			}
			validationErr := openapi3filter.ValidateRequest(context.Background(), &openapi3filter.RequestValidationInput{Request: fixture.request, PathParams: pathParams, Route: route})
			if validationErr == nil {
				t.Fatal("OpenAPI 请求校验器接受了非法输入")
			}
		})
	}
}

// Test_OpenAPI_article_image_request_validation_when_input_is_valid 验证合法 multipart、图片标识和存储键可通过边界校验。
func Test_OpenAPI_article_image_request_validation_when_input_is_valid(t *testing.T) {
	// Given
	document := loadAuthoritativeSpec(t)
	router, err := legacy.NewRouter(document)
	if err != nil {
		t.Fatalf("构造 OpenAPI 路由失败：%v", err)
	}
	requests := []*http.Request{
		multipartRequest(t, true),
		httptest.NewRequest(http.MethodDelete, "/api/v1/article-images/"+articleImageIDFixture, nil),
		httptest.NewRequest(http.MethodGet, "/media/article-images/"+articleImageKeyFixture, nil),
	}

	// When / Then
	for _, request := range requests {
		route, pathParams, routeErr := router.FindRoute(request)
		if routeErr != nil {
			t.Fatalf("合法请求未匹配路由：%v", routeErr)
		}
		if validationErr := openapi3filter.ValidateRequest(context.Background(), &openapi3filter.RequestValidationInput{Request: request, PathParams: pathParams, Route: route}); validationErr != nil {
			t.Fatalf("OpenAPI 请求校验器拒绝合法输入：%v", validationErr)
		}
	}
}

func multipartRequest(t *testing.T, includeFile bool) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if includeFile {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="file"; filename="image.jpg"`)
		header.Set("Content-Type", "image/jpeg")
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatalf("创建 multipart 文件 part 失败：%v", err)
		}
		if _, err = part.Write([]byte{0xff, 0xd8, 0xff, 0xd9}); err != nil {
			t.Fatalf("写入 multipart 文件 part 失败：%v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭 multipart writer 失败：%v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/article-images", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

// Test_OpenAPI_article_DTOs_when_image_contract_added 验证普通文章 DTO 未被正文图片契约扩展。
func Test_OpenAPI_article_DTOs_when_image_contract_added(t *testing.T) {
	// Given
	document := loadAuthoritativeSpec(t)

	// When / Then
	assertExactProperties(t, document, "Article", []string{"id", "title", "slug", "digest", "content", "article_type_id", "tag_ids", "status", "support", "comment", "visited", "version", "created_at", "updated_at"})
	assertExactProperties(t, document, "ArticleCreate", []string{"title", "slug", "digest", "content", "article_type_id", "tag_ids", "status"})
	assertExactProperties(t, document, "ArticlePatch", []string{"title", "slug", "digest", "content", "article_type_id", "tag_ids", "status"})
}

func assertExactProperties(t *testing.T, document *openapi3.T, name string, expected []string) {
	t.Helper()
	properties := document.Components.Schemas[name].Value.Properties
	actual := make([]string, 0, len(properties))
	for property := range properties {
		actual = append(actual, property)
	}
	slices.Sort(actual)
	slices.Sort(expected)
	if !slices.Equal(actual, expected) {
		t.Fatalf("schema %s 属性变化：got %v want %v", name, actual, expected)
	}
}
