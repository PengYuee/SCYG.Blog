package apidocs_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest/apidocs"
)

func Test_Docs_disabled_registers_no_routes(t *testing.T) {
	router := gin.New()
	if err := apidocs.Mount(router, false); err != nil {
		t.Fatalf("mount disabled: %v", err)
	}
	for _, path := range []string{"/docs", "/openapi.yaml", "/docs/assets/scalar.js"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d", path, response.Code)
		}
	}
}

func Test_Docs_serves_offline_assets_and_authoritative_spec(t *testing.T) {
	router := gin.New()
	if err := apidocs.Mount(router, true); err != nil {
		t.Fatalf("mount: %v", err)
	}
	docs := get(t, router, "/docs")
	if strings.Contains(docs.Body.String(), "https://") || strings.Contains(docs.Body.String(), "http://") {
		t.Fatalf("external URL in HTML: %s", docs.Body.String())
	}
	references := regexp.MustCompile(`(?:src|data-url)="([^"]+)"`).FindAllStringSubmatch(docs.Body.String(), -1)
	for _, reference := range references {
		response := get(t, router, reference[1])
		if response.Code != http.StatusOK {
			t.Fatalf("asset %s status=%d", reference[1], response.Code)
		}
	}
	spec := get(t, router, "/openapi.yaml")
	authoritative, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read authoritative spec: %v", err)
	}
	if !bytes.Equal(spec.Body.Bytes(), authoritative) {
		t.Fatal("embedded OpenAPI drift")
	}
}

func Test_Docs_rejects_duplicate_mount_without_panic(t *testing.T) {
	router := gin.New()
	if err := apidocs.Mount(router, true); err != nil {
		t.Fatalf("first mount: %v", err)
	}
	if err := apidocs.Mount(router, true); err == nil {
		t.Fatal("duplicate mount accepted")
	}
}

func Test_Docs_vendor_artifact_matches_manifest_checksum(t *testing.T) {
	artifact, err := os.ReadFile(filepath.Join("assets", "scalar.js"))
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	sum := sha256.Sum256(artifact)
	if hex.EncodeToString(sum[:]) != "b5edb255af0e112c4530c41da2350fca28a72f4388694880ba50acb442fda88f" {
		t.Fatal("Scalar artifact checksum mismatch")
	}
}

func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	return response
}
