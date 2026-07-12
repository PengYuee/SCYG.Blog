package blobstorage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const tempPrefix = ".article-image-"

// TempToken 是存储层生成且只能由本实例提交的临时文件标识。
type TempToken struct{ name string }

func (token TempToken) Name() string { return token.name }

// TempMetadata 描述已完整写入并同步的临时对象。
type TempMetadata struct{ Size int64 }

// TempEntry 描述一个可清理临时对象。
type TempEntry struct {
	name       string
	ModifiedAt time.Time
}

func (entry TempEntry) Name() string { return entry.name }

// ReadSeekCloser 是可定位读取且可关闭的 blob 句柄。
type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

type fileOperations interface {
	io.Writer
	ReadSeekCloser
	Sync() error
	Stat() (fs.FileInfo, error)
}
type rootOperations interface {
	openFile(string, int, os.FileMode) (fileOperations, error)
	open(string) (fileOperations, error)
	link(string, string) error
	remove(string) error
	filesystem() fs.FS
	syncDirectory() error
	close() error
}
type osRootOperations struct{ root *os.Root }

func (ops osRootOperations) openFile(name string, flag int, mode os.FileMode) (fileOperations, error) {
	return ops.root.OpenFile(name, flag, mode)
}
func (ops osRootOperations) open(name string) (fileOperations, error) { return ops.root.Open(name) }
func (ops osRootOperations) link(oldName, newName string) error {
	return ops.root.Link(oldName, newName)
}
func (ops osRootOperations) remove(name string) error { return ops.root.Remove(name) }
func (ops osRootOperations) filesystem() fs.FS        { return ops.root.FS() }
func (ops osRootOperations) close() error             { return ops.root.Close() }
func (ops osRootOperations) syncDirectory() error {
	// Windows 的目录句柄不能通过 os.File.Sync 刷新；硬链接与删除仍由 NTFS 原子持久化。
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := ops.root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}

// CommitCleanupError 表示最终文件已提交，但临时链接清理仍需重试。
type CommitCleanupError struct{ Err error }

// Error 返回不会诱导调用方删除最终文件的中文描述。
func (failure *CommitCleanupError) Error() string {
	return "最终图片已提交，临时文件清理待重试：" + failure.Err.Error()
}

// Unwrap 保留底层临时清理故障。
func (failure *CommitCleanupError) Unwrap() error { return failure.Err }

// Committed 告知调用方不得补偿删除最终目标。
func (*CommitCleanupError) Committed() bool { return true }

// Filesystem 在一个固定 os.Root 内管理 blob。
type Filesystem struct{ root rootOperations }

// New 验证并创建绝对存储根。
func New(directory string) (*Filesystem, error) {
	clean := filepath.Clean(strings.TrimSpace(directory))
	if !filepath.IsAbs(clean) {
		return nil, errors.New("图片存储根必须是绝对路径")
	}
	if err := os.MkdirAll(clean, 0700); err != nil {
		return nil, fmt.Errorf("创建图片存储根：%w", err)
	}
	root, err := os.OpenRoot(clean)
	if err != nil {
		return nil, fmt.Errorf("打开图片存储根：%w", err)
	}
	return &Filesystem{root: osRootOperations{root: root}}, nil
}

// Close 关闭固定根句柄。
func (store *Filesystem) Close() error { return store.root.close() }

// WriteTemp 将流复制到同目录唯一临时文件并同步。
func (store *Filesystem) WriteTemp(ctx context.Context, id string, reader io.Reader) (TempToken, TempMetadata, error) {
	if !lowerHex(id, 32) {
		return TempToken{}, TempMetadata{}, errors.New("图片标识不合法")
	}
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return TempToken{}, TempMetadata{}, fmt.Errorf("生成临时标识：%w", err)
	}
	name := tempPrefix + id + "-" + hex.EncodeToString(random) + ".tmp"
	file, err := store.root.openFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return TempToken{}, TempMetadata{}, fmt.Errorf("创建临时图片：%w", err)
	}
	size, copyErr := copyContext(ctx, file, reader)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err = errors.Join(copyErr, syncErr, closeErr); err != nil {
		_ = store.root.remove(name)
		return TempToken{}, TempMetadata{}, fmt.Errorf("写入临时图片：%w", err)
	}
	return TempToken{name: name}, TempMetadata{Size: size}, nil
}

// CommitTemp 以原子硬链接 no-replace 语义提交临时对象。
func (store *Filesystem) CommitTemp(token TempToken, key string) error {
	if !validTemp(token.name) || !validKey(key) {
		return errors.New("图片存储名称不合法")
	}
	if err := store.root.link(token.name, key); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("提交临时图片：%w", err)
		}
		same, compareErr := store.sameFile(token.name, key)
		if compareErr != nil {
			return fmt.Errorf("确认已提交图片：%w", compareErr)
		}
		if !same {
			return fmt.Errorf("目标图片已存在：%w", fs.ErrExist)
		}
	}
	if err := store.root.remove(token.name); err != nil {
		return &CommitCleanupError{Err: fmt.Errorf("移除已提交临时图片：%w", err)}
	}
	if err := store.root.syncDirectory(); err != nil {
		return &CommitCleanupError{Err: fmt.Errorf("同步图片目录：%w", err)
	}
	return nil
}

func (store *Filesystem) sameFile(leftName, rightName string) (bool, error) {
	left, err := store.root.open(leftName)
	if err != nil {
		return false, err
	}
	leftInfo, leftErr := left.Stat()
	leftCloseErr := left.Close()
	if err = errors.Join(leftErr, leftCloseErr); err != nil {
		return false, err
	}
	right, err := store.root.open(rightName)
	if err != nil {
		return false, err
	}
	rightInfo, rightErr := right.Stat()
	rightCloseErr := right.Close()
	if err = errors.Join(rightErr, rightCloseErr); err != nil {
		return false, err
	}
	return os.SameFile(leftInfo, rightInfo), nil
}

// DeleteTemp 严格、幂等删除本实现拥有的临时文件。
func (store *Filesystem) DeleteTemp(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validTemp(name) {
		return errors.New("临时图片名称不合法")
	}
	if err := store.root.remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("删除临时图片：%w", err)
	}
	return nil
}

// Open 安全打开普通最终图片。
func (store *Filesystem) Open(key string) (ReadSeekCloser, fs.FileInfo, error) {
	if !validKey(key) {
		return nil, nil, errors.New("图片存储键不合法")
	}
	file, err := store.root.open(key)
	if err != nil {
		return nil, nil, fmt.Errorf("打开图片：%w", err)
	}
	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, fmt.Errorf("读取图片状态：%w", err)
	}
	if !stat.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, errors.New("图片不是普通文件")
	}
	return file, stat, nil
}

// Delete 幂等删除最终图片。
func (store *Filesystem) Delete(key string) error {
	if !validKey(key) {
		return errors.New("图片存储键不合法")
	}
	if err := store.root.remove(key); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("删除图片：%w", err)
	}
	return nil
}

// ListExpiredTemps 按修改时间和名称确定性列出本实现拥有的临时文件。
func (store *Filesystem) ListExpiredTemps(ctx context.Context, cutoff time.Time, limit int) ([]TempEntry, error) {
	if limit < 1 {
		return []TempEntry{}, nil
	}
	entries, err := fs.ReadDir(store.root.filesystem(), ".")
	if err != nil {
		return nil, fmt.Errorf("枚举临时图片：%w", err)
	}
	result := make([]TempEntry, 0, limit)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !validTemp(entry.Name()) || !entry.Type().IsRegular() {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, fmt.Errorf("读取临时图片状态：%w", infoErr)
		}
		if !info.ModTime().After(cutoff) {
			result = append(result, TempEntry{name: entry.Name(), ModifiedAt: info.ModTime()})
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].ModifiedAt.Equal(result[right].ModifiedAt) {
			return result[left].name < result[right].name
		}
		return result[left].ModifiedAt.Before(result[right].ModifiedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
