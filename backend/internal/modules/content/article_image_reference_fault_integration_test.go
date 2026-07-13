//go:build integration

package content_test

import (
	"testing"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

func Test_CreateArticle_rolls_back_image_when_article_save_fails(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	if _, err := createReferenceArticle(t, fixture.module, "duplicate-slug", "原文章"); err != nil {
		t.Fatal(err)
	}
	imageResult := uploadReferenceImage(t, fixture.module)

	// When
	_, err := createReferenceArticle(t, fixture.module, "duplicate-slug", "![图]("+imageResult.URL+")")

	// Then
	assertPendingWithoutReferences(t, fixture, imageResult)
	if err == nil {
		t.Fatal("CreateArticle() error = nil")
	}
}

func Test_CreateArticle_rolls_back_when_reference_replace_fails(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	imageResult := uploadReferenceImage(t, fixture.module)
	installReferenceFailureTrigger(t, fixture)

	// When
	_, err := createReferenceArticle(t, fixture.module, "reference-failure", "![图]("+imageResult.URL+")")

	// Then
	assertPendingWithoutReferences(t, fixture, imageResult)
	if err == nil {
		t.Fatal("CreateArticle() error = nil")
	}
}

func Test_CreateArticle_rolls_back_when_image_save_fails(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	imageResult := uploadReferenceImage(t, fixture.module)
	if err := fixture.database.GORM().Exec(`CREATE FUNCTION todo7_fail_image_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'todo7 image save failure'; END $$; CREATE TRIGGER todo7_image_update BEFORE UPDATE ON article_images FOR EACH ROW EXECUTE FUNCTION todo7_fail_image_update()`).Error; err != nil {
		t.Fatal(err)
	}

	// When
	_, err := createReferenceArticle(t, fixture.module, "image-save-failure", "![图]("+imageResult.URL+")")

	// Then
	assertPendingWithoutReferences(t, fixture, imageResult)
	if err == nil {
		t.Fatal("CreateArticle() error = nil")
	}
}

func installReferenceFailureTrigger(t *testing.T, fixture articleReferenceFixture) {
	t.Helper()
	if err := fixture.database.GORM().Exec(`CREATE FUNCTION todo7_fail_reference_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'todo7 reference failure'; END $$; CREATE TRIGGER todo7_reference_insert BEFORE INSERT ON article_image_references FOR EACH ROW EXECUTE FUNCTION todo7_fail_reference_insert()`).Error; err != nil {
		t.Fatal(err)
	}
}

func assertPendingWithoutReferences(t *testing.T, fixture articleReferenceFixture, imageResult module.ArticleImageResult) {
	t.Helper()
	var articles, references, pending int64
	_ = fixture.database.GORM().Table(`"Article"`).Where(`"Slug" LIKE ?`, "%failure%").Count(&articles).Error
	_ = fixture.database.GORM().Table("article_image_references").Where("image_id = ?", imageResult.ID).Count(&references).Error
	_ = fixture.database.GORM().Table("article_images").Where("id = ? AND status = 'pending'", imageResult.ID).Count(&pending).Error
	if articles != 0 || references != 0 || pending != 1 {
		t.Fatalf("articles=%d references=%d pending=%d", articles, references, pending)
	}
}
