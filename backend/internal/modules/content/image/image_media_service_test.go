package image

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"
	"time"
)

type mediaBlobStub struct {
	data []byte
	err  error
}

func (blob mediaBlobStub) Load(string) ([]byte, error) { return blob.data, blob.err }

func (blob mediaBlobStub) Stage(context.Context, string, io.Reader) (string, int64, error) {
	return "", 0, nil
}
func (blob mediaBlobStub) Commit(string, string) error           { return nil }
func (blob mediaBlobStub) Discard(context.Context, string) error { return nil }
func (blob mediaBlobStub) Delete(string) error                   { return nil }
func (blob mediaBlobStub) ListExpiredTemps(context.Context, time.Time, int) ([]string, error) {
	return nil, nil
}
func (blob mediaBlobStub) DeleteTemp(context.Context, string) error { return nil }

type mediaRepositoryStub struct{ value *ArticleImage }

func (repo mediaRepositoryStub) FindByStorageKey(context.Context, StorageKey) (*ArticleImage, error) {
	return repo.value, nil
}

func TestGetMediaRejectsSameSizeBlobWithWrongDigest(t *testing.T) {
	id, _ := NewArticleImageID("0123456789abcdef0123456789abcdef")
	owner, _ := NewImageOwnerID("abcdef0123456789abcdef0123456789")
	key, _ := NewStorageKey("0123456789abcdef0123456789abcdef.jpg")
	data := []byte("abc")
	digest := sha256.Sum256([]byte("xyz"))
	value, err := NewArticleImage(ArticleImageMetadata{ID: id, OwnerID: owner, StorageKey: key, MediaType: MediaTypeJPEG, ByteSize: int64(len(data)), Width: 1, Height: 1, SHA256: hex.EncodeToString(digest[:])}, time.Unix(1, 0), time.Unix(1, 0).Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err = value.Commit(time.Unix(2, 0)); err != nil {
		t.Fatal(err)
	}
	service := &Service{blob: mediaBlobStub{data: data}}
	_, err = service.getMedia(context.Background(), key, mediaRepositoryStub{value: value}.FindByStorageKey)
	var stableErr *Error
	if !errors.As(err, &stableErr) || stableErr.Code != CodeInternal {
		t.Fatalf("wrong digest error = %v", err)
	}
}
