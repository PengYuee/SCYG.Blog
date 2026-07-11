package bad

// AllProtocols is a renamed universal module-root facade.
type AllProtocols interface {
	CreateArticle(Query) error
	ListArticles(Query) error
}
