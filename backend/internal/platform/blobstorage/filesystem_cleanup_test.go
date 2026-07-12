package blobstorage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type removeOnceFaultRoot struct {
	rootOperations
	failed bool
}

func (root *removeOnceFaultRoot) remove(name string) error {
	if !root.failed {
		root.failed = true
		return errors.New("注入首次删除故障")
	}
	return root.rootOperations.remove(name)
}

func Test_CommitTemp_retry_cleans_same_file_after_remove_failure(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fault := &removeOnceFaultRoot{rootOperations: store.root}
	store.root = fault
	id := "0123456789abcdef0123456789abcdef"
	token, _, err := store.WriteTemp(context.Background(), id, strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	key := id + ".jpg"
	err = store.CommitTemp(token, key)
	var cleanup *CommitCleanupError
	if !errors.As(err, &cleanup) || !cleanup.Committed() {
		t.Fatalf("首次错误=%v", err)
	}
	if _, err = os.Stat(filepath.Join(root, token.Name())); err != nil {
		t.Fatal("temp应保留")
	}
	payload, _ := os.ReadFile(filepath.Join(root, key))
	if string(payload) != "payload" {
		t.Fatal("final字节错误")
	}
	if err = store.CommitTemp(token, key); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, token.Name())); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("重试未清temp")
	}
}

func Test_CommitTemp_retry_rejects_different_existing_target(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	id := "0123456789abcdef0123456789abcdef"
	token, _, _ := store.WriteTemp(context.Background(), id, strings.NewReader("temp"))
	key := id + ".jpg"
	os.WriteFile(filepath.Join(root, key), []byte("attacker"), 0600)
	if err = store.CommitTemp(token, key); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("错误=%v", err)
	}
	payload, _ := os.ReadFile(filepath.Join(root, key))
	if string(payload) != "attacker" {
		t.Fatal("覆盖攻击者目标")
	}
	if _, err = os.Stat(filepath.Join(root, token.Name())); err != nil {
		t.Fatal("错误删除temp")
	}
}

func Test_DeleteTemp_is_strict_idempotent_and_context_aware(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	id := "0123456789abcdef0123456789abcdef"
	token, _, _ := store.WriteTemp(context.Background(), id, strings.NewReader("x"))
	if err = store.DeleteTemp(context.Background(), token.Name()); err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteTemp(context.Background(), token.Name()); err != nil {
		t.Fatal(err)
	}
	for _, attack := range []string{"upload-spoof.tmp", "../outside", `..\\outside`, "/outside", id + ".jpg"} {
		if err = store.DeleteTemp(context.Background(), attack); err == nil {
			t.Fatalf("接受攻击名=%s", attack)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = store.DeleteTemp(ctx, token.Name()); !errors.Is(err, context.Canceled) {
		t.Fatalf("context=%v", err)
	}
}

func Test_DeleteTemp_propagates_fault_and_closes_enumeration_loop(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	id := "0123456789abcdef0123456789abcdef"
	token, _, _ := store.WriteTemp(context.Background(), id, strings.NewReader("x"))
	outside := filepath.Join(parent, "outside")
	os.WriteFile(outside, []byte("safe"), 0600)
	old := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(root, token.Name()), old, old)
	entries, err := store.ListExpiredTemps(context.Background(), time.Now(), 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
	base := store.root
	injected := errors.New("注入temp删除故障")
	store.root = openDeleteFaultRoot{rootOperations: base, removeErr: injected}
	if err = store.DeleteTemp(context.Background(), entries[0].Name()); !errors.Is(err, injected) {
		t.Fatalf("fault=%v", err)
	}
	store.root = base
	if err = store.DeleteTemp(context.Background(), entries[0].Name()); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(outside)
	if string(content) != "safe" {
		t.Fatal("根外哨兵被修改")
	}
}
