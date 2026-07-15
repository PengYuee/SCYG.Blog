package content

import (
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
)

const (
	defaultArticleImageMaxFileBytes int64 = 5 << 20
	defaultArticleImageMaxPixels    int64 = 25_000_000
	defaultArticleImageMaxDimension       = 8192
	defaultArticleImageLifetime           = 24 * time.Hour
)

// ArticleImagePolicyOptions 汇集来自已验证启动配置的图片安全与生命周期值。
type ArticleImagePolicyOptions struct {
	// MaxFileBytes 是单个原始图片文件的字节上限。
	MaxFileBytes int64
	// MaxPixels 是图片解码前允许的像素总数上限。
	MaxPixels int64
	// MaxDimension 是图片任一边的像素上限。
	MaxDimension int
	// PendingTTL 是上传图片等待文章确认的期限。
	PendingTTL time.Duration
	// OrphanGrace 是失去最后引用后允许恢复的期限。
	OrphanGrace time.Duration
}

// ArticleImagePolicy 是跨 content、REST 与存储共享的不可变图片策略。
type ArticleImagePolicy struct {
	maxFileBytes int64
	maxPixels    int64
	maxDimension int
	pendingTTL   time.Duration
	orphanGrace  time.Duration
}

// NewArticleImagePolicy 从已验证配置构造不可变图片策略。
func NewArticleImagePolicy(options ArticleImagePolicyOptions) ArticleImagePolicy {
	return ArticleImagePolicy{maxFileBytes: options.MaxFileBytes, maxPixels: options.MaxPixels, maxDimension: options.MaxDimension, pendingTTL: options.PendingTTL, orphanGrace: options.OrphanGrace}
}

// DefaultArticleImagePolicy 返回兼容既有运行时行为的默认策略。
func DefaultArticleImagePolicy() ArticleImagePolicy {
	return NewArticleImagePolicy(ArticleImagePolicyOptions{MaxFileBytes: defaultArticleImageMaxFileBytes, MaxPixels: defaultArticleImageMaxPixels, MaxDimension: defaultArticleImageMaxDimension, PendingTTL: defaultArticleImageLifetime, OrphanGrace: defaultArticleImageLifetime})
}

// MaxFileBytes 返回单个原始图片文件的字节上限。
func (policy ArticleImagePolicy) MaxFileBytes() int64 { return policy.maxFileBytes }

// MaxPixels 返回图片解码前允许的像素总数上限。
func (policy ArticleImagePolicy) MaxPixels() int64 { return policy.maxPixels }

// MaxDimension 返回图片任一边的像素上限。
func (policy ArticleImagePolicy) MaxDimension() int { return policy.maxDimension }

// PendingTTL 返回上传图片等待文章确认的期限。
func (policy ArticleImagePolicy) PendingTTL() time.Duration { return policy.pendingTTL }

// OrphanGrace 返回失去最后引用后允许恢复的期限。
func (policy ArticleImagePolicy) OrphanGrace() time.Duration { return policy.orphanGrace }

func (policy ArticleImagePolicy) validationLimits() domain.ArticleImageValidationLimits {
	return domain.NewArticleImageValidationLimits(policy.maxFileBytes, policy.maxPixels, policy.maxDimension)
}

func (policy ArticleImagePolicy) orDefault() ArticleImagePolicy {
	if policy.maxFileBytes <= 0 || policy.maxPixels <= 0 || policy.maxDimension <= 0 || policy.pendingTTL <= 0 || policy.orphanGrace <= 0 {
		return DefaultArticleImagePolicy()
	}
	return policy
}
