package content_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/image"
	restcontent "github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest/content"
)

func Test_ArticleImageHTTP_rejects_file_above_custom_limit(t *testing.T) {
	// Given
	const maxFileBytes = 2
	policy := image.NewPolicy(image.PolicyOptions{MaxFileBytes: maxFileBytes, MaxPixels: 1, MaxDimension: 1, PendingTTL: time.Hour, OrphanGrace: time.Hour})
	service := &imageHTTPService{}
	handler, err := restcontent.NewHandler(service, service, service, service, service, policy)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err = handler.Register(router, testLoginHandler{}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := multipartRequest(t, []struct {
		name    string
		payload []byte
	}{{"file", bytes.Repeat([]byte("x"), maxFileBytes+1)}})

	// When
	router.ServeHTTP(recorder, request)

	// Then
	if recorder.Code != http.StatusBadRequest || service.uploadCalls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", recorder.Code, service.uploadCalls, recorder.Body.String())
	}
}
