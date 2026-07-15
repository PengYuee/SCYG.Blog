package content

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

type cleanupImageService struct {
	QueryService
	CommandService
}

func (*cleanupImageService) UploadArticleImage(context.Context, module.UploadArticleImage) (module.ArticleImageResult, error) {
	return module.ArticleImageResult{ID: "0123456789abcdef0123456789abcdef", StorageKey: "0123456789abcdef0123456789abcdef.jpg", URL: "/media/article-images/0123456789abcdef0123456789abcdef.jpg", MediaType: "image/jpeg", ByteSize: 3, Width: 1, Height: 1, Status: "pending", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (*cleanupImageService) CancelArticleImage(context.Context, module.DeleteArticleImage) error {
	return nil
}

func (*cleanupImageService) GetArticleImageMedia(context.Context, module.GetArticleImage) (module.ArticleImageMedia, error) {
	return module.ArticleImageMedia{}, nil
}

type cleanupFaultHandle struct {
	requestTempHandle
	closeErr error
}

func (handle cleanupFaultHandle) Close() error {
	return errors.Join(handle.requestTempHandle.Close(), handle.closeErr)
}

type cleanupFaultOperations struct {
	closeErr  error
	removeErr error
	paths     []string
}

func (operations *cleanupFaultOperations) Create() (requestTempHandle, error) {
	file, err := os.CreateTemp("", "scyg-article-image-request-fault-*.tmp")
	if err == nil {
		operations.paths = append(operations.paths, file.Name())
	}
	return file, err
}

func (operations *cleanupFaultOperations) Open(path string) (requestTempHandle, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return cleanupFaultHandle{requestTempHandle: file, closeErr: operations.closeErr}, nil
}

func (operations *cleanupFaultOperations) Remove(path string) error {
	if operations.removeErr != nil {
		return operations.removeErr
	}
	return os.Remove(path)
}

func (operations *cleanupFaultOperations) cleanup() {
	for _, path := range operations.paths {
		_ = os.Remove(path)
	}
}

func cleanupMultipartRequest(t *testing.T, name string, payload []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(name, "client.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/article-images", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func cleanupRouter(t *testing.T, operations requestTempOperations) http.Handler {
	t.Helper()
	service := &cleanupImageService{}
	handler := &Handler{queries: service, commands: service, imagePolicy: module.DefaultArticleImagePolicy(), tempFiles: operations}
	router := gin.New()
	if err := handler.Register(router); err != nil {
		t.Fatal(err)
	}
	return router
}

func Test_CreateArticleImage_cleanup_failure_returns_stable_problem_before_201(t *testing.T) {
	cases := []struct {
		name      string
		closeErr  error
		removeErr error
	}{{"close", errors.New("close"), nil}, {"remove", nil, errors.New("remove")}, {"both", errors.New("close"), errors.New("remove")}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			operations := &cleanupFaultOperations{closeErr: test.closeErr, removeErr: test.removeErr}
			defer operations.cleanup()
			recorder := httptest.NewRecorder()
			cleanupRouter(t, operations).ServeHTTP(recorder, cleanupMultipartRequest(t, "file", []byte("abc")))
			if recorder.Code != http.StatusInternalServerError || recorder.Header().Get("Content-Type") != "application/problem+json" {
				t.Fatalf("status=%d type=%q body=%s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
			}
			if len(operations.paths) != 1 {
				t.Fatalf("paths=%v", operations.paths)
			}
		})
	}
}

func Test_SpoolUniqueImagePart_early_error_does_not_create_temp(t *testing.T) {
	operations := &cleanupFaultOperations{}
	handler := &Handler{imagePolicy: module.DefaultArticleImagePolicy(), tempFiles: operations}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormField("unknown")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("x"))
	_ = writer.Close()
	_, err = handler.spoolUniqueImagePart(context.Background(), multipart.NewReader(&body, writer.Boundary()))
	if err == nil || len(operations.paths) != 0 {
		t.Fatalf("error=%v paths=%v", err, operations.paths)
	}
}

func Test_SpoolUniqueImagePart_cancel_attempts_close_and_remove(t *testing.T) {
	operations := &cleanupFaultOperations{}
	handler := &Handler{imagePolicy: module.DefaultArticleImagePolicy(), tempFiles: operations}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := handler.spoolUniqueImagePart(ctx, multipartReaderForSpool(t, bytes.Repeat([]byte{'x'}, 1024)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	for _, path := range operations.paths {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("temp remains=%s err=%v", path, statErr)
		}
	}
}
