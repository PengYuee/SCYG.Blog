package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_probe_succeeds_when_live_and_ready_are_healthy(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// When
	err := probe(context.Background(), server.Client(), server.URL)

	// Then
	if err != nil {
		t.Fatalf("健康端点均正常时不应失败：%v", err)
	}
}

func Test_probe_fails_closed_when_ready_is_unhealthy(t *testing.T) {
	// Given
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/ready" {
			response.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		response.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// When
	err := probe(context.Background(), server.Client(), server.URL)

	// Then
	if err == nil {
		t.Fatal("就绪端点异常时健康检查必须失败关闭")
	}
}
