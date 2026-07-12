package content

import (
	"context"
	"time"
)

const (
	// ActionUploadArticleImage 授权上传正文图片。
	ActionUploadArticleImage Action = "content.article_image.upload"
	// ActionDeleteArticleImage 授权取消正文图片。
	ActionDeleteArticleImage Action = "content.article_image.delete"
	// ActionReadArticleImage 授权读取未确认正文图片。
	ActionReadArticleImage Action = "content.article_image.read"
)

// ArticleImageContent 是不暴露通用 IO 类型的图片内容读取源。
type (
	ArticleImageContent interface{ ReadArticleImage([]byte) (int, error) }
	// ArticleImageStager 将验证后的图片写入受控临时对象。
	ArticleImageStager interface {
		StageArticleImage(context.Context, string, ArticleImageContent) (string, int64, error)
	}
	// ArticleImagePublisher 原子发布受控临时图片。
	ArticleImagePublisher interface{ CommitArticleImage(string, string) error }
	// ArticleImageDiscarder 丢弃受控临时图片。
	ArticleImageDiscarder interface {
		DiscardArticleImage(context.Context, string) error
	}
	// ArticleImageLoader 将最终图片按严格存储键读入有界字节切片。
	ArticleImageLoader interface{ LoadArticleImage(string) ([]byte, error) }
)

// UploadArticleImage 描述已由协议边界提取的单一图片内容。
type (
	UploadArticleImage struct{ Content ArticleImageContent }
	// DeleteArticleImage 描述待取消图片标识。
	DeleteArticleImage struct{ ID string }
	// GetArticleImage 描述按严格存储键读取图片。
	GetArticleImage struct{ StorageKey string }
)

// ArticleImageResult 是上传成功后返回的公开图片元数据。
type (
	ArticleImageResult struct {
		ID         string
		StorageKey string
		URL        string
		MediaType  string
		ByteSize   int64
		Width      int
		Height     int
		Status     string
		ExpiresAt  time.Time
	}
	// ArticleImageMedia 是可直接映射到 HTTP 响应的有界图片内容。
	ArticleImageMedia struct {
		Content   []byte
		MediaType string
		ByteSize  int64
		SHA256    string
		Pending   bool
	}
)
