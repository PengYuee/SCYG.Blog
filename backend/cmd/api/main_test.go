package main

import "testing"

func Test_ConfigFile_defaults_to_local_yaml_when_flag_absent(t *testing.T) {
	// Given
	args := []string{}

	// When
	path, err := parseConfigFile(args)

	// Then
	if err != nil || path != "config.local.yaml" {
		t.Fatalf("path=%q err=%v", path, err)
	}
}

func Test_ConfigFile_accepts_explicit_override(t *testing.T) {
	// Given
	args := []string{"-config", "testdata/api.yaml"}

	// When
	path, err := parseConfigFile(args)

	// Then
	if err != nil || path != "testdata/api.yaml" {
		t.Fatalf("path=%q err=%v", path, err)
	}
}
