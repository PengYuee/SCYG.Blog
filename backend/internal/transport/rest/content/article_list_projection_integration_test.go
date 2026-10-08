//go:build integration

package content_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/gorm"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
)

func TestArticlePostgresKeepsCategorySnapshotAfterMoveAndDelete(t *testing.T) {
	for _, endpoint := range []string{"read-committed", "/api/v1/articles", "/api/v1/manage/articles", "/api/v1/articles/%d", "/api/v1/manage/articles/%d"} {
		t.Run(endpoint, func(t *testing.T) {
			db, store, _ := integrationFixture(t)
			router := integrationRouter(t, db, store, "abcdef0123456789abcdef0123456789")
			var oldType, newType, articleID int64
			if err := db.GORM().Raw(`INSERT INTO article_types (name, image) VALUES ('Old category', '/old.png') RETURNING id`).Scan(&oldType).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.GORM().Raw(`INSERT INTO article_types (name) VALUES ('New category') RETURNING id`).Scan(&newType).Error; err != nil {
				t.Fatal(err)
			}
			status := 2
			if strings.HasPrefix(endpoint, "/api/v1/manage/") {
				status = 1
			}
			if err := db.GORM().Raw(`INSERT INTO articles (article_type_id, title, slug, digest, content, status) VALUES (?, 'Snapshot', 'snapshot', 'digest', 'body', ?) RETURNING id`, oldType, status).Scan(&articleID).Error; err != nil {
				t.Fatal(err)
			}
			// Commit on a different connection after the article SELECT but before
			// category assembly. ReadCommitted loses the old category; the application
			// repeatable-read snapshot must still return that category's real data.
			fired := false
			var moveErr error
			move := func(tx *gorm.DB) {
				statement := tx.Statement.SQL.String()
				if fired || !strings.Contains(statement, "articles AS a") || strings.Contains(statement, "count(") {
					return
				}
				fired = true
				moveErr = db.GORM().Connection(func(writer *gorm.DB) error {
					return writer.Transaction(func(writeTx *gorm.DB) error {
						if err := writeTx.Exec(`UPDATE articles SET article_type_id = ?, version = version + 1 WHERE id = ?`, newType, articleID).Error; err != nil {
							return err
						}
						return writeTx.Exec(`DELETE FROM article_types WHERE id = ?`, oldType).Error
					})
				})
			}
			const callback = "test:move_article_after_category_snapshot"
			if err := db.GORM().Callback().Query().After("gorm:query").Register(callback, move); err != nil {
				t.Fatal(err)
			}
			if err := db.GORM().Callback().Row().After("gorm:row").Register(callback, move); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = db.GORM().Callback().Query().Remove(callback)
				_ = db.GORM().Callback().Row().Remove(callback)
			})
			if endpoint == "read-committed" {
				query, err := article.NewQuery(db.GORM())
				if err != nil {
					t.Fatal(err)
				}
				old, err := query.GetManage(context.Background(), article.Get{ID: articleID})
				if err != nil || !fired || moveErr != nil || old.ArticleTypeID != oldType {
					t.Fatalf("ReadCommitted article=%#v read=%v move=%v fired=%v", old, err, moveErr, fired)
				}
				categories, err := taxonomy.New(db.GORM(), taxonomy.Dependencies{Authorizer: integrationAllowAll{}, Clock: integrationClock{}})
				if err != nil {
					t.Fatal(err)
				}
				_, err = categories.GetArticleType(context.Background(), taxonomy.GetArticleType{ID: old.ArticleTypeID})
				var failure *taxonomy.Error
				if !errors.As(err, &failure) || failure.Code != taxonomy.CodeNotFound {
					t.Fatalf("ReadCommitted late category lookup=%v, want not found", err)
				}
				return
			}
			path := endpoint
			list := !strings.Contains(endpoint, "%d")
			if !list {
				path = fmt.Sprintf(endpoint, articleID)
			}
			read := func() generated.Article {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
				if response.Code != http.StatusOK {
					t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
				}
				if list {
					var body struct {
						Items []generated.Article `json:"items"`
					}
					if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Items) != 1 {
						t.Fatalf("list/body = %v/%s", err, response.Body.String())
					}
					return body.Items[0]
				}
				var body generated.Article
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				return body
			}
			old := read()
			if !fired || moveErr != nil {
				t.Fatalf("move fired=%v error=%v", fired, moveErr)
			}
			if old.ID != articleID || old.ArticleTypeID != oldType || old.ArticleType.ID != oldType || old.ArticleType.Name != "Old category" || old.ArticleType.Image == nil || *old.ArticleType.Image != "/old.png" || old.Content != "body" {
				t.Fatalf("mixed or incomplete snapshot: %#v", old)
			}
			current := read()
			if current.ArticleTypeID != newType || current.ArticleType.ID != newType || current.ArticleType.Name != "New category" || current.ArticleType.Image != nil || current.Version != old.Version+1 {
				t.Fatalf("new snapshot: %#v", current)
			}
		})
	}
}

func TestArticlePostgresMissingCategoryIsInternalFailure(t *testing.T) {
	db, store, _ := integrationFixture(t)
	router := integrationRouter(t, db, store, "abcdef0123456789abcdef0123456789")
	var typeID, articleID int64
	if err := db.GORM().Raw(`INSERT INTO article_types (name) VALUES ('Unavailable category') RETURNING id`).Scan(&typeID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM().Raw(`INSERT INTO articles (article_type_id, title, slug, digest, content, status) VALUES (?, 'Existing article', 'existing-article', 'digest', 'body', 2) RETURNING id`, typeID).Scan(&articleID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM().Exec(`UPDATE article_types SET is_deleted = true, deleted_at = CURRENT_TIMESTAMP WHERE id = ?`, typeID).Error; err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/articles/%d", articleID), nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("category failure misclassified: status=%d body=%s", response.Code, response.Body.String())
	}
}
