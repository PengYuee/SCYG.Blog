//go:build integration

package article

import (
	"context"
	"testing"

	"gorm.io/gorm"
)

func TestArticleGetPostgresKeepsVersionContentAndTagsInOneSnapshot(t *testing.T) {
	for _, management := range []bool{false, true} {
		name := "public"
		if management {
			name = "manage"
		}
		t.Run(name, func(t *testing.T) {
			fixture := openArticleIntegrationFixture(t)
			ctx := context.Background()
			created, err := fixture.service.Create(ctx, Create{Status: ArticleCreationStatusDraft, ArticleTypeID: fixture.typeID, Title: "Snapshot", Slug: "snapshot", Digest: "digest", Content: "old body", TagIDs: []int64{fixture.tagID}})
			if err != nil {
				t.Fatal(err)
			}
			if !management {
				created, err = fixture.service.Publish(ctx, Publish{ID: created.ID, Version: created.Version})
				if err != nil {
					t.Fatal(err)
				}
			}
			var nextTag int64
			if err := fixture.db.GORM().Raw(`INSERT INTO tags (name) VALUES ('next-tag') RETURNING id`).Scan(&nextTag).Error; err != nil {
				t.Fatal(err)
			}
			query, err := NewQuery(fixture.db.GORM())
			if err != nil {
				t.Fatal(err)
			}
			read := query.Get
			if management {
				read = query.GetManage
			}
			// Commit a real replacement immediately after the article SELECT has
			// materialized, before any subsequent SELECT could read its tags.
			fired := false
			var updateErr error
			callback := "test:replace_article_after_snapshot"
			if err := fixture.db.GORM().Callback().Query().After("gorm:query").Register(callback, func(*gorm.DB) {
				if fired {
					return
				}
				fired = true
				updateErr = fixture.db.GORM().Transaction(func(tx *gorm.DB) error {
					if err := tx.Exec(`UPDATE articles SET content = 'new body', version = version + 1 WHERE id = ?`, created.ID).Error; err != nil {
						return err
					}
					if err := tx.Exec(`DELETE FROM article_tags WHERE article_id = ?`, created.ID).Error; err != nil {
						return err
					}
					return tx.Exec(`INSERT INTO article_tags (article_id, tag_id) VALUES (?, ?)`, created.ID, nextTag).Error
				})
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = fixture.db.GORM().Callback().Query().Remove(callback) })
			old, err := read(ctx, Get{ID: created.ID})
			if err != nil || updateErr != nil || !fired {
				t.Fatalf("read=%v replacement=%v fired=%v", err, updateErr, fired)
			}
			if old.Version != created.Version || old.Content != "old body" || len(old.TagIDs) != 1 || old.TagIDs[0] != fixture.tagID {
				t.Fatalf("mixed article snapshot: %#v", old)
			}
			current, err := read(ctx, Get{ID: created.ID})
			if err != nil || current.Version != created.Version+1 || current.Content != "new body" || len(current.TagIDs) != 1 || current.TagIDs[0] != nextTag {
				t.Fatalf("replacement snapshot=%#v error=%v", current, err)
			}
			if management {
				if _, err := query.Get(ctx, Get{ID: created.ID}); !isArticleCode(err, CodeNotFound) {
					t.Fatalf("public draft read error=%v", err)
				}
			}
			if err := fixture.db.GORM().Exec(`DELETE FROM article_tags WHERE article_id = ?`, created.ID).Error; err != nil {
				t.Fatal(err)
			}
			withoutTags, err := read(ctx, Get{ID: created.ID})
			if err != nil || len(withoutTags.TagIDs) != 0 {
				t.Fatalf("untagged article=%#v error=%v", withoutTags, err)
			}
			if err := fixture.service.Delete(ctx, Delete{ID: created.ID, Version: current.Version}); err != nil {
				t.Fatal(err)
			}
			for _, id := range []int64{created.ID, created.ID + 1000} {
				if _, err := read(ctx, Get{ID: id}); !isArticleCode(err, CodeNotFound) {
					t.Fatalf("deleted/missing id=%d error=%v", id, err)
				}
			}
			if _, err := read(ctx, Get{ID: 0}); !isArticleCode(err, CodeValidation) {
				t.Fatalf("invalid id error=%v", err)
			}
		})
	}
}
