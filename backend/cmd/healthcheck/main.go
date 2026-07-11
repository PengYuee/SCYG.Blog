// Command healthcheck 同时验证 API 存活与就绪端点。
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	// defaultHealthBaseURL 是容器内部 API 的默认探测地址。
	defaultHealthBaseURL = "http://127.0.0.1:8080"
	// healthTimeout 限制完整健康探测的最长时间。
	healthTimeout = 4 * time.Second
)

// main 执行有界健康探测并以进程退出码报告结果。
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "API 健康检查失败："+err.Error())
		os.Exit(1)
	}
}

// run 依次请求存活与就绪端点，任一非 200 响应均失败关闭。
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), healthTimeout)
	defer cancel()
	client := &http.Client{Timeout: healthTimeout}
	return probe(ctx, client, defaultHealthBaseURL)
}

// probe 请求指定 API 的存活与就绪端点，任一非 200 响应均失败关闭。
func probe(ctx context.Context, client *http.Client, baseURL string) error {
	for _, path := range []string{"/live", "/ready"} {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
		if err != nil {
			return fmt.Errorf("创建 %s 请求失败：%w", path, err)
		}
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("请求 %s 失败：%w", path, err)
		}
		closeErr := response.Body.Close()
		if closeErr != nil {
			return fmt.Errorf("关闭 %s 响应失败：%w", path, closeErr)
		}
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("%s 返回状态码 %d", path, response.StatusCode)
		}
	}
	return nil
}
