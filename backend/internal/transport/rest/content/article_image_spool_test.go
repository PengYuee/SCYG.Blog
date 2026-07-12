package content

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"os"
	"runtime"
	"testing"
)

func multipartReaderForSpool(t *testing.T, payload []byte) *multipart.Reader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "untrusted.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	return multipart.NewReader(&body, writer.Boundary())
}

func Test_SpoolUniqueImagePart_removes_transport_temp_after_close(t *testing.T) {
	source, err := (&Handler{tempFiles: osRequestTempOperations{}}).spoolUniqueImagePart(context.Background(), multipartReaderForSpool(t, []byte("payload")))
	if err != nil {
		t.Fatal(err)
	}
	path := source.path
	if info, statErr := os.Stat(path); statErr != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode().Perm(), statErr)
	}
	if err = source.closeAndRemove(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temp remains: %v", err)
	}
}

func Test_SpoolUniqueImagePart_cancellation_leaves_no_request_temp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&Handler{tempFiles: osRequestTempOperations{}}).spoolUniqueImagePart(ctx, multipartReaderForSpool(t, bytes.Repeat([]byte{'x'}, 1024)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	after, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	count := func(entries []os.DirEntry) int {
		total := 0
		for _, entry := range entries {
			if len(entry.Name()) >= 27 && entry.Name()[:27] == "scyg-article-image-request-" {
				total++
			}
		}
		return total
	}
	if count(after) != count(before) {
		t.Fatalf("request temp leaked before=%d after=%d", count(before), count(after))
	}
}
