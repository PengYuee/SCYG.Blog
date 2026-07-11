// Package architecture enforces the binding modular-monolith source rules.
package architecture

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const modulePath = "github.com/PengYuee/SCYG.Blog/backend"

// Violation is one actionable architecture failure.
type Violation struct {
	// Code identifies the stable architecture rule.
	Code string
	// Path identifies the offending file relative to the scanned root.
	Path string
	// Detail explains the corrective action.
	Detail string
}

// Error renders a stable, path-bearing diagnostic.
func (violation Violation) Error() string {
	return fmt.Sprintf("%s: %s: %s", violation.Code, violation.Path, violation.Detail)
}

type sourceFile struct {
	parsed   *ast.File
	absolute string
	relative string
}

// Scan parses every Go source below root and returns deterministic violations.
func Scan(root string) ([]Violation, error) {
	files, err := loadSources(root)
	if err != nil {
		return nil, err
	}
	violations := make([]Violation, 0)
	for _, file := range files {
		violations = append(violations, checkSize(file)...)
		violations = append(violations, checkPath(file)...)
		violations = append(violations, checkImports(file)...)
		violations = append(violations, checkDeclarations(file)...)
	}
	violations = append(violations, checkModuleShape(files)...)
	sort.Slice(violations, func(left, right int) bool {
		if violations[left].Path == violations[right].Path {
			return violations[left].Code < violations[right].Code
		}
		return violations[left].Path < violations[right].Path
	})
	return violations, nil
}

func loadSources(root string) ([]sourceFile, error) {
	files := make([]sourceFile, 0)
	fileSet := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		parsed, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("make %s relative: %w", path, err)
		}
		files = append(files, sourceFile{absolute: path, relative: filepath.ToSlash(relative), parsed: parsed})
		return nil
	})
	return files, err
}

func checkSize(file sourceFile) []Violation {
	if file.relative == "internal/generated/openapi/openapi.gen.go" {
		return nil
	}
	handle, err := os.Open(file.absolute)
	if err != nil {
		return []Violation{{Code: "ARCH_FILE_READ", Path: file.relative, Detail: err.Error()}}
	}
	count := 0
	scanner := bufio.NewScanner(handle)
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
			count++
		}
	}
	scanErr := scanner.Err()
	closeErr := handle.Close()
	if scanErr != nil {
		return []Violation{{Code: "ARCH_FILE_READ", Path: file.relative, Detail: scanErr.Error()}}
	}
	if closeErr != nil {
		return []Violation{{Code: "ARCH_FILE_READ", Path: file.relative, Detail: closeErr.Error()}}
	}
	if count > 250 {
		return []Violation{{Code: "ARCH_FILE_SIZE", Path: file.relative, Detail: fmt.Sprintf("handwritten file has %d pure LOC; maximum is 250", count)}}
	}
	return nil
}
