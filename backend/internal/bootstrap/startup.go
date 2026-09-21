package bootstrap

import (
	"fmt"
	"net"
	"net/url"
)

const (
	// docsPath 是 API 文档页面的固定路径。
	docsPath = "/docs"
	// openAPIPath 是 OpenAPI 定义的固定路径。
	openAPIPath = "/openapi.yaml"
)

// startupAttributes 构造服务就绪日志的完整属性集。
func startupAttributes(address net.Addr, docsEnabled bool) ([]any, error) {
	serviceURL, err := startupURL(address, "")
	if err != nil {
		return nil, err
	}
	attributes := []any{"service_url", serviceURL}
	if docsEnabled {
		docsURL, docsErr := startupURL(address, docsPath)
		if docsErr != nil {
			return nil, docsErr
		}
		openAPIURL, openAPIErr := startupURL(address, openAPIPath)
		if openAPIErr != nil {
			return nil, openAPIErr
		}
		attributes = append(attributes, "docs_url", docsURL, "openapi_url", openAPIURL)
	}
	return attributes, nil
}

// startupURL 从实际监听地址构造可点击 URL，并将通配地址归一化为本机地址。
func startupURL(address net.Addr, path string) (string, error) {
	host, port, err := net.SplitHostPort(address.String())
	if err != nil {
		return "", fmt.Errorf("拆分监听地址 %q: %w", address.String(), err)
	}
	switch host {
	case "0.0.0.0", "":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return (&url.URL{Scheme: "http", Host: net.JoinHostPort(host, port), Path: path}).String(), nil
}
