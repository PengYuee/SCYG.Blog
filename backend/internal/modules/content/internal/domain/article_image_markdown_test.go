package domain

import (
	"errors"
	"reflect"
	"testing"
)

func Test_ParseArticleImageReferences_extracts_unique_images_in_stable_order(t *testing.T) {
	markdown := []byte("![a](/media/article-images/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.jpg)\n![dup](/media/article-images/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.jpg)\n![b][ref]\n\n[ref]: /media/article-images/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.png\n`![code](/media/article-images/cccccccccccccccccccccccccccccccc.jpg)`\n![external](https://example.com/x.png)")
	keys, err := ParseArticleImageReferences(markdown)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	got := make([]string, len(keys))
	for index, key := range keys {
		got[index] = key.String()
	}
	want := []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.jpg", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("期望 %v，实际 %v", want, got)
	}
}

func Test_ParseArticleImageReferences_rejects_controlled_url_outside_image(t *testing.T) {
	tests := []string{
		"[link](/media/article-images/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.jpg)",
		`<a href="/media/article-images/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.jpg">x</a>`,
		`<img src="/media/article-images/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.jpg">`,
	}
	for _, markdown := range tests {
		if _, err := ParseArticleImageReferences([]byte(markdown)); !errors.Is(err, ErrInvalidArticleImageReference) {
			t.Fatalf("%q 期望受控 URL 错误，实际 %v", markdown, err)
		}
	}
}

func Test_ParseArticleImageReferences_rejects_noncanonical_controlled_image_url(t *testing.T) {
	tests := []string{
		"![x](/media/article-images/%61aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.jpg)",
		"![x](/media/article-images/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.jpg?x=1)",
		"![x](/media/article-images/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.jpg#x)",
		"![x](/media\\article-images\\aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.jpg)",
	}
	for _, markdown := range tests {
		if _, err := ParseArticleImageReferences([]byte(markdown)); !errors.Is(err, ErrInvalidArticleImageReference) {
			t.Fatalf("%q 期望受控 URL 错误，实际 %v", markdown, err)
		}
	}
}
