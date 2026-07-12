package domain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

const articleImagePathPrefix = "/media/article-images/"

var ErrInvalidArticleImageReference = errors.New("正文图片引用不合法")

// ParseArticleImageReferences 使用 Markdown AST 提取受控图片键并保持首次出现顺序。
func ParseArticleImageReferences(source []byte) ([]StorageKey, error) {
	parserContext := parser.NewContext()
	document := goldmark.New().Parser().Parse(text.NewReader(source), parser.WithContext(parserContext))
	keys := make([]StorageKey, 0)
	seen := make(map[string]struct{})
	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch value := node.(type) {
		case *ast.Image:
			destination := string(value.Destination)
			if !strings.HasPrefix(destination, articleImagePathPrefix) {
				if strings.HasPrefix(destination, "/media") && strings.Contains(destination, "article-images") {
					return ast.WalkStop, ErrInvalidArticleImageReference
				}
				return ast.WalkContinue, nil
			}
			key, parseErr := NewStorageKey(strings.TrimPrefix(destination, articleImagePathPrefix))
			if parseErr != nil {
				return ast.WalkStop, fmt.Errorf("图片地址 %q：%w", destination, ErrInvalidArticleImageReference)
			}
			if _, exists := seen[key.String()]; !exists {
				seen[key.String()] = struct{}{}
				keys = append(keys, key)
			}
		case *ast.Link:
			if isControlledArticleImageURL(string(value.Destination)) {
				return ast.WalkStop, ErrInvalidArticleImageReference
			}
		case *ast.RawHTML:
			if isControlledArticleImageURL(string(value.Segments.Value(source))) {
				return ast.WalkStop, ErrInvalidArticleImageReference
			}
		case *ast.HTMLBlock:
			if isControlledArticleImageURL(string(value.Lines().Value(source))) {
				return ast.WalkStop, ErrInvalidArticleImageReference
			}
		case *ast.Text:
			raw := string(value.Segment.Value(source))
			if strings.Contains(raw, `/media\article-images\`) {
				return ast.WalkStop, ErrInvalidArticleImageReference
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func isControlledArticleImageURL(raw string) bool {
	return strings.Contains(raw, articleImagePathPrefix) || strings.Contains(raw, `/media\article-images\`)
}
