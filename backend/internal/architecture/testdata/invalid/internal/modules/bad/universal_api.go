package bad

// ContentAPI demonstrates a forbidden universal protocol interface.
type ContentAPI interface {
	CreateArticle(Query) error
	ListArticles(Query) error
}
