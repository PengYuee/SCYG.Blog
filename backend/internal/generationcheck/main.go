// Command generationcheck rejects OpenAPI generation drift without modifying committed output.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	committedOutput = "internal/generated/openapi/openapi.gen.go"
	generatorModule = "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.7.2"
)

// main runs the deterministic generation check and reports failures without a shell wrapper.
func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "OpenAPI generation drift check: %v\n", err)
		os.Exit(1)
	}
}

// run generates into an isolated directory and compares exact bytes with the committed artifact.
func run() error {
	tempDirectory, err := os.MkdirTemp("", "scyg-openapi-check-")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(tempDirectory); cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "remove temporary generation directory: %v\n", cleanupErr)
		}
	}()

	config, err := os.ReadFile("oapi-codegen.yaml")
	if err != nil {
		return fmt.Errorf("read generator config: %w", err)
	}
	tempOutput := filepath.ToSlash(filepath.Join(tempDirectory, "openapi.gen.go"))
	tempConfig := filepath.Join(tempDirectory, "oapi-codegen.yaml")
	updatedConfig := strings.Replace(string(config), "output: "+committedOutput, "output: "+tempOutput, 1)
	if updatedConfig == string(config) {
		return fmt.Errorf("generator output setting %q not found", committedOutput)
	}
	// The config path is created inside the process-owned temporary directory.
	//nolint:gosec
	if writeErr := os.WriteFile(tempConfig, []byte(updatedConfig), 0o600); writeErr != nil {
		return fmt.Errorf("write temporary generator config: %w", writeErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// The executable, module version, and specification path are compile-time constants.
	//nolint:gosec
	command := exec.CommandContext(ctx, "go", "run", generatorModule, "--config", tempConfig, "api/openapi.yaml")
	if output, commandErr := command.CombinedOutput(); commandErr != nil {
		return fmt.Errorf("run pinned generator: %w: %s", commandErr, bytes.TrimSpace(output))
	}
	committed, err := os.ReadFile(committedOutput)
	if err != nil {
		return fmt.Errorf("read committed output: %w", err)
	}
	// The output path is created inside the process-owned temporary directory.
	//nolint:gosec
	generated, err := os.ReadFile(tempOutput)
	if err != nil {
		return fmt.Errorf("read temporary output: %w", err)
	}
	if !bytes.Equal(committed, generated) {
		return fmt.Errorf("%s is stale; run task api:generate", committedOutput)
	}
	return nil
}
