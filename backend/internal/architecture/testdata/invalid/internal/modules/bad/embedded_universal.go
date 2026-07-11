package bad

type articleCommands interface{ CreateArticle(Query) error }
type articleQueries interface{ ListArticles(Query) error }

// Everything embeds command and query surfaces into one universal facade.
type Everything interface {
	articleCommands
	articleQueries
}
