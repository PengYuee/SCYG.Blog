// Package content exposes protocol-neutral content commands, queries, and results.
package content

import (
	"errors"
	"reflect"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/application"
)

// Clock supplies deterministic use-case time.
type Clock = application.Clock

// UnitOfWork owns transaction-scoped content repositories.
type UnitOfWork = application.UnitOfWork

// ArticleReadModel serves published article projections.
type ArticleReadModel = application.ArticleReadModel

// TaxonomyReadModel serves nondeleted taxonomy projections.
type TaxonomyReadModel = application.TaxonomyReadModel

// Dependencies contains the explicit protocol-neutral collaborators required by Module.
type Dependencies struct {
	Clock      Clock
	Authorizer Authorizer
	UnitOfWork UnitOfWork
	Articles   ArticleReadModel
	Taxonomies TaxonomyReadModel
}

// Module is the concrete protocol-neutral content facade.
type Module struct {
	clock      Clock
	authorizer Authorizer
	unit       UnitOfWork
	articles   ArticleReadModel
	taxonomies TaxonomyReadModel
}

// NewModule safely composes all content use cases and defaults omitted authorization to DenyAll.
func NewModule(dependencies Dependencies) (*Module, error) {
	if nilLike(dependencies.Clock) {
		return nil, errors.New("content clock is nil")
	}
	if nilLike(dependencies.UnitOfWork) {
		return nil, errors.New("content unit of work is nil")
	}
	if nilLike(dependencies.Articles) {
		return nil, errors.New("content article read model is nil")
	}
	if nilLike(dependencies.Taxonomies) {
		return nil, errors.New("content taxonomy read model is nil")
	}
	return &Module{clock: dependencies.Clock, authorizer: AuthorizerOrDeny(dependencies.Authorizer), unit: dependencies.UnitOfWork, articles: dependencies.Articles, taxonomies: dependencies.Taxonomies}, nil
}

func nilLike(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
