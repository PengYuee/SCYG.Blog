package article

import (
	"errors"
	"testing"
	"time"
)

type markdownContentClock struct{}

func (markdownContentClock) Now() time.Time { return time.Unix(100, 0).UTC() }

func TestArticleMarkdownWhitespaceSurvivesCreationAndRevision(t *testing.T) {
	cases := []struct {
		name     string
		original string
		revised  string
	}{
		{"multiline", "# 标题\n\n段落。\r\n\r\n\t缩进代码", "# 更新\n\n段落。\r\n\r\n\t更新代码"},
		{"first line tab", "\t缩进代码\n\n段落。  \n", "\t更新代码\n\n更新段落。  \r\n\n"},
		{"first line four spaces", "    缩进代码\n\n段落。  ", "    更新代码\n\n更新段落。  \n\n"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			draft, err := parseCreate(Create{
				ArticleTypeID: 1,
				Title:         "Markdown 正文",
				Slug:          "markdown-content",
				Digest:        "格式化正文",
				Content:       testCase.original,
				TagIDs:        []int64{1},
			})
			if err != nil {
				t.Fatalf("parse multiline article: %v", err)
			}
			draft.ID, err = NewArticleID(1)
			if err != nil {
				t.Fatal(err)
			}
			article, err := NewArticle(draft, StatusDraft, markdownContentClock{})
			if err != nil {
				t.Fatalf("create multiline article: %v", err)
			}
			if got := article.content.String(); got != testCase.original {
				t.Fatalf("created Markdown = %q, want %q", got, testCase.original)
			}

			body, err := NewContent(testCase.revised)
			if err != nil {
				t.Fatalf("parse revised Markdown: %v", err)
			}
			revision := ArticleRevision{
				ArticleTypeID: draft.ArticleTypeID,
				Title:         draft.Title,
				Slug:          draft.Slug,
				Digest:        draft.Digest,
				Content:       body,
				TagIDs:        draft.TagIDs,
			}
			if err := article.Revise(article.version, revision, markdownContentClock{}); err != nil {
				t.Fatalf("revise multiline article: %v", err)
			}
			if got := article.content.String(); got != testCase.revised {
				t.Fatalf("revised Markdown = %q, want %q", got, testCase.revised)
			}
		})
	}
}

func TestContentRejectsWhitespaceInvalidUTF8AndOtherControls(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"ASCII whitespace", " \t\r\n    \n"},
		{"Unicode whitespace", "\u00a0\u2003\u3000"},
		{"invalid UTF-8", "正文\xff"},
		{"NUL", "正常段落\n\x00隐藏控制字符"},
		{"vertical tab", "段落\v后续"},
		{"form feed", "段落\f后续"},
		{"DEL", "段落\x7f后续"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := NewContent(testCase.raw); !errors.Is(err, ErrInvalidValue) {
				t.Fatalf("content error = %v, want %v", err, ErrInvalidValue)
			}
			if (Content{value: testCase.raw}).valid() {
				t.Fatal("domain content validation accepted rejected input")
			}
		})
	}
}
