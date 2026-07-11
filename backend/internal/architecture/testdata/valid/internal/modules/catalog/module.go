package catalog

// Module is the concrete catalog facade; construction belongs to a later todo.
type Module struct{}

// Find executes the protocol-neutral query.
func (Module) Find(query FindArticle) (ArticleResult, error) { return ArticleResult{}, nil }
