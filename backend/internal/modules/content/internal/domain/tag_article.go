package domain

// TagArticle 仅表示文章与标签之间的业务关联。
type TagArticle struct {
	articleID ArticleID
	tagID     TagID
}

// NewTagArticle 使用已解析标识创建已校验关联。
func NewTagArticle(articleID ArticleID, tagID TagID) (TagArticle, error) {
	if !articleID.valid() {
		return TagArticle{}, invalid("article_id")
	}
	if !tagID.valid() {
		return TagArticle{}, invalid("tag_id")
	}
	return TagArticle{articleID: articleID, tagID: tagID}, nil
}
func (link TagArticle) ArticleID() ArticleID { return link.articleID }
func (link TagArticle) TagID() TagID         { return link.tagID }
