package postgres

import (
	"testing"
	"time"
)

func Test_Mapping_ArticleType_roundtrips_image_and_meun(t *testing.T) {
	// Given
	image := "hero.png"
	row := articleTypeModel{ID: 1, Name: "News", Image: &image, Meun: 7, Version: 1, CreationTime: time.Unix(1, 0).UTC()}

	// When
	item, err := articleTypeFromModel(row)

	// Then
	if err != nil {
		t.Fatalf("articleTypeFromModel() error = %v", err)
	}
	if item.Image() == nil || *item.Image() != image || item.Meun() != 7 {
		t.Fatalf("article type = image %v, meun %d", item.Image(), item.Meun())
	}
}
