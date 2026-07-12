package domain

// ArticleID 是已解析的正文章标识。
type ArticleID struct{ value int64 }

// ArticleTypeID 是已解析的正文章分类标识。
type ArticleTypeID struct{ value int64 }

// TagID 是已解析的正标签标识。
type TagID struct{ value int64 }

// NewArticleID 将正整数解析为文章标识。
func NewArticleID(raw int64) (ArticleID, error) {
	if raw <= 0 {
		return ArticleID{}, invalid("article_id")
	}
	return ArticleID{raw}, nil
}

// NewArticleTypeID 将正整数解析为文章分类标识。
func NewArticleTypeID(raw int64) (ArticleTypeID, error) {
	if raw <= 0 {
		return ArticleTypeID{}, invalid("article_type_id")
	}
	return ArticleTypeID{raw}, nil
}

// NewTagID 将正整数解析为标签标识。
func NewTagID(raw int64) (TagID, error) {
	if raw <= 0 {
		return TagID{}, invalid("tag_id")
	}
	return TagID{raw}, nil
}

// Int64 返回文章标识数值。
func (id ArticleID) Int64() int64 { return id.value }

// Int64 返回文章分类标识数值。
func (id ArticleTypeID) Int64() int64 { return id.value }

// Int64 返回标签标识数值。
func (id TagID) Int64() int64        { return id.value }
func (id ArticleID) valid() bool     { return id.value > 0 }
func (id ArticleTypeID) valid() bool { return id.value > 0 }
func (id TagID) valid() bool         { return id.value > 0 }
