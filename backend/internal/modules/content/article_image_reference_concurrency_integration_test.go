//go:build integration

package content_test

import (
	"context"
	"testing"
	"time"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

func Test_PatchArticle_two_articles_swap_images_without_deadlock(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	firstImage := uploadReferenceImage(t, fixture.module)
	secondImage := uploadReferenceImage(t, fixture.module)
	firstArticle, err := createReferenceArticle(t, fixture.module, "swap-first", "![甲]("+firstImage.URL+")")
	if err != nil {
		t.Fatal(err)
	}
	secondArticle, err := createReferenceArticle(t, fixture.module, "swap-second", "![乙]("+secondImage.URL+")")
	if err != nil {
		t.Fatal(err)
	}
	firstBody := "![乙](" + secondImage.URL + ")"
	secondBody := "![甲](" + firstImage.URL + ")"
	start := make(chan struct{})
	results := make(chan error, 2)

	// When
	go func() {
		<-start
		_, patchErr := fixture.module.PatchArticle(context.Background(), module.PatchArticle{ID: firstArticle.ID, Version: firstArticle.Version, Content: &firstBody})
		results <- patchErr
	}()
	go func() {
		<-start
		_, patchErr := fixture.module.PatchArticle(context.Background(), module.PatchArticle{ID: secondArticle.ID, Version: secondArticle.Version, Content: &secondBody})
		results <- patchErr
	}()
	close(start)

	// Then
	for range 2 {
		select {
		case result := <-results:
			if result != nil {
				t.Fatal(result)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("两文章交叉换图事务超时")
		}
	}
	var firstReferences, secondReferences int64
	_ = fixture.database.GORM().Table("article_image_references").Where("article_id = ? AND image_id = ?", firstArticle.ID, secondImage.ID).Count(&firstReferences).Error
	_ = fixture.database.GORM().Table("article_image_references").Where("article_id = ? AND image_id = ?", secondArticle.ID, firstImage.ID).Count(&secondReferences).Error
	var committed int64
	_ = fixture.database.GORM().Table("article_images").Where("id IN ? AND status = 'committed'", []string{firstImage.ID, secondImage.ID}).Count(&committed).Error
	if firstReferences != 1 || secondReferences != 1 || committed != 2 {
		t.Fatalf("first=%d second=%d committed=%d", firstReferences, secondReferences, committed)
	}
}

func Test_PatchArticle_reverse_multiple_keys_complete_without_deadlock(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	first := uploadReferenceImage(t, fixture.module)
	second := uploadReferenceImage(t, fixture.module)
	third := uploadReferenceImage(t, fixture.module)
	left, err := createReferenceArticle(t, fixture.module, "multi-left", "![甲]("+first.URL+")")
	if err != nil {
		t.Fatal(err)
	}
	right, err := createReferenceArticle(t, fixture.module, "multi-right", "![丙]("+third.URL+")")
	if err != nil {
		t.Fatal(err)
	}
	leftBody := "![乙](" + second.URL + ") ![丙](" + third.URL + ")"
	rightBody := "![乙](" + second.URL + ") ![甲](" + first.URL + ")"
	start := make(chan struct{})
	results := make(chan error, 2)

	// When
	go func() {
		<-start
		_, patchErr := fixture.module.PatchArticle(context.Background(), module.PatchArticle{ID: left.ID, Version: left.Version, Content: &leftBody})
		results <- patchErr
	}()
	go func() {
		<-start
		_, patchErr := fixture.module.PatchArticle(context.Background(), module.PatchArticle{ID: right.ID, Version: right.Version, Content: &rightBody})
		results <- patchErr
	}()
	close(start)

	// Then
	for range 2 {
		select {
		case patchErr := <-results:
			if patchErr != nil {
				t.Fatal(patchErr)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("反序多图事务超时")
		}
	}
}

func Test_PatchArticle_waits_for_cleanup_image_lock_then_orphans_consistently(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	imageResult := uploadReferenceImage(t, fixture.module)
	article, err := createReferenceArticle(t, fixture.module, "cleanup-lock", "![图]("+imageResult.URL+")")
	if err != nil {
		t.Fatal(err)
	}
	cleanupTransaction := fixture.database.GORM().Begin()
	if cleanupTransaction.Error != nil {
		t.Fatal(cleanupTransaction.Error)
	}
	if err = cleanupTransaction.Exec("SELECT id FROM article_images WHERE id = ? FOR UPDATE", imageResult.ID).Error; err != nil {
		_ = cleanupTransaction.Rollback()
		t.Fatal(err)
	}
	plain := "清理竞态后的正文"
	result := make(chan error, 1)

	// When
	go func() {
		_, patchErr := fixture.module.PatchArticle(context.Background(), module.PatchArticle{ID: article.ID, Version: article.Version, Content: &plain})
		result <- patchErr
	}()
	select {
	case patchErr := <-result:
		_ = cleanupTransaction.Rollback()
		t.Fatalf("Patch 未等待 cleanup 行锁：%v", patchErr)
	case <-time.After(150 * time.Millisecond):
	}
	if err = cleanupTransaction.Commit().Error; err != nil {
		t.Fatal(err)
	}

	// Then
	select {
	case patchErr := <-result:
		if patchErr != nil {
			t.Fatal(patchErr)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("释放 cleanup 行锁后 Patch 未完成")
	}
	var references, orphaned int64
	_ = fixture.database.GORM().Table("article_image_references").Where("article_id = ?", article.ID).Count(&references).Error
	_ = fixture.database.GORM().Table("article_images").Where("id = ? AND status = 'orphaned'", imageResult.ID).Count(&orphaned).Error
	if references != 0 || orphaned != 1 {
		t.Fatalf("references=%d orphaned=%d", references, orphaned)
	}
}

func Test_PatchArticle_rolls_back_when_reference_count_lock_is_cancelled(t *testing.T) {
	// Given
	fixture := openArticleReferenceFixture(t)
	imageResult := uploadReferenceImage(t, fixture.module)
	article, err := createReferenceArticle(t, fixture.module, "count-lock-cancel", "![图]("+imageResult.URL+")")
	if err != nil {
		t.Fatal(err)
	}
	blockingTransaction := fixture.database.GORM().Begin()
	if blockingTransaction.Error != nil {
		t.Fatal(blockingTransaction.Error)
	}
	if err = blockingTransaction.Exec("SELECT id FROM article_images WHERE id = ? FOR UPDATE", imageResult.ID).Error; err != nil {
		_ = blockingTransaction.Rollback()
		t.Fatal(err)
	}
	plain := "计数锁取消后的正文"
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	// When
	_, err = fixture.module.PatchArticle(ctx, module.PatchArticle{ID: article.ID, Version: article.Version, Content: &plain})
	if rollbackErr := blockingTransaction.Rollback().Error; rollbackErr != nil {
		t.Fatal(rollbackErr)
	}

	// Then
	if err == nil {
		t.Fatal("PatchArticle() error = nil")
	}
	var references, committed int64
	_ = fixture.database.GORM().Table("article_image_references").Where("article_id = ?", article.ID).Count(&references).Error
	_ = fixture.database.GORM().Table("article_images").Where("id = ? AND status = 'committed'", imageResult.ID).Count(&committed).Error
	var content string
	_ = fixture.database.GORM().Table(`"Article"`).Select(`"Content"`).Where(`"Id" = ?`, article.ID).Row().Scan(&content)
	if references != 1 || committed != 1 || content != "![图]("+imageResult.URL+")" {
		t.Fatalf("references=%d committed=%d content=%q", references, committed, content)
	}
}
