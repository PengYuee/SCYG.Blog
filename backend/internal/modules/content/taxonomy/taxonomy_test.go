package taxonomy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/persistence"
)

type typedNilClock struct{}

func (*typedNilClock) Now() time.Time { return time.Time{} }

func Test_NewService_rejectsTypedNilClock(t *testing.T) {
	var clock *typedNilClock
	_, err := NewService(&Repository{db: &gorm.DB{}}, Dependencies{Clock: clock})
	if err == nil || err.Error() != "taxonomy clock is nil" {
		t.Fatalf("error = %v, want typed-nil clock rejection", err)
	}
}

func Test_ArticleTypeModel_nullableModifiedTime_fallsBackToCreationTime(t *testing.T) {
	created := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	entity, err := entityFromArticleType(articleTypeModel{ID: 1, Name: "News", Version: 1, AuditFields: persistence.NewAuditFields(created, time.Time{}, time.Time{})})
	if err != nil {
		t.Fatalf("map article type: %v", err)
	}
	if !entity.Modified.Equal(created) {
		t.Fatalf("modified time = %v, want creation time %v", entity.Modified, created)
	}
}

func Test_TagModel_nullableModifiedTime_fallsBackToCreationTime(t *testing.T) {
	created := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	entity, err := entityFromTag(tagModel{ID: 1, Name: "Go", Version: 1, AuditFields: persistence.NewAuditFields(created, time.Time{}, time.Time{})})
	if err != nil {
		t.Fatalf("map tag: %v", err)
	}
	if !entity.Modified.Equal(created) {
		t.Fatalf("modified time = %v, want creation time %v", entity.Modified, created)
	}
}

func Test_CreateArticleType_authorizesBeforeDetailValidation(t *testing.T) {
	service := &Service{authorizer: content.DenyAll{}}
	_, err := service.CreateArticleType(context.Background(), CreateArticleType{Name: "News", Meun: -1})
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != CodePermissionDenied {
		t.Fatalf("error = %v, want permission_denied", err)
	}
}

func Test_TranslateDatabase_mapsConstraintCodes(t *testing.T) {
	for _, testCase := range []struct {
		name string
		code string
		want Code
	}{
		{name: "unique", code: "23505", want: CodeAlreadyExists},
		{name: "foreign key", code: "23503", want: CodeFailedPrecondition},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := translateDatabase(&pgconn.PgError{Code: testCase.code})
			var failure *Error
			if !errors.As(err, &failure) || failure.Code != testCase.want {
				t.Fatalf("error = %v, want %s", err, testCase.want)
			}
		})
	}
}
