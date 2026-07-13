//go:build integration

package content_test

import (
	"context"
	"sync"
	"testing"
	"time"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	contentpostgres "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/postgres"
)

func serviceForReferenceOwner(t *testing.T, fixture articleReferenceFixture, owner string) *module.Module {
	t.Helper()
	authorID, err := module.NewAuthorID(owner)
	if err != nil {
		t.Fatal(err)
	}
	service, err := contentpostgres.New(contentpostgres.Dependencies{Database: fixture.database, Authorizer: integrationAuthorizer{}, CurrentAuthor: module.NewFixedCurrentAuthorProvider(authorID), ImageFilesystem: fixture.storage, ImagePendingTTL: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func Test_CreateAndPatch_without_managed_images_do_not_require_current_author(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	service, err := contentpostgres.New(contentpostgres.Dependencies{Database: fixture.database, Authorizer: integrationAuthorizer{}, ImageFilesystem: fixture.storage, ImagePendingTTL: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}

	// When
	created, err := createReferenceArticle(t, service, "without-author", "![外部](https://example.com/image.jpg)")
	if err != nil {
		t.Fatal(err)
	}
	body := "没有受控图片"
	_, err = service.PatchArticle(context.Background(), module.PatchArticle{ID: created.ID, Version: created.Version, Content: &body})
	// Then
	if err != nil {
		t.Fatal(err)
	}
}

func Test_CreateArticle_rolls_back_when_image_belongs_to_another_owner(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	foreign := serviceForReferenceOwner(t, fixture, "11111111111111111111111111111111")
	imageResult := uploadReferenceImage(t, foreign)

	// When
	_, err := createReferenceArticle(t, fixture.module, "cross-owner", "![图]("+imageResult.URL+")")

	// Then
	if err == nil {
		t.Fatal("CreateArticle() error = nil")
	}
	var articles, pending int64
	_ = fixture.database.GORM().Table(`"Article"`).Count(&articles).Error
	_ = fixture.database.GORM().Table("article_images").Where("id = ? AND status = 'pending'", imageResult.ID).Count(&pending).Error
	if articles != 0 || pending != 1 {
		t.Fatalf("articles=%d pending=%d", articles, pending)
	}
}

func Test_CreateArticle_rolls_back_when_pending_image_is_expired(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	imageResult := uploadReferenceImage(t, fixture.module)
	if err := fixture.database.GORM().Table("article_images").Where("id = ?", imageResult.ID).Updates(map[string]any{"created_at": time.Now().Add(-2 * time.Hour), "expires_at": time.Now().Add(-time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}

	// When
	_, err := createReferenceArticle(t, fixture.module, "expired-image", "![图]("+imageResult.URL+")")

	// Then
	if err == nil {
		t.Fatal("CreateArticle() error = nil")
	}
	var articles, pending int64
	_ = fixture.database.GORM().Table(`"Article"`).Count(&articles).Error
	_ = fixture.database.GORM().Table("article_images").Where("id = ? AND status = 'pending'", imageResult.ID).Count(&pending).Error
	if articles != 0 || pending != 1 {
		t.Fatalf("articles=%d pending=%d", articles, pending)
	}
}

func Test_PatchArticle_version_conflict_precedes_missing_image_file(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	created, err := createReferenceArticle(t, fixture.module, "stale-first", "正文")
	if err != nil {
		t.Fatal(err)
	}
	missing := "![图](/media/article-images/11111111111111111111111111111111.jpg)"

	// When
	_, err = fixture.module.PatchArticle(context.Background(), module.PatchArticle{ID: created.ID, Version: created.Version + 1, Content: &missing})

	// Then
	var applicationError *module.ApplicationError
	if err == nil || !errorAs(err, &applicationError) || applicationError.Code != module.CodeStaleVersion {
		t.Fatalf("PatchArticle() error = %v", err)
	}
	var references int64
	_ = fixture.database.GORM().Table("article_image_references").Where("article_id = ?", created.ID).Count(&references).Error
	if references != 0 {
		t.Fatalf("references=%d", references)
	}
}

func Test_CreateArticle_opposite_image_orders_complete_without_deadlock(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	first := uploadReferenceImage(t, fixture.module)
	second := uploadReferenceImage(t, fixture.module)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index, markdown := range []string{"![一](" + first.URL + ") ![二](" + second.URL + ")", "![二](" + second.URL + ") ![一](" + first.URL + ")"} {
		wait.Add(1)
		go func(slug string, body string) {
			defer wait.Done()
			<-start
			_, createErr := createReferenceArticle(t, fixture.module, slug, body)
			results <- createErr
		}("opposite-"+string(rune('a'+index)), markdown)
	}

	// When
	close(start)
	done := make(chan struct{})
	go func() { wait.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("反序图片锁事务超时")
	}

	// Then
	close(results)
	for result := range results {
		if result != nil {
			t.Fatal(result)
		}
	}
	var references int64
	_ = fixture.database.GORM().Table("article_image_references").Count(&references).Error
	if references != 4 {
		t.Fatalf("references=%d", references)
	}
}

func errorAs(err error, target **module.ApplicationError) bool {
	for current := err; current != nil; {
		if typed, ok := current.(*module.ApplicationError); ok {
			*target = typed
			return true
		}
		unwrapper, ok := current.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		current = unwrapper.Unwrap()
	}
	return false
}
