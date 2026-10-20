package tarfs

import (
	"archive/tar"
	"bytes"
	"compress/flate"
	"io"
	"strings"
	"testing"
)

func buildTar(t testing.TB, files map[string]string) *bytes.Buffer {
	t.Helper()

	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	for name, body := range files {
		hdr := &tar.Header{
			Name: name,
			Size: int64(len(body)),
			Mode: 0o644,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %s: %v", name, err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("write body %s: %v", name, err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}

	return buf
}

func TestOpenWithBackslashInHeader(t *testing.T) {
	buf := buildTar(t, map[string]string{
		"win\\dir\\file.txt": "windows path",
	})

	fs, err := NewFromReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("NewFromReader: %v", err)
	}

	f, err := fs.Open("win/dir/file.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(content) != "windows path" {
		t.Fatalf("expected content to match, got %q", string(content))
	}

	if !fs.Exists("win\\dir\\file.txt") {
		t.Fatalf("Exists should accept backslash paths")
	}
}

func TestARCReadAfterEviction(t *testing.T) {
	files := map[string]string{
		"a.txt": strings.Repeat("A", 2048),
		"b.txt": strings.Repeat("B", 2048),
		"c.txt": strings.Repeat("C", 2048),
	}
	buf := buildTar(t, files)

	config := &Config{
		UseARCCache:          true,
		ARCCacheSize:         1,
		LargeFileCompression: 1,
		CompressionAlgorithm: CompressionDeflate,
		CompressionLevel:     flate.DefaultCompression,
	}

	fs, err := NewFromReaderWithConfig(bytes.NewReader(buf.Bytes()), config)
	if err != nil {
		t.Fatalf("NewFromReaderWithConfig: %v", err)
	}

	// Initial load should evict earlier entries from ARC cache, so opening
	// "a.txt" must fall back to stored payload instead of returning empty data.
	f, err := fs.Open("a.txt")
	if err != nil {
		t.Fatalf("Open a.txt: %v", err)
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		t.Fatalf("ReadAll a.txt: %v", err)
	}
	if string(data) != files["a.txt"] {
		t.Fatalf("unexpected data for a.txt: got %d bytes", len(data))
	}

	// Access other files to ensure cache churn does not corrupt data.
	for _, name := range []string{"b.txt", "c.txt"} {
		f, err := fs.Open(name)
		if err != nil {
			t.Fatalf("Open %s: %v", name, err)
		}
		got, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			t.Fatalf("ReadAll %s: %v", name, err)
		}
		if string(got) != files[name] {
			t.Fatalf("unexpected data for %s", name)
		}
	}

	// Re-open a.txt after churn to confirm re-populating ARC works.
	f, err = fs.Open("a.txt")
	if err != nil {
		t.Fatalf("re-open a.txt: %v", err)
	}
	data, err = io.ReadAll(f)
	f.Close()
	if err != nil {
		t.Fatalf("re-read a.txt: %v", err)
	}
	if string(data) != files["a.txt"] {
		t.Fatalf("unexpected data for a.txt after churn")
	}
}

func TestInvalidCompressionLevelFailsConstruction(t *testing.T) {
	config := DefaultConfig()
	config.LargeFileCompression = 1
	config.UseARCCache = true
	config.CompressionLevel = 20
	if _, err := NewFromReaderWithConfig(buildTar(t, map[string]string{"file": "data"}), config); err == nil {
		t.Fatal("invalid compression level accepted")
	}
}
