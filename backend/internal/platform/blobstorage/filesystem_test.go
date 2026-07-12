package blobstorage

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func Test_Filesystem_write_commit_open_delete_roundtrip(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	token, meta, err := store.WriteTemp(context.Background(), "0123456789abcdef0123456789abcdef", io.NopCloser(io.LimitReader(zeroReader{}, 12)))
	if err != nil || meta.Size != 12 {
		t.Fatalf("write: %v size=%d", err, meta.Size)
	}
	key := "0123456789abcdef0123456789abcdef.jpg"
	if err = store.CommitTemp(token, key); err != nil {
		t.Fatal(err)
	}
	file, stat, err := store.Open(key)
	if err != nil || stat.Size() != 12 {
		t.Fatalf("open: %v", err)
	}
	file.Close()
	if err = store.Delete(key); err != nil {
		t.Fatal(err)
	}
	if err = store.Delete(key); err != nil {
		t.Fatal(err)
	}
}

func Test_Filesystem_rejects_untrusted_names_and_root_escape(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sentinel := filepath.Join(parent, "sentinel")
	if err = os.WriteFile(sentinel, []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	attacks := []string{"../sentinel", `..\\sentinel`, "/sentinel", `C:\\sentinel`, "%2e%2e", "a/b", `a\\b`, "name:stream"}
	for _, attack := range attacks {
		if _, _, openErr := store.Open(attack); openErr == nil {
			t.Fatalf("攻击被接受: %s", attack)
		}
	}
	content, _ := os.ReadFile(sentinel)
	if string(content) != "safe" {
		t.Fatal("根外哨兵被修改")
	}
}

func Test_Filesystem_lists_only_owned_expired_temps(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	token, _, err := store.WriteTemp(context.Background(), "0123456789abcdef0123456789abcdef", io.NopCloser(io.LimitReader(zeroReader{}, 1)))
	if err != nil {
		t.Fatal(err)
	}
	spoof := filepath.Join(root, "upload-spoof.tmp")
	os.WriteFile(spoof, []byte("x"), 0600)
	old := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(root, token.Name()), old, old)
	got, err := store.ListExpiredTemps(context.Background(), time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name() != token.Name() {
		t.Fatalf("temps=%v", got)
	}
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = 0
	}
	return len(buffer), nil
}

type renameFaultRoot struct{ rootOperations }

func (renameFaultRoot) link(string, string) error { return errors.New("注入重命名故障") }

func Test_Filesystem_rename_failure_keeps_temp_without_final_file(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	token, _, err := store.WriteTemp(context.Background(), "0123456789abcdef0123456789abcdef", io.LimitReader(zeroReader{}, 1))
	if err != nil {
		t.Fatal(err)
	}
	store.root = renameFaultRoot{rootOperations: store.root}
	key := "0123456789abcdef0123456789abcdef.jpg"
	if err = store.CommitTemp(token, key); err == nil {
		t.Fatal("重命名故障未传播")
	}
	if _, statErr := os.Stat(filepath.Join(root, key)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("故障后出现最终文件")
	}
	if _, statErr := os.Stat(filepath.Join(root, token.Name())); statErr != nil {
		t.Fatal("临时文件未保留供清理")
	}
}

type openDeleteFaultRoot struct {
	rootOperations
	openErr   error
	removeErr error
}

func (root openDeleteFaultRoot) open(name string) (fileOperations, error) {
	if root.openErr != nil {
		return nil, root.openErr
	}
	return root.rootOperations.open(name)
}
func (root openDeleteFaultRoot) remove(name string) error {
	if root.removeErr != nil {
		return root.removeErr
	}
	return root.rootOperations.remove(name)
}

type partialReader struct{ sent bool }

func (reader *partialReader) Read(buffer []byte) (int, error) {
	if reader.sent {
		return 0, errors.New("注入读取故障")
	}
	reader.sent = true
	copy(buffer, []byte("partial"))
	return 7, nil
}

func Test_Filesystem_partial_write_removes_temp(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _, err = store.WriteTemp(context.Background(), "0123456789abcdef0123456789abcdef", &partialReader{})
	if err == nil {
		t.Fatal("部分写入故障未传播")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("残留临时文件=%d", len(entries))
	}
}

func Test_Filesystem_open_and_delete_faults_propagate(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := store.root
	injected := errors.New("注入操作故障")
	store.root = openDeleteFaultRoot{rootOperations: base, openErr: injected}
	if _, _, err = store.Open("0123456789abcdef0123456789abcdef.jpg"); !errors.Is(err, injected) {
		t.Fatalf("open=%v", err)
	}
	store.root = openDeleteFaultRoot{rootOperations: base, removeErr: injected}
	if err = store.Delete("0123456789abcdef0123456789abcdef.jpg"); !errors.Is(err, injected) {
		t.Fatalf("delete=%v", err)
	}
}

func Test_Filesystem_commit_collision_preserves_existing_file(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	token, _, err := store.WriteTemp(context.Background(), "0123456789abcdef0123456789abcdef", io.LimitReader(zeroReader{}, 1))
	if err != nil {
		t.Fatal(err)
	}
	key := "0123456789abcdef0123456789abcdef.jpg"
	if err = os.WriteFile(filepath.Join(root, key), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = store.CommitTemp(token, key); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("collision=%v", err)
	}
	content, _ := os.ReadFile(filepath.Join(root, key))
	if string(content) != "original" {
		t.Fatal("碰撞覆盖了原文件")
	}
}
