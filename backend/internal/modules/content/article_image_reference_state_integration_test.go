//go:build integration

package content_test

import (
	"context"
	"testing"
	"time"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

func Test_CreateArticle_rolls_back_when_image_metadata_is_missing(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	imageResult := uploadReferenceImage(t, fixture.module)
	if err := fixture.database.GORM().Table("article_images").Where("id = ?", imageResult.ID).Delete(nil).Error; err != nil {
		t.Fatal(err)
	}

	// When
	_, err := createReferenceArticle(t, fixture.module, "missing-metadata", "![图]("+imageResult.URL+")")

	// Then
	if err == nil {
		t.Fatal("CreateArticle() error = nil")
	}
	var articles int64
	_ = fixture.database.GORM().Table(`"Article"`).Count(&articles).Error
	if articles != 0 {
		t.Fatalf("articles=%d", articles)
	}
}

func Test_PatchArticle_rejects_orphan_after_grace_without_changing_article(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	imageResult := uploadReferenceImage(t, fixture.module)
	created, err := createReferenceArticle(t, fixture.module, "grace-past", "![图]("+imageResult.URL+")")
	if err != nil {
		t.Fatal(err)
	}
	plain := "正文已移除图片"
	removed, err := fixture.module.PatchArticle(context.Background(), module.PatchArticle{ID: created.ID, Version: created.Version, Content: &plain})
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if err = fixture.database.GORM().Table("article_images").Where("id = ?", imageResult.ID).Updates(map[string]any{"created_at": past.Add(-3 * time.Hour), "committed_at": past.Add(-2 * time.Hour), "orphaned_at": past.Add(-time.Hour), "expires_at": past}).Error; err != nil {
		t.Fatal(err)
	}
	recoverBody := "![图](" + imageResult.URL + ")"

	// When
	_, err = fixture.module.PatchArticle(context.Background(), module.PatchArticle{ID: removed.ID, Version: removed.Version, Content: &recoverBody})

	// Then
	if err == nil {
		t.Fatal("PatchArticle() error = nil")
	}
	var version uint64
	var content string
	if queryErr := fixture.database.GORM().Table(`"Article"`).Select(`"Version","Content"`).Where(`"Id" = ?`, removed.ID).Row().Scan(&version, &content); queryErr != nil {
		t.Fatal(queryErr)
	}
	var references int64
	_ = fixture.database.GORM().Table("article_image_references").Where("article_id = ?", removed.ID).Count(&references).Error
	if version != removed.Version || content != plain || references != 0 {
		t.Fatalf("version=%d content=%q references=%d", version, content, references)
	}
}

func Test_PatchArticle_keeps_shared_image_committed_until_last_reference_removed(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	imageResult := uploadReferenceImage(t, fixture.module)
	body := "![图](" + imageResult.URL + ")"
	first, err := createReferenceArticle(t, fixture.module, "shared-first", body)
	if err != nil {
		t.Fatal(err)
	}
	second, err := createReferenceArticle(t, fixture.module, "shared-second", body)
	if err != nil {
		t.Fatal(err)
	}
	plain := "正文无图片"

	// When
	_, err = fixture.module.PatchArticle(context.Background(), module.PatchArticle{ID: first.ID, Version: first.Version, Content: &plain})
	if err != nil {
		t.Fatal(err)
	}
	var committedAfterFirst int64
	_ = fixture.database.GORM().Table("article_images").Where("id = ? AND status = 'committed'", imageResult.ID).Count(&committedAfterFirst).Error
	_, err = fixture.module.PatchArticle(context.Background(), module.PatchArticle{ID: second.ID, Version: second.Version, Content: &plain})
	// Then
	if err != nil {
		t.Fatal(err)
	}
	var orphanedAfterLast int64
	_ = fixture.database.GORM().Table("article_images").Where("id = ? AND status = 'orphaned'", imageResult.ID).Count(&orphanedAfterLast).Error
	if committedAfterFirst != 1 || orphanedAfterLast != 1 {
		t.Fatalf("committedAfterFirst=%d orphanedAfterLast=%d", committedAfterFirst, orphanedAfterLast)
	}
}
