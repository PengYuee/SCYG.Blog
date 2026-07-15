package postgres

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/blobstorage"
)

func Test_ImageStorage_load_uses_custom_file_limit(t *testing.T) {
	// Given
	const maxFileBytes = 2
	directory := t.TempDir()
	const key = "0123456789abcdef0123456789abcdef.jpg"
	if err := os.WriteFile(filepath.Join(directory, key), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	filesystem, err := blobstorage.New(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = filesystem.Close() })
	policy := module.NewArticleImagePolicy(module.ArticleImagePolicyOptions{MaxFileBytes: maxFileBytes, MaxPixels: 1, MaxDimension: 1, PendingTTL: time.Hour, OrphanGrace: time.Hour})
	storage := NewImageStorage(filesystem, policy)

	// When
	_, err = storage.LoadArticleImage(key)

	// Then
	if err == nil || err.Error() != "图片文件大小不合法" {
		t.Fatalf("超过自定义读取上限应被拒绝：%v", err)
	}
}
