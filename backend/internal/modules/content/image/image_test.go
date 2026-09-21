package image

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
	"time"
)

func TestNewStorageKeyRejectsShortInputWithoutPanic(t *testing.T) {
	for _, raw := range []string{"", "a", "0123456789abcdef0123456789abcdef", "0123456789abcdef0123456789abcdef.gif", "0123456789ABCDEF0123456789abcdef.jpg"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := NewStorageKey(raw); err == nil {
				t.Fatalf("storage key %q was accepted", raw)
			}
		})
	}
}

func TestValidateArticleImageReencodesSafeJPEGAndRejectsTrailingData(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 3))
	source.Set(0, 0, color.White)
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, source, nil); err != nil {
		t.Fatal(err)
	}
	validated, err := ValidateArticleImage(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("valid JPEG rejected: %v", err)
	}
	if validated.MediaType != MediaTypeJPEG || validated.Width != 2 || validated.Height != 3 || len(validated.Bytes) < 1 {
		t.Fatalf("unexpected validated image: %+v", validated)
	}
	if _, err := ValidateArticleImage(bytes.NewReader(append(encoded.Bytes(), 1, 2, 3))); !errors.Is(err, ErrInvalidArticleImage) {
		t.Fatalf("JPEG with trailing data error = %v", err)
	}
}

func TestArticleImageLifecyclePreservesPendingCommitOrphanRecovery(t *testing.T) {
	created := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	id, _ := NewArticleImageID("0123456789abcdef0123456789abcdef")
	owner, _ := NewImageOwnerID("abcdef0123456789abcdef0123456789")
	key, _ := NewStorageKey("0123456789abcdef0123456789abcdef.jpg")
	value, err := NewArticleImage(ArticleImageMetadata{ID: id, OwnerID: owner, StorageKey: key, MediaType: MediaTypeJPEG, ByteSize: 1, Width: 1, Height: 1, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}, created, created.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if value.Status() != ArticleImageStatusPending {
		t.Fatalf("initial status = %s", value.Status())
	}
	if err := value.Commit(created.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if value.Status() != ArticleImageStatusCommitted {
		t.Fatalf("committed status = %s", value.Status())
	}
	if err := value.OrphanWithGrace(created.Add(2*time.Hour), 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if value.Status() != ArticleImageStatusOrphaned {
		t.Fatalf("orphaned status = %s", value.Status())
	}
	if err := value.Commit(created.Add(3 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if value.Status() != ArticleImageStatusCommitted || !value.OrphanedAt().IsZero() {
		t.Fatalf("recovered image = status %s orphanedAt %v", value.Status(), value.OrphanedAt())
	}
}

func TestArticleImageRejectsCommitAtExpiry(t *testing.T) {
	created := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	id, _ := NewArticleImageID("0123456789abcdef0123456789abcdef")
	owner, _ := NewImageOwnerID("abcdef0123456789abcdef0123456789")
	key, _ := NewStorageKey("0123456789abcdef0123456789abcdef.jpg")
	value, err := NewArticleImage(ArticleImageMetadata{ID: id, OwnerID: owner, StorageKey: key, MediaType: MediaTypeJPEG, ByteSize: 1, Width: 1, Height: 1, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}, created, created.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Commit(created.Add(time.Hour)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("commit at expiry error = %v", err)
	}
}
