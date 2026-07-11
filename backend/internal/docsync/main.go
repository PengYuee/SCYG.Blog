// Command docsync synchronizes the API documentation copy with the authoritative contract.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
)

const (
	authoritativeSpec = "api/openapi.yaml"
	embeddedSpec      = "internal/transport/rest/apidocs/assets/openapi.yaml"
)

// main copies the authoritative specification or rejects drift in check mode.
func main() {
	check := flag.Bool("check", false, "reject documentation specification drift")
	flag.Parse()
	source, err := os.ReadFile(authoritativeSpec)
	if err != nil {
		fail("read authoritative specification", err)
	}
	if *check {
		target, readErr := os.ReadFile(embeddedSpec)
		if readErr != nil {
			fail("read embedded specification", readErr)
		}
		if !bytes.Equal(source, target) {
			fail("embedded specification is stale; run task api:docs:sync", nil)
		}
		return
	}
	// The destination is a compile-time repository path.
	//nolint:gosec
	writeErr := os.WriteFile(embeddedSpec, source, 0o600)
	if writeErr != nil {
		fail("write embedded specification", writeErr)
	}
}

func fail(message string, err error) {
	if err == nil {
		fmt.Fprintln(os.Stderr, message)
	} else {
		fmt.Fprintf(os.Stderr, "%s: %v\n", message, err)
	}
	os.Exit(1)
}
