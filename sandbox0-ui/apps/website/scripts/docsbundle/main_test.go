package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBundlerBuildsReleaseAlignedBundle(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	siteDir := filepath.Join(repoRoot, "sandbox0-ui", "apps", "website")
	mustWriteFile(t, filepath.Join(siteDir, "src", "app", "docs", "docs.ts"), "export const docsNavigation = [];\n")
	mustWriteFile(t, filepath.Join(siteDir, "src", "app", "docs", "page.tsx"), "export default function Page() { return null; }\n")
	mustWriteFile(t, filepath.Join(siteDir, "src", "app", "docs", "layout.tsx"), "export default function Layout() { return null; }\n")
	mustWriteFile(t, filepath.Join(siteDir, "src", "app", "docs", "get-started", "page.mdx"), "# Get Started\n\nSandbox0 docs.\n")
	mustWriteFile(t, filepath.Join(siteDir, "src", "generated", "docs", "sandbox0infra-schema.json"), "{\"ok\":true}\n")
	mustWriteFile(t, filepath.Join(siteDir, "out", "index.html"), "<html>root</html>\n")
	mustWriteFile(t, filepath.Join(siteDir, "out", "docs", "get-started", "index.html"), "<html>docs</html>\n")

	result, err := (&bundler{
		repoRoot:   repoRoot,
		siteDir:    siteDir,
		sourceDir:  filepath.Join(siteDir, "src", "app", "docs"),
		generated:  filepath.Join(siteDir, "src", "generated", "docs"),
		exportDir:  filepath.Join(siteDir, "out"),
		bundleRoot: filepath.Join(siteDir, "dist", "docs-bundles"),
		version:    "1.2.3",
		now:        time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
	}).build()
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}

	manifestData, err := os.ReadFile(result.manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var manifest bundleManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	if manifest.Sandbox0Version != "1.2.3" {
		t.Fatalf("unexpected version: %s", manifest.Sandbox0Version)
	}
	if manifest.Bundle.SiteExportDir != "site" {
		t.Fatalf("unexpected site export dir: %s", manifest.Bundle.SiteExportDir)
	}
	if len(manifest.Documents) != 1 {
		t.Fatalf("expected 1 document, got %d", len(manifest.Documents))
	}
	if manifest.Documents[0].Route != "/docs/get-started" {
		t.Fatalf("unexpected route: %s", manifest.Documents[0].Route)
	}
	if manifest.Documents[0].Title != "Get Started" {
		t.Fatalf("unexpected title: %s", manifest.Documents[0].Title)
	}

	checksums, err := os.ReadFile(filepath.Join(result.bundleDir, "SHA256SUMS"))
	if err != nil {
		t.Fatalf("read checksums: %v", err)
	}
	if !strings.Contains(string(checksums), "manifest.json") {
		t.Fatalf("checksum file does not reference manifest.json")
	}
	if !strings.Contains(string(checksums), "site/docs/get-started/index.html") {
		t.Fatalf("checksum file does not reference site export")
	}

	authority, err := os.ReadFile(filepath.Join(result.bundleDir, "AUTHORITY.md"))
	if err != nil {
		t.Fatalf("read authority note: %v", err)
	}
	if !strings.Contains(string(authority), "pkg/apispec/openapi.yaml") {
		t.Fatalf("authority note missing OpenAPI authority statement")
	}

	entries := tarEntries(t, result.archivePath)
	if _, ok := entries["sandbox0-docs-bundle-1.2.3/manifest.json"]; !ok {
		t.Fatalf("archive missing manifest.json")
	}
	if _, ok := entries["sandbox0-docs-bundle-1.2.3/docs-source/get-started/page.mdx"]; !ok {
		t.Fatalf("archive missing source docs")
	}
	if _, ok := entries["sandbox0-docs-bundle-1.2.3/site/docs/get-started/index.html"]; !ok {
		t.Fatalf("archive missing site export")
	}
	if _, ok := entries["sandbox0-docs-bundle-1.2.3/generated-docs/sandbox0infra-schema.json"]; !ok {
		t.Fatalf("archive missing generated docs asset")
	}

	archiveSHA, err := os.ReadFile(result.archiveSHA256Path)
	if err != nil {
		t.Fatalf("read archive sha256: %v", err)
	}
	if !strings.Contains(string(archiveSHA), "sandbox0-docs-bundle-1.2.3.tar.gz") {
		t.Fatalf("archive sha256 file missing archive name")
	}
}

func tarEntries(t *testing.T, path string) map[string]struct{} {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open tar.gz: %v", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("create gzip reader: %v", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	entries := make(map[string]struct{})
	for {
		header, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("read tar entry: %v", err)
		}
		entries[header.Name] = struct{}{}
	}
	return entries
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
