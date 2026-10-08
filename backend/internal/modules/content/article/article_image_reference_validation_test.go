package article

import (
	"errors"
	"reflect"
	"testing"
)

func TestManagedImageReferencesExtractsAndDeduplicatesControlledImages(t *testing.T) {
	keys, err := ManagedImageReferences("![first](/media/article-images/first.jpg) ![again](http://127.0.0.1:8080/media/article-images/first.jpg) ![second](https://api.example.test/media/article-images/second.png) ![external](https://images.example.test/picture.png)")
	if err != nil {
		t.Fatalf("extract references: %v", err)
	}
	if want := []string{"first.jpg", "second.png"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %#v, want %#v", keys, want)
	}
}

func TestManagedImageReferencesDoesNotTreatPlainProseAsAReference(t *testing.T) {
	keys, err := ManagedImageReferences("正文提到 /media/article-images/not-a-markdown-reference.jpg，但没有嵌入图片。")
	if err != nil {
		t.Fatalf("plain prose rejected: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("keys = %#v, want empty", keys)
	}
}

func TestManagedImageReferencesRejectsControlledURLsOutsideImageNodes(t *testing.T) {
	cases := []string{
		"[download](/media/article-images/image.jpg)",
		`<a href="/media/article-images/image.jpg">download</a>`,
		`<img src="/media\\article-images\\image.jpg">`,
	}
	for _, source := range cases {
		t.Run(source, func(t *testing.T) {
			if _, err := ManagedImageReferences(source); !errors.Is(err, ErrInvalidImageReference) {
				t.Fatalf("error = %v, want %v", err, ErrInvalidImageReference)
			}
		})
	}
}

func TestManagedImageReferencesRejectsMalformedControlledImageURL(t *testing.T) {
	_, err := ManagedImageReferences("![bad](/media/article-images/nested/image.jpg)")
	if !errors.Is(err, ErrInvalidImageReference) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidImageReference)
	}
}
