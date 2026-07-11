package deliverytest_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// backendRoot 返回 delivery 配置所在的后端模块根目录。
func backendRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位 delivery 测试文件")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
}

// readDeliveryFile 读取后端模块内的交付配置并在缺失时立即失败。
func readDeliveryFile(t *testing.T, relativePath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(backendRoot(t), relativePath))
	if err != nil {
		t.Fatalf("读取交付配置 %s 失败：%v", relativePath, err)
	}
	return string(content)
}
