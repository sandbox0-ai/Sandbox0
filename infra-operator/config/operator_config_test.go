/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOperatorConfigDefault(t *testing.T) {
	// Use a non-existent path to trigger default values
	config, err := LoadOperatorConfig("/non/existent/path")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if config.ImageRepo != DefaultImageRepo {
		t.Fatalf("expected default image repo %q, got %q", DefaultImageRepo, config.ImageRepo)
	}
}

func TestLoadOperatorConfigOverridesImageRepo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "operator-config-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")
	configData := []byte("imageRepo: custom/infra\n")
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}

	config, err := LoadOperatorConfig(configPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if config.ImageRepo != "custom/infra" {
		t.Fatalf("expected overridden image repo, got %q", config.ImageRepo)
	}
}
