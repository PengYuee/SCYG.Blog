// Package main is the API process entrypoint.
package main

import (
	"log"
	"os"
)

// main reports that runtime composition belongs to a later implementation gate.
func main() {
	logger := log.New(os.Stderr, "", 0)
	logger.Print("bootstrap not implemented")
	os.Exit(1)
}
