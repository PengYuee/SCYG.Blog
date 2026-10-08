//go:build integration

package application_test

import (
	"context"
	"testing"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
)

func responseWorkflow(t *testing.T, fixture applicationFixture) (*application.ArticleResponses, *article.Service) {
	t.Helper()
	articles, err := article.New(fixture.db.GORM(), article.Dependencies{Authorizer: applicationAllowAll{}, Clock: fixture.clock})
	if err != nil {
		t.Fatal(err)
	}
	categories, err := taxonomy.New(fixture.db.GORM(), taxonomy.Dependencies{Authorizer: applicationAllowAll{}, Clock: fixture.clock})
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := application.NewArticleResponses(fixture.db.GORM(), articles, categories, fixture.workflow)
	if err != nil {
		t.Fatal(err)
	}
	return workflow, articles
}

func TestArticleResponsesPostgresPreservesWriteVersionAndCategory(t *testing.T) {
	fixture := newApplicationFixture(t)
	workflow, _ := responseWorkflow(t, fixture)
	ctx := context.Background()
	if err := fixture.db.GORM().Exec(`UPDATE article_types SET image = '/category.png' WHERE id = ?`, fixture.typeID).Error; err != nil {
		t.Fatal(err)
	}
	assertResult := func(item article.Result, err error, version uint64, status string) {
		t.Helper()
		if err != nil || item.Version != version || item.Status != status || item.ArticleTypeID != fixture.typeID || item.ArticleTypeName != "application-test-type" || item.ArticleTypeImage == nil || *item.ArticleTypeImage != "/category.png" {
			t.Fatalf("write result=%#v error=%v, want version=%d status=%s and category", item, err, version, status)
		}
	}
	created, err := workflow.Create(ctx, article.Create{ArticleTypeID: fixture.typeID, Title: "Response", Slug: "response", Digest: "digest", Content: "body", TagIDs: []int64{fixture.tagID}, Status: article.ArticleCreationStatusDraft})
	assertResult(created, err, 1, "draft")
	title := "Updated response"
	patched, err := workflow.Patch(ctx, article.Patch{ID: created.ID, Version: created.Version, Title: &title})
	assertResult(patched, err, 2, "draft")
	if len(patched.TagIDs) != 1 || patched.TagIDs[0] != fixture.tagID {
		t.Fatalf("omitted tags lost: %#v", patched)
	}
	published, err := workflow.Publish(ctx, article.Publish{ID: patched.ID, Version: patched.Version})
	assertResult(published, err, 3, "published")
	archived, err := workflow.Archive(ctx, article.Archive{ID: published.ID, Version: published.Version})
	assertResult(archived, err, 4, "archived")
}

func TestArticleResponsesPostgresRollsBackWritesWithoutLiveCategory(t *testing.T) {
	for _, operation := range []string{"create", "patch", "publish", "archive"} {
		t.Run(operation, func(t *testing.T) {
			fixture := newApplicationFixture(t)
			workflow, articles := responseWorkflow(t, fixture)
			ctx := context.Background()
			before, err := fixture.workflow.Create(ctx, article.Create{ArticleTypeID: fixture.typeID, Title: "Before", Slug: "before", Digest: "digest", Content: "body", TagIDs: []int64{fixture.tagID}, Status: article.ArticleCreationStatusDraft})
			if err != nil {
				t.Fatal(err)
			}
			if operation == "archive" {
				before, err = articles.Publish(ctx, article.Publish{ID: before.ID, Version: before.Version})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := fixture.db.GORM().Exec(`UPDATE article_types SET is_deleted = true, deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, fixture.typeID).Error; err != nil {
				t.Fatal(err)
			}
			var out article.Result
			switch operation {
			case "create":
				out, err = workflow.Create(ctx, article.Create{ArticleTypeID: fixture.typeID, Title: "New", Slug: "new", Digest: "digest", Content: "body", Status: article.ArticleCreationStatusDraft})
			case "patch":
				title := "Must roll back"
				out, err = workflow.Patch(ctx, article.Patch{ID: before.ID, Version: before.Version, Title: &title})
			case "publish":
				out, err = workflow.Publish(ctx, article.Publish{ID: before.ID, Version: before.Version})
			case "archive":
				out, err = workflow.Archive(ctx, article.Archive{ID: before.ID, Version: before.Version})
			}
			if err == nil || out.ID != 0 {
				t.Fatalf("unavailable category write result=%#v error=%v", out, err)
			}
			after, err := articles.GetManage(ctx, article.Get{ID: before.ID})
			if err != nil || after.Version != before.Version || after.Status != before.Status || after.Title != before.Title || len(after.TagIDs) != 1 || after.TagIDs[0] != fixture.tagID {
				t.Fatalf("write failed to roll back: before=%#v after=%#v error=%v", before, after, err)
			}
			var count int64
			if err := fixture.db.GORM().Table("articles").Where("slug = ?", "new").Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("failed create persisted: count=%d error=%v", count, err)
			}
		})
	}
}
