package content_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
)

func Test_ContentREST_public_taxonomy_returns_count_and_strong_etag(t *testing.T) {
	service := &testService{
		publicArticleTypePage: taxonomy.PublicArticleTypePage{
			Items: []taxonomy.PublicArticleTypeResult{{ID: 3, Name: "News", ArticleCount: 4, Version: 2}}, Number: 1, Size: 20, TotalItems: 1, TotalPages: 1,
		},
		publicTagPage: taxonomy.PublicTagPage{
			Items: []taxonomy.PublicTagResult{{ID: 5, Name: "Go", ArticleCount: 4, Version: 3}}, Number: 1, Size: 20, TotalItems: 1, TotalPages: 1,
		},
	}
	router := routerForService(t, service)

	tests := []struct {
		path string
		want string
	}{
		{path: "/api/v1/article-types?page=1&pageSize=20", want: `{"articleCount":4,"id":3,"image":null,"name":"News"}`},
		{path: "/api/v1/tags?page=1&pageSize=20", want: `{"articleCount":4,"id":5,"name":"Go"}`},
	}
	for _, testCase := range tests {
		t.Run(testCase.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, testCase.path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var body struct {
				Items []json.RawMessage `json:"items"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Items) != 1 || string(body.Items[0]) != testCase.want {
				t.Fatalf("body=%s want item=%s err=%v", response.Body.String(), testCase.want, err)
			}
		})
	}

	for _, testCase := range []struct {
		path string
		want string
	}{
		{"/api/v1/article-types/3", `"1"`},
		{"/api/v1/tags/5", `"1"`},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, testCase.path, nil))
		if response.Code != http.StatusOK || response.Header().Get("ETag") != testCase.want {
			t.Fatalf("path=%s status=%d etag=%q", testCase.path, response.Code, response.Header().Get("ETag"))
		}
	}
}
