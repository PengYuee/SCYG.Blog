package contracttest

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// hanText 用于确认正文图片合同保留中文说明。
var hanText = regexp.MustCompile(`\p{Han}`)

// Test_OpenAPI_article_image_documentation_when_spec_loaded 验证正文图片文档完整且不约束其他 API 的语言。
func Test_OpenAPI_article_image_documentation_when_spec_loaded(t *testing.T) {
	// Given
	document := loadAuthoritativeSpec(t)
	all := operations(document)
	upload := all[http.MethodPost+" /api/v1/article-images"]
	remove := all[http.MethodDelete+" /api/v1/article-images/{image_id}"]
	media := all[http.MethodGet+" /media/article-images/{storage_key}"]
	checkChinese := func(label, text string) {
		t.Helper()
		if strings.TrimSpace(text) == "" || !hanText.MatchString(text) {
			t.Fatalf("%s 缺少中文说明", label)
		}
	}

	// When / Then
	for label, operation := range map[string]struct {
		summary     string
		description string
	}{
		"正文图片上传": {summary: upload.Summary, description: upload.Description},
		"正文图片取消": {summary: remove.Summary, description: remove.Description},
		"正文图片读取": {summary: media.Summary, description: media.Description},
	} {
		checkChinese(label+"摘要", operation.summary)
		checkChinese(label+"说明", operation.description)
	}
	checkChinese("正文图片标签", document.Tags.Get("ArticleImages").Description)
	checkChinese("上传请求体", upload.RequestBody.Value.Description)
	checkChinese("上传文件", upload.RequestBody.Value.Content.Get("multipart/form-data").Schema.Value.Properties["file"].Value.Description)
	for _, name := range []string{"ArticleImageID", "ArticleImageStorageKey"} {
		checkChinese("图片参数 "+name, document.Components.Parameters[name].Value.Description)
	}
	for _, name := range []string{"ArticleImageID", "ArticleImageStorageKey", "ArticleImageMediaType", "ArticleImageStatus", "ArticleImage"} {
		checkChinese("图片 schema "+name, document.Components.Schemas[name].Value.Description)
	}
	for label, description := range map[string]*string{
		"上传成功响应": response(t, upload, "201").Description,
		"取消成功响应": response(t, remove, "204").Description,
		"读取成功响应": response(t, media, "200").Description,
		"缓存命中响应": response(t, media, "304").Description,
	} {
		if description == nil {
			t.Fatalf("%s 缺少说明", label)
		}
		checkChinese(label, *description)
	}
}
