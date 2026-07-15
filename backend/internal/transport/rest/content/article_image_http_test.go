package content_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	restcontent "github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest/content"
)

const (
	testImageID  = "0123456789abcdef0123456789abcdef"
	testImageKey = testImageID + ".jpg"
)

type imageHTTPService struct {
	restcontent.QueryService
	restcontent.CommandService
	upload      module.ArticleImageResult
	media       module.ArticleImageMedia
	uploadErr   error
	deleteErr   error
	mediaErr    error
	uploaded    []byte
	uploadCalls int
	deleteCalls int
}

func (service *imageHTTPService) UploadArticleImage(_ context.Context, command module.UploadArticleImage) (module.ArticleImageResult, error) {
	service.uploadCalls++
	buffer := make([]byte, 32*1024)
	for {
		count, err := command.Content.ReadArticleImage(buffer)
		service.uploaded = append(service.uploaded, buffer[:count]...)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return module.ArticleImageResult{}, err
		}
	}
	return service.upload, service.uploadErr
}

func (service *imageHTTPService) CancelArticleImage(context.Context, module.DeleteArticleImage) error {
	service.deleteCalls++
	return service.deleteErr
}

func (service *imageHTTPService) GetArticleImageMedia(context.Context, module.GetArticleImage) (module.ArticleImageMedia, error) {
	return service.media, service.mediaErr
}

func imageRouter(t *testing.T, service *imageHTTPService) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler, err := restcontent.NewHandler(service, service, module.DefaultArticleImagePolicy())
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	if err = handler.Register(router); err != nil {
		t.Fatal(err)
	}
	return router
}

func multipartRequest(t *testing.T, parts []struct {
	name    string
	payload []byte
},
) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		field, err := writer.CreateFormFile(part.name, "ignored-client-name.jpg")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = field.Write(part.payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/article-images", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func Test_ArticleImageHTTP_uploads_one_file_through_real_strict_router(t *testing.T) {
	service := &imageHTTPService{upload: module.ArticleImageResult{ID: testImageID, StorageKey: testImageKey, URL: "/media/article-images/" + testImageKey, MediaType: "image/jpeg", ByteSize: 3, Width: 1, Height: 1, Status: "pending", ExpiresAt: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)}}
	recorder := httptest.NewRecorder()
	imageRouter(t, service).ServeHTTP(recorder, multipartRequest(t, []struct {
		name    string
		payload []byte
	}{{"file", []byte("abc")}}))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.uploadCalls != 1 || string(service.uploaded) != "abc" {
		t.Fatalf("calls=%d payload=%q", service.uploadCalls, service.uploaded)
	}
	if recorder.Header().Get("Location") != "/media/article-images/"+testImageKey {
		t.Fatalf("location=%q", recorder.Header().Get("Location"))
	}
}

func Test_ArticleImageHTTP_rejects_invalid_multipart_without_calling_usecase(t *testing.T) {
	cases := []struct {
		name  string
		parts []struct {
			name    string
			payload []byte
		}
	}{{"missing", nil}, {"unknown", []struct {
		name    string
		payload []byte
	}{{"caption", []byte("x")}}}, {"repeated", []struct {
		name    string
		payload []byte
	}{{"file", []byte("a")}, {"file", []byte("b")}}}, {"oversize", []struct {
		name    string
		payload []byte
	}{{"file", bytes.Repeat([]byte{'x'}, 5<<20+1)}}}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			service := &imageHTTPService{}
			recorder := httptest.NewRecorder()
			imageRouter(t, service).ServeHTTP(recorder, multipartRequest(t, test.parts))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if service.uploadCalls != 0 {
				t.Fatalf("uploadCalls=%d", service.uploadCalls)
			}
		})
	}
}

func Test_ArticleImageHTTP_get_uses_cache_and_security_headers(t *testing.T) {
	digest := strings.Repeat("a", 64)
	service := &imageHTTPService{media: module.ArticleImageMedia{Content: []byte("jpeg"), MediaType: "image/jpeg", ByteSize: 4, SHA256: digest}}
	router := imageRouter(t, service)
	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/media/article-images/"+testImageKey, nil))
	if first.Code != http.StatusOK || first.Body.String() != "jpeg" {
		t.Fatalf("status=%d body=%q", first.Code, first.Body.String())
	}
	if first.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || first.Header().Get("X-Content-Type-Options") != "nosniff" || first.Header().Get("Content-Length") != "4" {
		t.Fatalf("headers=%v", first.Header())
	}
	second := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/media/article-images/"+testImageKey, nil)
	request.Header.Set("If-None-Match", ` "other" , W/"`+digest+`" `)
	router.ServeHTTP(second, request)
	if second.Code != http.StatusNotModified || second.Body.Len() != 0 || second.Header().Get("ETag") != `"`+digest+`"` {
		t.Fatalf("status=%d etag=%q body=%q", second.Code, second.Header().Get("ETag"), second.Body.String())
	}
}

func Test_ArticleImageHTTP_malformed_if_none_match_returns_bytes(t *testing.T) {
	digest := strings.Repeat("b", 64)
	service := &imageHTTPService{media: module.ArticleImageMedia{Content: []byte("png"), MediaType: "image/png", ByteSize: 3, SHA256: digest, Pending: true}}
	request := httptest.NewRequest(http.MethodGet, "/media/article-images/"+testImageID+".png", nil)
	request.Header.Set("If-None-Match", `W/invalid, "`+digest+`"`)
	recorder := httptest.NewRecorder()
	imageRouter(t, service).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "png" || recorder.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("status=%d cache=%q body=%q", recorder.Code, recorder.Header().Get("Cache-Control"), recorder.Body.String())
	}
}

func Test_ArticleImageHTTP_delete_maps_idempotent_not_found_and_committed_conflict(t *testing.T) {
	cases := []struct {
		name    string
		failure error
		status  int
	}{
		{"pending or orphaned owner", nil, http.StatusNoContent},
		{"cross owner", &module.ApplicationError{Code: module.CodeNotFound, Kind: module.KindMissing, Cause: module.ErrNotFound}, http.StatusNotFound},
		{"committed", &module.ApplicationError{Code: module.CodeFailedPrecondition, Kind: module.KindConflict, Cause: module.ErrFailedPrecondition}, http.StatusConflict},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			service := &imageHTTPService{deleteErr: test.failure}
			recorder := httptest.NewRecorder()
			imageRouter(t, service).ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/article-images/"+testImageID, nil))
			if recorder.Code != test.status || service.deleteCalls != 1 {
				t.Fatalf("status=%d calls=%d body=%s", recorder.Code, service.deleteCalls, recorder.Body.String())
			}
		})
	}
}

func Test_ArticleImageHTTP_get_maps_orphan_and_missing_file_to_not_found(t *testing.T) {
	service := &imageHTTPService{mediaErr: &module.ApplicationError{Code: module.CodeNotFound, Kind: module.KindMissing, Cause: module.ErrNotFound}}
	recorder := httptest.NewRecorder()
	imageRouter(t, service).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/media/article-images/"+testImageKey, nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
