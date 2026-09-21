package content_test

import (
	"context"
	"testing"
	"time"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/image"
	restcontent "github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest/content"
)

func Test_ContentREST_CreateManageArticle_maps_supported_statuses_and_returns_created_metadata(t *testing.T) {
	cases := []struct {
		name           string
		requestStatus  generated.ArticleCreateStatus
		commandStatus  article.ArticleCreationStatus
		responseStatus generated.ArticleStatus
		resultStatus   string
	}{
		{"草稿", generated.ArticleCreateStatusDraft, article.ArticleCreationStatusDraft, generated.Draft, "draft"},
		{"直接发布", generated.ArticleCreateStatusPublished, article.ArticleCreationStatusPublished, generated.Published, "published"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// Given
			createdAt := time.Date(2026, 7, 15, 1, 0, 0, 0, time.UTC)
			service := &testService{allowWrites: true, article: article.Result{ID: 7, ArticleTypeID: 1, Title: "标题", Slug: "title", Digest: "摘要", Content: "正文", Status: testCase.resultStatus, TagIDs: []int64{1}, Version: 1, CreatedAt: createdAt}}
			handler, err := restcontent.NewHandler(service, service, service, service, service, image.DefaultPolicy())
			if err != nil {
				t.Fatalf("创建 REST 处理器失败：%v", err)
			}
			request := generated.CreateManageArticleRequestObject{Body: &generated.ArticleCreate{ArticleTypeID: 1, Title: "标题", Slug: "title", Digest: "摘要", Content: "正文", TagIds: []int64{1}, Status: testCase.requestStatus}}

			// When
			response, createErr := handler.CreateManageArticle(context.Background(), request)

			// Then
			if createErr != nil {
				t.Fatalf("创建文章失败：%v", createErr)
			}
			created, ok := response.(generated.CreateManageArticle201JSONResponse)
			if !ok {
				t.Fatalf("创建响应类型=%T", response)
			}
			if service.lastArticleCreate.Status != testCase.commandStatus || created.Body.Status != testCase.responseStatus {
				t.Fatalf("状态映射错误：command=%d response=%d", service.lastArticleCreate.Status, created.Body.Status)
			}
			if created.Body.Version != 1 || created.Headers.ETag != `"1"` || created.Headers.Location != "/api/v1/manage/articles/7" {
				t.Fatalf("创建元数据错误：body=%#v headers=%#v", created.Body, created.Headers)
			}
		})
	}
}

func Test_ContentREST_CreateManageArticle_rejects_forged_archived_status_without_calling_service(t *testing.T) {
	// Given
	service := &testService{allowWrites: true}
	handler, err := restcontent.NewHandler(service, service, service, service, service, image.DefaultPolicy())
	if err != nil {
		t.Fatalf("创建 REST 处理器失败：%v", err)
	}
	request := generated.CreateManageArticleRequestObject{Body: &generated.ArticleCreate{ArticleTypeID: 1, Title: "标题", Slug: "title", Digest: "摘要", Content: "正文", TagIds: []int64{1}, Status: generated.ArticleCreateStatus(3)}}

	// When
	_, createErr := handler.CreateManageArticle(context.Background(), request)

	// Then
	if createErr == nil || service.writeCalls != 0 {
		t.Fatalf("伪造归档状态未在 REST 映射层拒绝：error=%v calls=%d", createErr, service.writeCalls)
	}
}
