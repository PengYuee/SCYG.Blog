package bad

import "context"

// ArticleRepository demonstrates universal untyped CRUD semantics.
type ArticleRepository interface {
	Create(context.Context, any) error
	Get(context.Context, any) (any, error)
	Update(context.Context, any) error
	Delete(context.Context, any) error
}
