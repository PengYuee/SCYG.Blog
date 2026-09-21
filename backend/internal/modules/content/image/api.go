// Package image owns article image validation, storage, and lifecycle rules.
package image

import (
	"context"
	"io"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

// Dependencies are the non-persistence collaborators of Service.
type Dependencies struct {
	Authorizer    content.Authorizer
	CurrentAuthor content.CurrentAuthorProvider
	Blob          Blob
	Clock         content.Clock
	Policy        Policy
}

// Authorization actions for image upload and deletion.
const (
	ActionUploadArticleImage content.Action = "content.article_image.upload"
	ActionDeleteArticleImage content.Action = "content.article_image.delete"
)

type (
	// Content is the minimal upload reader accepted by the image feature.
	Content interface{ Read([]byte) (int, error) }
	// Upload contains image upload content.
	Upload struct{ Content Content }
	// Delete identifies a pending image to cancel.
	Delete struct{ ID string }
	// Get identifies an image blob by storage key.
	Get struct{ StorageKey string }
)

// Result is the protocol-neutral uploaded image result.
type Result struct {
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

// Media is the image payload returned for media reads.
type Media struct {
	Content   []byte
	MediaType string
	ByteSize  int64
	SHA256    string
	Pending   bool
}

// Blob abstracts staged and committed image storage.
type Blob interface {
	Stage(context.Context, string, io.Reader) (string, int64, error)
	Commit(string, string) error
	Discard(context.Context, string) error
	Load(string) ([]byte, error)
	Delete(string) error
	ListExpiredTemps(context.Context, time.Time, int) ([]string, error)
	DeleteTemp(context.Context, string) error
}

// PolicyOptions configures image size, pixel, and lifecycle limits.
type PolicyOptions struct {
	MaxFileBytes int64
	MaxPixels    int64
	MaxDimension int
	PendingTTL   time.Duration
	OrphanGrace  time.Duration
}

// Policy contains validated image limits used by services and cleanup.
type Policy struct {
	maxFileBytes int64
	maxPixels    int64
	maxDimension int
	pendingTTL   time.Duration
	orphanGrace  time.Duration
}

// NewPolicy creates an image policy from explicit limits.
func NewPolicy(options PolicyOptions) Policy {
	return Policy{options.MaxFileBytes, options.MaxPixels, options.MaxDimension, options.PendingTTL, options.OrphanGrace}
}

// DefaultPolicy returns the default image policy.
func DefaultPolicy() Policy {
	return NewPolicy(PolicyOptions{MaxFileBytes: 5 << 20, MaxPixels: 25_000_000, MaxDimension: 8192, PendingTTL: 24 * time.Hour, OrphanGrace: 24 * time.Hour})
}

func (policy Policy) valid() Policy {
	if policy.maxFileBytes <= 0 || policy.maxPixels <= 0 || policy.maxDimension <= 0 || policy.pendingTTL <= 0 || policy.orphanGrace <= 0 {
		return DefaultPolicy()
	}
	return policy
}

// MaxFileBytes returns the maximum accepted file size.
func (policy Policy) MaxFileBytes() int64 { return policy.valid().maxFileBytes }

// MaxPixels returns the maximum accepted pixel count.
func (policy Policy) MaxPixels() int64 { return policy.valid().maxPixels }

// MaxDimension returns the maximum accepted width or height.
func (policy Policy) MaxDimension() int { return policy.valid().maxDimension }

// PendingTTL returns the pending-image expiration interval.
func (policy Policy) PendingTTL() time.Duration { return policy.valid().pendingTTL }

// OrphanGrace returns the orphan-image grace interval.
func (policy Policy) OrphanGrace() time.Duration { return policy.valid().orphanGrace }

// ReferenceKeys is opaque to application callers except for image-owned operations.
type ReferenceKeys struct{ keys []StorageKey }

// ParseReferenceKeys validates and deduplicates article image keys.
func ParseReferenceKeys(raw []string) (ReferenceKeys, error) {
	result := ReferenceKeys{keys: make([]StorageKey, 0, len(raw))}
	seen := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		key, err := NewStorageKey(value)
		if err != nil {
			return ReferenceKeys{}, &Error{Code: CodeValidation, Cause: ErrInvalidReference}
		}
		if _, exists := seen[key.String()]; !exists {
			seen[key.String()] = struct{}{}
			result.keys = append(result.keys, key)
		}
	}
	return result, nil
}
func (keys ReferenceKeys) empty() bool { return len(keys.keys) == 0 }

// Empty reports whether no image references were supplied.
func (keys ReferenceKeys) Empty() bool { return keys.empty() }

// PreparedReferences contains only image-owned references prepared for a transaction.
type PreparedReferences struct {
	keys  ReferenceKeys
	owner ImageOwnerID
}

func (prepared PreparedReferences) empty() bool { return prepared.keys.empty() }
