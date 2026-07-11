package httpserver_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func Test_HTTPServer_import_boundary_excludes_transport_content_and_generated(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package: %v", err)
	}
	forbidden := []string{"/internal/transport/", "/internal/content/", "/internal/generated/"}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", entry.Name(), parseErr)
		}
		for _, spec := range parsed.Imports {
			imported, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				t.Fatalf("unquote %s: %v", spec.Path.Value, unquoteErr)
			}
			for _, fragment := range forbidden {
				if strings.Contains(imported, fragment) {
					t.Fatalf("%s imports forbidden %s", entry.Name(), imported)
				}
			}
		}
	}
}
