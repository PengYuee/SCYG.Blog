package config

import "time"

// ArticleImages 是不可变的文章图片存储、生命周期与上传安全配置。
type ArticleImages struct {
	directory           string
	developmentAuthorID string
	pendingTTL          time.Duration
	orphanGrace         time.Duration
	cleanupInterval     time.Duration
	uploadRequestBytes  int64
	maxFileBytes        int64
	maxPixels           int64
	maxDimension        int
}

// Directory 返回已清理的存储目录；相对路径由组合根相对配置文件目录解析。
func (images ArticleImages) Directory() string { return images.directory }

// PendingTTL 返回未确认图片的最长存活时间。
func (images ArticleImages) PendingTTL() time.Duration { return images.pendingTTL }

// OrphanGrace 返回失去引用后允许恢复的宽限期。
func (images ArticleImages) OrphanGrace() time.Duration { return images.orphanGrace }

// CleanupInterval 返回后台清理扫描间隔。
func (images ArticleImages) CleanupInterval() time.Duration { return images.cleanupInterval }

// UploadRequestBytes 返回图片上传 HTTP 请求体上限。
func (images ArticleImages) UploadRequestBytes() int64 { return images.uploadRequestBytes }

// MaxFileBytes 返回单个图片文件的字节上限。
func (images ArticleImages) MaxFileBytes() int64 { return images.maxFileBytes }

// MaxPixels 返回解码后图片像素总数上限。
func (images ArticleImages) MaxPixels() int64 { return images.maxPixels }

// MaxDimension 返回图片任一边的像素上限。
func (images ArticleImages) MaxDimension() int { return images.maxDimension }

// DevelopmentAuthorID 返回仅开发环境可用的固定作者标识；日志和序列化不得调用该 getter。
func (images ArticleImages) DevelopmentAuthorID() string { return images.developmentAuthorID }
