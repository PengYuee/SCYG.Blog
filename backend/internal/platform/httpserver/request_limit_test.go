package httpserver_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/httpserver"
)

func Test_RequestLimit_applies_exact_route_and_method_boundaries(t *testing.T) {
	tests := []struct {
		name, method, target string
		size                 int64
		want                 int
	}{
		{"ordinary exact 1MiB", http.MethodPost, "/ordinary", httpserver.MaxRequestBodyBytes, http.StatusNoContent},
		{"ordinary 1MiB plus one", http.MethodPost, "/ordinary", httpserver.MaxRequestBodyBytes + 1, http.StatusRequestEntityTooLarge},
		{"upload exact 6MiB", http.MethodPost, "/api/v1/article-images", httpserver.MaxArticleImageUploadRequestBytes, http.StatusNoContent},
		{"upload 6MiB plus one", http.MethodPost, "/api/v1/article-images", httpserver.MaxArticleImageUploadRequestBytes + 1, http.StatusRequestEntityTooLarge},
		{"get upload path remains default", http.MethodGet, "/api/v1/article-images", httpserver.MaxRequestBodyBytes + 1, http.StatusRequestEntityTooLarge},
		{"delete upload path remains default", http.MethodDelete, "/api/v1/article-images", httpserver.MaxRequestBodyBytes + 1, http.StatusRequestEntityTooLarge},
		{"near path remains default", http.MethodPost, "/api/v1/article-images/", httpserver.MaxRequestBodyBytes + 1, http.StatusRequestEntityTooLarge},
		{"query remains default", http.MethodPost, "/api/v1/article-images?copy=true", httpserver.MaxRequestBodyBytes + 1, http.StatusRequestEntityTooLarge},
		{"force query remains default", http.MethodPost, "/api/v1/article-images?", httpserver.MaxRequestBodyBytes + 1, http.StatusRequestEntityTooLarge},
	}
	server, _ := testServer(t, func(router *gin.Engine) error {
		handler := func(ctx *gin.Context) {
			_, err := io.Copy(io.Discard, ctx.Request.Body)
			if err != nil {
				var maxBytesError *http.MaxBytesError
				if !errors.As(err, &maxBytesError) {
					ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_request_body"})
				}
				return
			}
			ctx.Status(http.StatusNoContent)
		}
		router.Any("/ordinary", handler)
		router.Any("/api/v1/article-images", handler)
		router.Any("/api/v1/article-images/", handler)
		return nil
	})
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(testCase.method, testCase.target, io.LimitReader(zeroReader{}, testCase.size))
			request.ContentLength = testCase.size
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != testCase.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if testCase.want == http.StatusRequestEntityTooLarge && response.Body.String() != "{\"error\":\"request_too_large\"}" {
				t.Fatalf("body=%q", response.Body.String())
			}
		})
	}
}

func Test_RequestLimit_rejects_chunked_body_without_trusting_content_length(t *testing.T) {
	server, _ := testServer(t, func(router *gin.Engine) error {
		router.POST("/ordinary", func(ctx *gin.Context) { _, _ = io.Copy(io.Discard, ctx.Request.Body); ctx.Status(http.StatusNoContent) })
		return nil
	})
	request := httptest.NewRequest(http.MethodPost, "/ordinary", io.LimitReader(zeroReader{}, httpserver.MaxRequestBodyBytes+1))
	request.ContentLength = -1
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge || response.Body.String() != "{\"error\":\"request_too_large\"}" {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
}

func Test_RequestLimit_rejects_body_when_content_length_underreports(t *testing.T) {
	server, _ := testServer(t, func(router *gin.Engine) error {
		router.POST("/ordinary", func(ctx *gin.Context) { _, _ = io.Copy(io.Discard, ctx.Request.Body) })
		return nil
	})
	request := httptest.NewRequest(http.MethodPost, "/ordinary", bytes.NewReader(make([]byte, httpserver.MaxRequestBodyBytes+1)))
	request.ContentLength = 1
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) { clear(buffer); return len(buffer), nil }

func Test_RequestLimit_rejects_chunked_force_query_upload_path_at_default_limit(t *testing.T) {
	// Given
	server, _ := testServer(t, func(router *gin.Engine) error {
		router.POST("/api/v1/article-images", func(ctx *gin.Context) {
			_, _ = io.Copy(io.Discard, ctx.Request.Body)
			ctx.Status(http.StatusNoContent)
		})
		return nil
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/article-images?", io.LimitReader(zeroReader{}, httpserver.MaxRequestBodyBytes+1))
	request.ContentLength = -1
	if !request.URL.ForceQuery {
		t.Fatal("测试请求必须保留尾随问号")
	}
	response := httptest.NewRecorder()

	// When
	server.Handler().ServeHTTP(response, request)

	// Then
	if response.Code != http.StatusRequestEntityTooLarge || response.Body.String() != "{\"error\":\"request_too_large\"}" {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
}

func Test_RequestLimit_tracking_reader_stops_force_query_at_default_limit(t *testing.T) {
	// Given
	server, _ := testServer(t, func(router *gin.Engine) error {
		router.POST("/api/v1/article-images", func(ctx *gin.Context) { _, _ = io.Copy(io.Discard, ctx.Request.Body) })
		return nil
	})
	body := &trackingReader{remaining: 2 * httpserver.MaxRequestBodyBytes}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/article-images?", body)
	request.ContentLength = -1
	response := httptest.NewRecorder()

	// When
	server.Handler().ServeHTTP(response, request)

	// Then
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d", response.Code)
	}
	if body.read != httpserver.MaxRequestBodyBytes+1 {
		t.Fatalf("read=%d want=%d", body.read, httpserver.MaxRequestBodyBytes+1)
	}
}

// trackingReader 记录中间件实际从流中消费的字节数。
type trackingReader struct {
	// remaining 是尚可读取的模拟请求字节数。
	remaining int64
	// read 是中间件实际消费的累计字节数。
	read int64
}

// Read 生成确定性字节流并累计读取量。
func (reader *trackingReader) Read(buffer []byte) (int, error) {
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	count := min(int64(len(buffer)), reader.remaining)
	reader.remaining -= count
	reader.read += count
	return int(count), nil
}
