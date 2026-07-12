package blobstorage

import (
	"os"
	"path/filepath"
	"testing"
)

func Test_Filesystem_symlink_cannot_escape_root(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	outside := filepath.Join(parent, "outside")
	os.MkdirAll(root, 0700)
	os.MkdirAll(outside, 0700)
	key := "0123456789abcdef0123456789abcdef.jpg"
	os.WriteFile(filepath.Join(outside, key), []byte("sentinel"), 0600)
	link := filepath.Join(root, key)
	if err := os.Symlink(filepath.Join(outside, key), link); err != nil {
		t.Skipf("当前 Windows 权限不能创建符号链接: %v", err)
	}
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, _, err = store.Open(key); err == nil {
		t.Fatal("os.Root 允许符号链接逃逸")
	}
	content, _ := os.ReadFile(filepath.Join(outside, key))
	if string(content) != "sentinel" {
		t.Fatal("根外哨兵被修改")
	}
}

func Test_Filesystem_open_root_resists_directory_replacement(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	moved := filepath.Join(parent, "moved")
	outside := filepath.Join(parent, "outside")
	os.MkdirAll(root, 0700)
	os.MkdirAll(outside, 0700)
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = os.Rename(root, moved); err != nil {
		t.Skipf("当前平台不允许移动已打开目录: %v", err)
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	key := "0123456789abcdef0123456789abcdef.jpg"
	sentinel := filepath.Join(outside, key)
	os.WriteFile(sentinel, []byte("safe"), 0600)
	if err = os.Symlink(sentinel, filepath.Join(root, key)); err != nil {
		t.Skipf("当前平台不能创建攻击符号链接: %v", err)
	}
	if _, _, err = store.Open(key); err == nil {
		t.Fatal("固定根句柄跟随了替换目录")
	}
	content, _ := os.ReadFile(sentinel)
	if string(content) != "safe" {
		t.Fatal("根外哨兵被修改")
	}
}
