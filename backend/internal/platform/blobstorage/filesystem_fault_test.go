package blobstorage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type faultFile struct {
	fileOperations
	writeErr error
	syncErr  error
	closeErr error
}

func (file faultFile) Write(buffer []byte) (int, error) {
	if file.writeErr != nil {
		return 0, file.writeErr
	}
	return file.fileOperations.Write(buffer)
}
func (file faultFile) Sync() error {
	if file.syncErr != nil {
		return file.syncErr
	}
	return file.fileOperations.Sync()
}
func (file faultFile) Close() error {
	realErr := file.fileOperations.Close()
	return errors.Join(realErr, file.closeErr)
}

type writeFaultRoot struct {
	rootOperations
	writeErr     error
	syncErr      error
	closeErr     error
	directoryErr error
}

func (root writeFaultRoot) openFile(name string, flag int, mode os.FileMode) (fileOperations, error) {
	file, err := root.rootOperations.openFile(name, flag, mode)
	if err != nil {
		return nil, err
	}
	return faultFile{fileOperations: file, writeErr: root.writeErr, syncErr: root.syncErr, closeErr: root.closeErr}, nil
}
func (root writeFaultRoot) syncDirectory() error {
	if root.directoryErr != nil {
		return root.directoryErr
	}
	return root.rootOperations.syncDirectory()
}

func Test_Filesystem_injected_write_sync_close_and_directory_faults(t *testing.T) {
	faults := []struct {
		name      string
		configure func(rootOperations) rootOperations
		commit    bool
	}{
		{"write", func(root rootOperations) rootOperations {
			return writeFaultRoot{rootOperations: root, writeErr: errors.New("write")}
		}, false},
		{"sync", func(root rootOperations) rootOperations {
			return writeFaultRoot{rootOperations: root, syncErr: errors.New("sync")}
		}, false},
		{"close", func(root rootOperations) rootOperations {
			return writeFaultRoot{rootOperations: root, closeErr: errors.New("close")}
		}, false},
		{"directory sync", func(root rootOperations) rootOperations {
			return writeFaultRoot{rootOperations: root, directoryErr: errors.New("directory")}
		}, true},
	}
	for _, fault := range faults {
		t.Run(fault.name, func(t *testing.T) {
			store, err := New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			store.root = fault.configure(store.root)
			token, _, writeErr := store.WriteTemp(context.Background(), "0123456789abcdef0123456789abcdef", strings.NewReader("payload"))
			if !fault.commit {
				if writeErr == nil {
					t.Fatal("故障未传播")
				}
				return
			}
			if writeErr != nil {
				t.Fatal(writeErr)
			}
			if err = store.CommitTemp(token, "0123456789abcdef0123456789abcdef.jpg"); err == nil {
				t.Fatal("目录同步故障未传播")
			}
		})
	}
}

func Test_Filesystem_commit_is_atomic_no_replace_under_race(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	id := "0123456789abcdef0123456789abcdef"
	first, _, _ := store.WriteTemp(context.Background(), id, strings.NewReader("first"))
	second, _, _ := store.WriteTemp(context.Background(), id, strings.NewReader("second"))
	key := id + ".jpg"
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, token := range []TempToken{first, second} {
		wait.Add(1)
		go func() { defer wait.Done(); <-start; results <- store.CommitTemp(token, key) }()
	}
	close(start)
	wait.Wait()
	close(results)
	success, exists := 0, 0
	for commitErr := range results {
		if commitErr == nil {
			success++
		} else if errors.Is(commitErr, fs.ErrExist) {
			exists++
		} else {
			t.Fatal(commitErr)
		}
	}
	if success != 1 || exists != 1 {
		t.Fatalf("success=%d exists=%d", success, exists)
	}
	payload, _ := os.ReadFile(filepath.Join(root, key))
	if string(payload) != "first" && string(payload) != "second" {
		t.Fatalf("目标被覆盖: %q", payload)
	}
}

func Test_New_rejects_drive_relative_directory(t *testing.T) {
	if _, err := New(`C:relative`); err == nil {
		t.Fatal("接受了驱动器相对路径")
	}
}

func Test_CommitTemp_directory_sync_failure_reports_committed_final(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	token, _, err := store.WriteTemp(context.Background(), "0123456789abcdef0123456789abcdef", strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	store.root = writeFaultRoot{rootOperations: store.root, directoryErr: errors.New("directory sync")}
	key := "0123456789abcdef0123456789abcdef.jpg"
	err = store.CommitTemp(token, key)
	var committed *CommitCleanupError
	if !errors.As(err, &committed) || !committed.Committed() {
		t.Fatalf("error=%v", err)
	}
	payload, readErr := os.ReadFile(filepath.Join(root, key))
	if readErr != nil || string(payload) != "payload" {
		t.Fatalf("payload=%q err=%v", payload, readErr)
	}
}
