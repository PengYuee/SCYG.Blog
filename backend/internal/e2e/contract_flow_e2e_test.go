//go:build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func Test_ContractFlow_article_payload_uses_numeric_status(t *testing.T) {
	payload := articleCreatePayload("contract", 1, 2, articleStatusDraft)
	var document struct {
		Status        int     `json:"status"`
		ArticleTypeID int64   `json:"article_type_id"`
		TagIDs        []int64 `json:"tag_ids"`
	}
	if err := json.Unmarshal([]byte(payload), &document); err != nil {
		t.Fatalf("解析 Article payload 失败：%v", err)
	}
	if document.Status != 1 || document.ArticleTypeID != 1 || len(document.TagIDs) != 1 || document.TagIDs[0] != 2 {
		t.Fatalf("Article payload 不符合契约：%+v", document)
	}
}

func Test_ContractFlow_stale_replays_previous_strong_etag(t *testing.T) {
	replay, err := staleReplayETag(`"1"`, `"2"`)
	if err != nil {
		t.Fatalf("构造 stale 序列失败：%v", err)
	}
	if replay != `"1"` {
		t.Fatalf("重放 ETag=%s，期望旧值", replay)
	}
	if _, err = staleReplayETag(`"1"`, `"1"`); err == nil {
		t.Fatal("未更新版本不应构成 stale 前置")
	}
}

func Test_ContractFlow_windows_control_file_cancels_context(t *testing.T) {
	if terminationUsesNativeSignal() {
		t.Skip("Unix 使用真实 SIGTERM，不执行 Windows 控制文件回归")
	}
	marker := filepath.Join(t.TempDir(), "control")
	ctx, cancel := signalContext(context.Background(), marker)
	defer cancel()
	if err := requestGracefulStop(nil, marker); err != nil {
		t.Fatalf("写入 Windows 优雅关闭控制失败：%v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("Windows 控制文件未触发 context cancellation")
	}
	if _, err := os.Stat(marker + ".stop"); err != nil {
		t.Fatalf("Windows 控制文件缺失：%v", err)
	}
}
