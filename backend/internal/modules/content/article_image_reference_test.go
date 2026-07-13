package content

import (
	"testing"
)

func Test_managedImageKeys_extracts_deduplicated_keys_when_markdown_contains_managed_images(t *testing.T) {
	// Given
	const first = "11111111111111111111111111111111.jpg"
	const second = "22222222222222222222222222222222.png"
	markdown := "![一](/media/article-images/" + first + ")\n![重复](/media/article-images/" + first + ")\n![二](/media/article-images/" + second + ")"

	// When
	keys, err := managedImageKeys(markdown)
	// Then
	if err != nil {
		t.Fatalf("managedImageKeys() error = %v", err)
	}
	if len(keys) != 2 || keys[0].String() != first || keys[1].String() != second {
		t.Fatalf("managedImageKeys() = %v", keys)
	}
}

func Test_managedImageKeys_rejects_raw_html_when_controlled_url_is_hidden(t *testing.T) {
	// Given
	markdown := `<img src="/media/article-images/11111111111111111111111111111111.jpg">`

	// When
	_, err := managedImageKeys(markdown)

	// Then
	if err == nil {
		t.Fatal("managedImageKeys() error = nil")
	}
}
