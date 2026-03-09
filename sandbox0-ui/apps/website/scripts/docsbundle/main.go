package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	docsBundleSchemaVersion = "1"
	docsBundleKind          = "sandbox0.docs-bundle"
	timeFormatRFC3339       = "2006-01-02T15:04:05Z"
)

type bundleManifest struct {
	SchemaVersion   string          `json:"schemaVersion"`
	Kind            string          `json:"kind"`
	Sandbox0Version string          `json:"sandbox0Version"`
	GeneratedAt     string          `json:"generatedAt"`
	SourcePriority  []string        `json:"sourcePriority"`
	Authority       bundleAuthority `json:"authorityBoundary"`
	Bundle          bundleLayout    `json:"bundle"`
	Documents       []docsDocument  `json:"documents"`
}

type bundleAuthority struct {
	AuthoritativeSources []string `json:"authoritativeSources"`
	BundledDocsRole      string   `json:"bundledDocsRole"`
}

type bundleLayout struct {
	Root             string   `json:"root"`
	SourceDocsDir    string   `json:"sourceDocsDir"`
	GeneratedDocsDir string   `json:"generatedDocsDir,omitempty"`
	SiteExportDir    string   `json:"siteExportDir"`
	NavigationFiles  []string `json:"navigationFiles"`
	ChecksumFile     string   `json:"checksumFile"`
	ArchiveFile      string   `json:"archiveFile"`
	ArchiveSHA256    string   `json:"archiveSha256File"`
}

type docsDocument struct {
	Title      string `json:"title"`
	Route      string `json:"route"`
	SourcePath string `json:"sourcePath"`
	SHA256     string `json:"sha256"`
}

type bundler struct {
	repoRoot   string
	siteDir    string
	sourceDir  string
	generated  string
	exportDir  string
	bundleRoot string
	version    string
	now        time.Time
}

type bundleResult struct {
	bundleDir         string
	archivePath       string
	archiveSHA256Path string
	manifestPath      string
}

func main() {
	var (
		version    = flag.String("version", "", "Sandbox0 release version for the docs bundle")
		repoRoot   = flag.String("repo-root", "", "Repository root path")
		siteDir    = flag.String("site-dir", "", "Website app root path")
		sourceDir  = flag.String("source-dir", "", "Docs source directory")
		generated  = flag.String("generated-dir", "", "Generated docs assets directory")
		exportDir  = flag.String("export-dir", "", "Static website export directory")
		bundleRoot = flag.String("bundle-root", "", "Output directory for generated docs bundles")
	)
	flag.Parse()

	if strings.TrimSpace(*version) == "" {
		fail(fmt.Errorf("missing required -version"))
	}

	root := strings.TrimSpace(*repoRoot)
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fail(err)
		}
		root = cwd
	}

	site := firstNonEmpty(*siteDir, filepath.Join(root, "sandbox0-ui", "apps", "website"))
	source := firstNonEmpty(*sourceDir, filepath.Join(site, "src", "app", "docs"))
	generatedDir := firstNonEmpty(*generated, filepath.Join(site, "src", "generated", "docs"))
	exported := firstNonEmpty(*exportDir, filepath.Join(site, "out"))
	outputRoot := firstNonEmpty(*bundleRoot, filepath.Join(site, "dist", "docs-bundles"))

	result, err := (&bundler{
		repoRoot:   root,
		siteDir:    site,
		sourceDir:  source,
		generated:  generatedDir,
		exportDir:  exported,
		bundleRoot: outputRoot,
		version:    strings.TrimSpace(*version),
		now:        bundleTime(),
	}).build()
	if err != nil {
		fail(err)
	}

	fmt.Printf("bundle_dir=%s\n", result.bundleDir)
	fmt.Printf("manifest=%s\n", result.manifestPath)
	fmt.Printf("archive=%s\n", result.archivePath)
	fmt.Printf("archive_sha256=%s\n", result.archiveSHA256Path)
}

func (b *bundler) build() (*bundleResult, error) {
	if err := requireDir(b.sourceDir); err != nil {
		return nil, err
	}
	if err := requireDir(b.exportDir); err != nil {
		return nil, err
	}

	bundleBaseName := "sandbox0-docs-bundle-" + b.version
	bundleDir := filepath.Join(b.bundleRoot, bundleBaseName)
	if err := os.RemoveAll(bundleDir); err != nil {
		return nil, fmt.Errorf("remove previous bundle dir: %w", err)
	}
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		return nil, fmt.Errorf("create bundle dir: %w", err)
	}

	sourceDest := filepath.Join(bundleDir, "docs-source")
	if err := copyDir(b.sourceDir, sourceDest); err != nil {
		return nil, fmt.Errorf("copy docs sources: %w", err)
	}

	generatedDest := filepath.Join(bundleDir, "generated-docs")
	generatedExists, err := dirExists(b.generated)
	if err != nil {
		return nil, err
	}
	if generatedExists {
		if err := copyDir(b.generated, generatedDest); err != nil {
			return nil, fmt.Errorf("copy generated docs assets: %w", err)
		}
	}

	siteDest := filepath.Join(bundleDir, "site")
	if err := copyDir(b.exportDir, siteDest); err != nil {
		return nil, fmt.Errorf("copy site export: %w", err)
	}

	documents, err := collectDocuments(sourceDest)
	if err != nil {
		return nil, err
	}

	archivePath := filepath.Join(b.bundleRoot, bundleBaseName+".tar.gz")
	archiveSHA256Path := archivePath + ".sha256"
	manifest := bundleManifest{
		SchemaVersion:   docsBundleSchemaVersion,
		Kind:            docsBundleKind,
		Sandbox0Version: b.version,
		GeneratedAt:     b.now.UTC().Format(timeFormatRFC3339),
		SourcePriority: []string{
			"source-code",
			"pkg/apispec/openapi.yaml",
			"s0-cli-help-and-implementation",
			"bundled-docs",
			"hosted-website-docs",
		},
		Authority: bundleAuthority{
			AuthoritativeSources: []string{
				"source-code",
				"pkg/apispec/openapi.yaml",
				"s0-cli-help-and-implementation",
			},
			BundledDocsRole: "Bundled docs are release-matched reference material and must not override source code, OpenAPI, or CLI behavior when they disagree.",
		},
		Bundle: bundleLayout{
			Root:             bundleBaseName,
			SourceDocsDir:    "docs-source",
			GeneratedDocsDir: generatedDirField(generatedExists),
			SiteExportDir:    "site",
			NavigationFiles:  []string{"docs-source/docs.ts", "docs-source/page.tsx", "docs-source/layout.tsx"},
			ChecksumFile:     "SHA256SUMS",
			ArchiveFile:      filepath.Base(archivePath),
			ArchiveSHA256:    filepath.Base(archiveSHA256Path),
		},
		Documents: documents,
	}

	manifestPath := filepath.Join(bundleDir, "manifest.json")
	if err := writeJSON(manifestPath, manifest); err != nil {
		return nil, err
	}

	if err := os.WriteFile(filepath.Join(bundleDir, "AUTHORITY.md"), []byte(authorityMarkdown(b.version)), 0o644); err != nil {
		return nil, fmt.Errorf("write authority note: %w", err)
	}

	if err := writeChecksums(filepath.Join(bundleDir, "SHA256SUMS"), bundleDir); err != nil {
		return nil, err
	}
	if err := createTarGz(archivePath, b.bundleRoot, bundleBaseName, b.now); err != nil {
		return nil, err
	}
	if err := writeArchiveSHA256(archiveSHA256Path, archivePath); err != nil {
		return nil, err
	}

	return &bundleResult{
		bundleDir:         bundleDir,
		archivePath:       archivePath,
		archiveSHA256Path: archiveSHA256Path,
		manifestPath:      manifestPath,
	}, nil
}

func bundleTime() time.Time {
	sourceDateEpoch := strings.TrimSpace(os.Getenv("SOURCE_DATE_EPOCH"))
	if sourceDateEpoch == "" {
		return time.Now().UTC()
	}
	seconds, err := strconv.ParseInt(sourceDateEpoch, 10, 64)
	if err != nil {
		fail(fmt.Errorf("parse SOURCE_DATE_EPOCH: %w", err))
	}
	return time.Unix(seconds, 0).UTC()
}

func collectDocuments(sourceRoot string) ([]docsDocument, error) {
	var documents []docsDocument
	err := filepath.WalkDir(sourceRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) != "page.mdx" {
			return nil
		}

		rel, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", path, err)
		}
		rel = filepath.ToSlash(rel)

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		documents = append(documents, docsDocument{
			Title:      extractTitle(data, rel),
			Route:      docsRouteFromRelativePath(rel),
			SourcePath: "docs-source/" + rel,
			SHA256:     sha256Hex(data),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("collect docs documents: %w", err)
	}

	sort.Slice(documents, func(i, j int) bool {
		return documents[i].Route < documents[j].Route
	})
	return documents, nil
}

func extractTitle(data []byte, rel string) string {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}

	parent := filepath.Base(filepath.Dir(rel))
	if parent == "." || parent == "/" {
		return "Docs"
	}
	return strings.ReplaceAll(parent, "-", " ")
}

func docsRouteFromRelativePath(rel string) string {
	if rel == "page.mdx" {
		return "/docs"
	}
	route := strings.TrimSuffix(rel, "/page.mdx")
	return "/docs/" + strings.TrimPrefix(filepath.ToSlash(route), "/")
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func authorityMarkdown(version string) string {
	return fmt.Sprintf(`# Sandbox0 Docs Authority

This bundle contains the documentation payload for Sandbox0 release %s.

Authority order:
1. Source code
2. pkg/apispec/openapi.yaml
3. s0 CLI help and implementation
4. This bundled docs payload
5. Hosted website docs

If bundled docs disagree with code, OpenAPI, or CLI behavior, the higher-priority source wins.
`, version)
}

func writeChecksums(path, root string) error {
	files, err := listBundleFiles(root)
	if err != nil {
		return err
	}

	var builder strings.Builder
	for _, filePath := range files {
		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			return fmt.Errorf("relative path for checksum %s: %w", filePath, err)
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read %s for checksum: %w", filePath, err)
		}
		builder.WriteString(sha256Hex(data))
		builder.WriteString("  ")
		builder.WriteString(filepath.ToSlash(rel))
		builder.WriteByte('\n')
	}

	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		return fmt.Errorf("write checksum file: %w", err)
	}
	return nil
}

func listBundleFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) == "SHA256SUMS" {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list bundle files: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

func createTarGz(outPath, baseDir, bundleName string, modTime time.Time) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create archive dir: %w", err)
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create archive %s: %w", outPath, err)
	}
	defer outFile.Close()

	gzipWriter, err := gzip.NewWriterLevel(outFile, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("create gzip writer: %w", err)
	}
	gzipWriter.Name = filepath.Base(outPath)
	gzipWriter.ModTime = modTime.UTC()
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	root := filepath.Join(baseDir, bundleName)
	files, err := listBundleFiles(root)
	if err != nil {
		return err
	}

	for _, filePath := range files {
		rel, err := filepath.Rel(baseDir, filePath)
		if err != nil {
			return fmt.Errorf("relative path for archive %s: %w", filePath, err)
		}
		info, err := os.Stat(filePath)
		if err != nil {
			return fmt.Errorf("stat %s: %w", filePath, err)
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("tar header %s: %w", filePath, err)
		}
		header.Name = filepath.ToSlash(rel)
		header.ModTime = modTime.UTC()
		header.AccessTime = modTime.UTC()
		header.ChangeTime = modTime.UTC()
		header.Uid = 0
		header.Gid = 0
		header.Uname = "root"
		header.Gname = "root"
		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("write tar header %s: %w", filePath, err)
		}

		in, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("open %s for archive: %w", filePath, err)
		}
		if _, err := io.Copy(tarWriter, in); err != nil {
			in.Close()
			return fmt.Errorf("write tar data %s: %w", filePath, err)
		}
		if err := in.Close(); err != nil {
			return fmt.Errorf("close %s: %w", filePath, err)
		}
	}
	return nil
}

func writeArchiveSHA256(outPath, archivePath string) error {
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return fmt.Errorf("read archive for checksum: %w", err)
	}
	content := fmt.Sprintf("%s  %s\n", sha256Hex(data), filepath.Base(archivePath))
	if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write archive checksum: %w", err)
	}
	return nil
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("relative path from %s to %s: %w", src, path, err)
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create parent for %s: %w", dst, err)
	}
	if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

func dirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		return info.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat %s: %w", path, err)
}

func requireDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}

func generatedDirField(exists bool) string {
	if !exists {
		return ""
	}
	return "generated-docs"
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
