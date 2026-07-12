//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

// writeCase 描述一个在生产 DenyAll 下必须以403拒绝的合法 REST 写请求。
type writeCase struct {
	Name, Method, Path, Body string
	Headers                  map[string]string
}

// seedResource 保存资源 Location 与强 ETag，确保 PATCH/DELETE 通过契约验证。
type seedResource struct {
	Location, ETag string
	ID             int64
}

// newHarnessWithDatabase 在同一随机数据库上重新组合生产 DenyAll 应用。
func newHarnessWithDatabase(t *testing.T, authorizer content.Authorizer, seed *harness) *harness {
	t.Helper()
	h := &harness{t: t, ctx: seed.ctx, cancel: seed.cancel, adminDSN: seed.adminDSN, dsn: seed.dsn, name: seed.name, client: &http.Client{Timeout: seed.client.Timeout, Transport: &http.Transport{DisableKeepAlives: true}}, authorizer: authorizer}
	h.start(authorizer)
	return h
}

func resourceSeed(t *testing.T, response *http.Response) seedResource {
	t.Helper()
	result := seedResource{Location: response.Header.Get("Location"), ETag: response.Header.Get("ETag")}
	segments := strings.Split(result.Location, "/")
	parsedID, err := strconv.ParseInt(segments[len(segments)-1], 10, 64)
	if err != nil {
		t.Fatalf("解析 seed 资源标识失败：%v", err)
	}
	result.ID = parsedID
	if result.Location == "" || result.ETag == "" {
		t.Fatalf("seed 缺少 Location/ETag：%+v", result)
	}
	return result
}

// deniedWrites 返回 ArticleType、Tag、Article 的 POST/PATCH/DELETE 共九个合法请求。
func deniedWrites(articleType, tag, article seedResource) []writeCase {
	articleBody := fmt.Sprintf(`{"title":"denied","slug":"denied","digest":"digest","content":"content","article_type_id":%d,"tag_ids":[%d],"status":1}`, articleType.ID, tag.ID)
	return []writeCase{
		{"创建 ArticleType", http.MethodPost, "/api/v1/article-types", `{"name":"denied-type","meun":2}`, nil},
		{"创建 Tag", http.MethodPost, "/api/v1/tags", `{"name":"denied-tag"}`, nil},
		{"创建 Article", http.MethodPost, "/api/v1/articles", articleBody, nil},
		{"更新 ArticleType", http.MethodPatch, articleType.Location, `{"name":"changed-type"}`, ifMatch(articleType.ETag)},
		{"删除 ArticleType", http.MethodDelete, articleType.Location, "", ifMatch(articleType.ETag)},
		{"更新 Tag", http.MethodPatch, tag.Location, `{"name":"changed-tag"}`, ifMatch(tag.ETag)},
		{"删除 Tag", http.MethodDelete, tag.Location, "", ifMatch(tag.ETag)},
		{"更新 Article", http.MethodPatch, article.Location, `{"title":"changed-article"}`, ifMatch(article.ETag)},
		{"删除 Article", http.MethodDelete, article.Location, "", ifMatch(article.ETag)},
	}
}

func ifMatch(etag string) map[string]string { return map[string]string{"If-Match": etag} }

// assertForbiddenProblem 验证403是授权拒绝而不是 schema/资源错误。
func assertForbiddenProblem(t *testing.T, response *http.Response) {
	t.Helper()
	var problem struct {
		Status int    `json:"status"`
		Type   string `json:"type"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("解析403 RFC9457失败：%v", err)
	}
	if problem.Status != http.StatusForbidden || !strings.HasSuffix(problem.Type, "/permission_denied") || problem.Title == "" || problem.Detail == "" {
		t.Fatalf("403 RFC9457不完整：%+v", problem)
	}
}
