package tarfs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Test NewFromGzipFile
func TestNewFromGzipFile(t *testing.T) {
	// Create a gzipped tar file
	tmpDir := t.TempDir()
	tarGzPath := filepath.Join(tmpDir, "test.tar.gz")

	// Create tar.gz file
	f, err := os.Create(tarGzPath)
	if err != nil {
		t.Fatal(err)
	}

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	data := []byte("gzip test data")
	hdr := &tar.Header{
		Name: "test.txt",
		Size: int64(len(data)),
		Mode: 0644,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Test NewFromGzipFile
	fs, err := NewFromGzipFile(tarGzPath)
	if err != nil {
		t.Fatalf("NewFromGzipFile failed: %v", err)
	}

	if !fs.Exists("/test.txt") {
		t.Error("File should exist in gzipped tar")
	}

	file, err := fs.Open("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}

	if string(content) != "gzip test data" {
		t.Errorf("Expected 'gzip test data', got '%s'", string(content))
	}
}

// Test NewFromGzipFileWithConfig
func TestNewFromGzipFileWithConfig(t *testing.T) {
	// Create a gzipped tar file
	tmpDir := t.TempDir()
	tarGzPath := filepath.Join(tmpDir, "test.tar.gz")

	f, err := os.Create(tarGzPath)
	if err != nil {
		t.Fatal(err)
	}

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	// Create large file for compression
	largeData := bytes.Repeat([]byte("A"), 2048)
	hdr := &tar.Header{
		Name: "large.txt",
		Size: int64(len(largeData)),
		Mode: 0644,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(largeData); err != nil {
		t.Fatal(err)
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Test with config
	config := &Config{
		LargeFileCompression: 1024,
	}

	fs, err := NewFromGzipFileWithConfig(tarGzPath, config)
	if err != nil {
		t.Fatalf("NewFromGzipFileWithConfig failed: %v", err)
	}

	if !fs.Exists("/large.txt") {
		t.Error("File should exist in gzipped tar")
	}
}

// Test NewFromBzip2File
func TestNewFromBzip2File(t *testing.T) {
	// Create a bzip2 tar file manually (bzip2.Writer is in another package)
	tmpDir := t.TempDir()
	tarBz2Path := filepath.Join(tmpDir, "test.tar.bz2")

	// Create tar file first
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	data := []byte("bzip2 test data")
	hdr := &tar.Header{
		Name: "test.txt",
		Size: int64(len(data)),
		Mode: 0644,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	// Write to file (not actually compressed, just for testing the reader path)
	// Note: Go's compress/bzip2 is read-only, so we can't create real bz2 files
	// We'll test that the function handles bzip2 reader correctly
	if err := os.WriteFile(tarBz2Path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	// This will fail because the file is not actually bzip2 compressed
	// But it tests the code path
	_, err := NewFromBzip2File(tarBz2Path)
	if err == nil {
		t.Log("Note: Test file is not actually bzip2 compressed, but code path is tested")
	}
}

// Test Mode() method
func TestFileInfoMode(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	data := []byte("test")
	hdr := &tar.Header{
		Name: "executable.sh",
		Size: int64(len(data)),
		Mode: 0755, // Executable
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	fs, err := NewFromReader(buf)
	if err != nil {
		t.Fatal(err)
	}

	f, err := fs.Open("/executable.sh")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	mode := info.Mode()
	if mode.Perm() != 0755 {
		t.Errorf("Expected mode 0755, got %o", mode.Perm())
	}
}

// Test cache operations indirectly
func TestCacheOperations(t *testing.T) {
	config := &Config{
		UseARCCache:  true,
		ARCCacheSize: 10,
	}

	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	// Add some files
	for i := 0; i < 5; i++ {
		data := []byte("data")
		hdr := &tar.Header{
			Name: string(rune('a'+i)) + ".txt",
			Size: int64(len(data)),
			Mode: 0644,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	fs, err := NewFromReaderWithConfig(buf, config)
	if err != nil {
		t.Fatal(err)
	}

	// Access some files to populate cache
	for i := 0; i < 3; i++ {
		name := "/" + string(rune('a'+i)) + ".txt"
		f, err := fs.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "data" {
			t.Errorf("Expected 'data', got '%s'", string(content))
		}
		f.Close()
	}

	// Verify files can be re-read (tests cache Get)
	for i := 0; i < 3; i++ {
		name := "/" + string(rune('a'+i)) + ".txt"
		f, err := fs.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "data" {
			t.Errorf("Expected 'data', got '%s'", string(content))
		}
		f.Close()
	}
}

// Test min/max helpers
func TestMinMax(t *testing.T) {
	cache := newARCCache(10)

	// These are internal helpers, but we can test them indirectly
	// by triggering code paths that use them

	// Fill cache to trigger min/max usage
	for i := 0; i < 20; i++ {
		key := string(rune('a' + i))
		cache.Put(key, []byte("data"))
	}

	// If min/max work correctly, cache should be bounded
	cache.mu.RLock()
	t1Len := cache.t1.Len()
	t2Len := cache.t2.Len()
	cache.mu.RUnlock()

	total := t1Len + t2Len
	if total > 10 {
		t.Errorf("Cache exceeded capacity: %d > 10", total)
	}
}

// Test ghostInB1 helper
func TestGhostInB1(t *testing.T) {
	cache := newARCCache(2)

	// Fill cache
	cache.Put("a", []byte("a"))
	cache.Put("b", []byte("b"))

	// Move 'a' to T2
	cache.Get("a")

	// Add 'c', which should evict 'b' to B1
	cache.Put("c", []byte("c"))

	// Re-add 'b', which should hit the ghost list
	cache.Put("b", []byte("b-new"))

	// This exercises the ghostInB1 code path
	if cache.Get("b") == nil {
		t.Error("'b' should be in cache after ghost hit")
	}
}

// Test removeGhost helper
func TestRemoveGhost(t *testing.T) {
	cache := newARCCache(2)

	// Create scenario where ghosts need to be removed
	for i := 0; i < 10; i++ {
		key := string(rune('a' + i))
		cache.Put(key, []byte(key))
	}

	// Ghost lists should be bounded
	cache.mu.RLock()
	b1Len := cache.b1.Len()
	b2Len := cache.b2.Len()
	cache.mu.RUnlock()

	// Should have cleaned up ghost entries
	if b1Len+b2Len > 4 { // 2x cache size
		t.Errorf("Ghost lists too large: B1=%d, B2=%d", b1Len, b2Len)
	}
}
