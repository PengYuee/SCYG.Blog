package article

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// ErrInvalidImageReference indicates a managed article image reference is malformed.
var ErrInvalidImageReference = errors.New("正文图片引用不合法")

const imagePathPrefix = "/media/article-images/"

// ManagedImageReferences extracts controlled image key candidates without image-specific key validation.
func ManagedImageReferences(source string) ([]string, error) {
	parserContext := parser.NewContext()
	document := goldmark.New().Parser().Parse(text.NewReader([]byte(source)), parser.WithContext(parserContext))
	keys := make([]string, 0)
	seen := make(map[string]struct{})
	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		appendKey := func(destination string) error {
			if !strings.HasPrefix(destination, imagePathPrefix) {
				return nil
			}
			key := strings.TrimPrefix(destination, imagePathPrefix)
			if key == "" || strings.Contains(key, "/") || strings.ContainsAny(key, "?#") {
				return ErrInvalidImageReference
			}
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				keys = append(keys, key)
			}
			return nil
		}
		rejectControlled := func(raw string) error {
			if strings.Contains(raw, imagePathPrefix) || strings.Contains(raw, `/media\article-images\`) || strings.Contains(raw, `/media\\article-images\\`) {
				return ErrInvalidImageReference
			}
			return nil
		}
		switch value := node.(type) {
		case *ast.Image:
			destination := string(value.Destination)
			if err := appendKey(destination); err != nil {
				return ast.WalkStop, fmt.Errorf("图片地址 %q：%w", destination, err)
			}
			if strings.HasPrefix(destination, "/media") && strings.Contains(destination, "article-images") && !strings.HasPrefix(destination, imagePathPrefix) {
				return ast.WalkStop, ErrInvalidImageReference
			}
		case *ast.Link:
			if err := rejectControlled(string(value.Destination)); err != nil {
				return ast.WalkStop, err
			}
		case *ast.RawHTML:
			if err := rejectControlled(string(value.Segments.Value([]byte(source)))); err != nil {
				return ast.WalkStop, err
			}
		case *ast.HTMLBlock:
			if err := rejectControlled(string(value.Lines().Value([]byte(source)))); err != nil {
				return ast.WalkStop, err
			}
		case *ast.Text:
			raw := string(value.Segment.Value([]byte(source)))
			if strings.Contains(raw, `/media\article-images\`) || strings.Contains(raw, `/media\\article-images\\`) {
				return ast.WalkStop, ErrInvalidImageReference
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}
