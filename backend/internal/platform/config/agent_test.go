package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAgentConfigurationEnvironmentAndValidation(t *testing.T) {
	t.Setenv("SCYG_AGENT_ENABLED", "true")
	t.Setenv("SCYG_AGENT_TARGET", "localhost:9099")
	t.Setenv("SCYG_AGENT_BLOG_CONTENT_LISTEN", "127.0.0.1:50100")
	t.Setenv("SCYG_AGENT_UNARY_TIMEOUT", "2s")
	t.Setenv("SCYG_AGENT_SSE_IDLE_TIMEOUT", "40s")
	t.Setenv("SCYG_AGENT_GRPC_SHUTDOWN_TIMEOUT", "8s")
	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Agent()
	if !a.Enabled() || a.Target() != "localhost:9099" || a.BlogContentListen() != "127.0.0.1:50100" || a.UnaryTimeout() != 2*time.Second || a.SSEIdleTimeout() != 40*time.Second || a.GRPCShutdownTimeout() != 8*time.Second {
		t.Fatal("environment settings not applied")
	}
	for _, value := range []string{"agent:\n  enabled: true\n  target: invalid\n", "agent:\n  enabled: true\n  unary_timeout: 0s\n", "agent:\n  enabled: true\n  blog_content_listen: 127.0.0.1:99999\n"} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(Options{File: path, DisableEnvironment: true}); err == nil {
			t.Fatal("enabled invalid integration accepted")
		}
	}
}
