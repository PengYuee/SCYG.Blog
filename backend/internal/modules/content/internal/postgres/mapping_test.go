package postgres

import (
	"testing"
	"testing/quick"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

func Test_Mapping_Article_roundtrips_persisted_state(t *testing.T) {
	// Given
	created := time.Date(2026, 7, 12, 1, 2, 3, 0, time.UTC)
	id, _ := domain.NewArticleID(7)
	typeID, _ := domain.NewArticleTypeID(3)
	title, _ := domain.NewTitle("Round trip")
	slug, _ := domain.NewSlug("round-trip")
	digest, _ := domain.NewDigest("digest")
	body, _ := domain.NewContent("body")
	tagID, _ := domain.NewTagID(9)
	version, _ := domain.NewVersion(4)
	article, err := domain.ReconstituteArticle(domain.ArticleState{ID: id, ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, Content: body, Status: domain.StatusPublished, TagIDs: []domain.TagID{tagID}, Version: version, CreatedAt: created, ModifiedAt: created.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	// When
	row, err := articleToModel(article)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := articleFromModel(row, []tagArticleModel{{ArticleID: 7, TagID: 9}})
	// Then
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID() != article.ID() || restored.Status() != domain.StatusPublished || restored.Version() != version || len(restored.TagIDs()) != 1 {
		t.Fatalf("roundtrip mismatch: %#v", restored)
	}
}

func Test_Mapping_Article_property_roundtrips_versions_and_identifiers(t *testing.T) {
	property := func(rawID uint16, rawType uint16, rawTag uint16, rawVersion uint16) bool {
		if rawID == 0 || rawType == 0 || rawTag == 0 || rawVersion == 0 {
			return true
		}
		id, _ := domain.NewArticleID(int64(rawID))
		typeID, _ := domain.NewArticleTypeID(int64(rawType))
		tagID, _ := domain.NewTagID(int64(rawTag))
		version, _ := domain.NewVersion(uint64(rawVersion))
		title, _ := domain.NewTitle("Property")
		slug, _ := domain.NewSlug("property")
		digest, _ := domain.NewDigest("digest")
		body, _ := domain.NewContent("body")
		created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		article, err := domain.ReconstituteArticle(domain.ArticleState{ID: id, ArticleTypeID: typeID, Title: title, Slug: slug, Digest: digest, Content: body, Status: domain.StatusArchived, TagIDs: []domain.TagID{tagID}, Version: version, CreatedAt: created, ModifiedAt: created})
		if err != nil {
			return false
		}
		row, err := articleToModel(article)
		if err != nil {
			return false
		}
		restored, err := articleFromModel(row, []tagArticleModel{{ArticleID: int64(rawID), TagID: int64(rawTag)}})
		return err == nil && restored.ID() == id && restored.ArticleTypeID() == typeID && restored.Version() == version && restored.TagIDs()[0] == tagID
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}

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

func Test_Mapping_Article_projection_preserves_read_counters(t *testing.T) {
	// Given
	row := articleProjectionRow{ID: 1, ArticleTypeID: 2, Title: "Title", Slug: "title", Digest: "Digest", Content: "Body", Status: 2, Support: 7, Comment: 8, Visited: 9, Version: 1, CreationTime: time.Unix(1, 0).UTC()}

	// When
	view, err := projectionView(row, nil)

	// Then
	if err != nil {
		t.Fatalf("projectionView() error = %v", err)
	}
	if view.Support != 7 || view.Comment != 8 || view.Visited != 9 {
		t.Fatalf("counters = %d/%d/%d", view.Support, view.Comment, view.Visited)
	}
}
