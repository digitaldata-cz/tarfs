package tarfs

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"
)

func TestCompression(t *testing.T) {
	// Create a test archive with a large file
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	// Create a large file (>10KB)
	largeData := bytes.Repeat([]byte("This is test data for compression. "), 500)

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

	// Small file
	smallData := []byte("small file")
	hdr = &tar.Header{
		Name: "small.txt",
		Size: int64(len(smallData)),
		Mode: 0644,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(smallData); err != nil {
		t.Fatal(err)
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	// Test with compression enabled
	config := &Config{
		LargeFileCompression: 10 * 1024, // 10KB threshold
		UseARCCache:          false,
	}

	fs, err := NewFromReaderWithConfig(buf, config)
	if err != nil {
		t.Fatal(err)
	}

	// Read large file (should be compressed)
	f, err := fs.Open("/large.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(data, largeData) {
		t.Error("Large file data mismatch after decompression")
	}

	// Read small file (should not be compressed)
	f2, err := fs.Open("/small.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()

	data2, err := io.ReadAll(f2)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(data2, smallData) {
		t.Error("Small file data mismatch")
	}
}

func TestARCCache(t *testing.T) {
	// Create test archive
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	files := []string{"file1.txt", "file2.txt", "file3.txt"}
	for _, name := range files {
		data := []byte("Content of " + name)
		hdr := &tar.Header{
			Name: name,
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

	// Test with ARC cache
	config := &Config{
		UseARCCache:  true,
		ARCCacheSize: 2,                     // Small cache for testing
		PreloadFiles: []string{"file1.txt"}, // Preload file1
	}

	fs, err := NewFromReaderWithConfig(buf, config)
	if err != nil {
		t.Fatal(err)
	}

	// Check that preloaded file exists and is accessible
	if !fs.Exists("/file1.txt") {
		t.Error("Preloaded file1.txt should exist")
	}

	f1, err := fs.Open("/file1.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f1.Close()

	data1, err := io.ReadAll(f1)
	if err != nil {
		t.Fatal(err)
	}

	expected1 := []byte("Content of file1.txt")
	if !bytes.Equal(data1, expected1) {
		t.Errorf("Expected %s, got %s", expected1, data1)
	}

	// Access cached files
	f2, err := fs.Open("/file2.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()

	data2, err := io.ReadAll(f2)
	if err != nil {
		t.Fatal(err)
	}

	expected2 := []byte("Content of file2.txt")
	if !bytes.Equal(data2, expected2) {
		t.Errorf("Expected %s, got %s", expected2, data2)
	}
}

func TestPreloadFiles(t *testing.T) {
	// Create test archive
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	preloadData := []byte("This should be preloaded")
	normalData := []byte("This is normal")

	// Preloaded file
	hdr := &tar.Header{
		Name: "preload.txt",
		Size: int64(len(preloadData)),
		Mode: 0644,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(preloadData); err != nil {
		t.Fatal(err)
	}

	// Normal file
	hdr = &tar.Header{
		Name: "normal.txt",
		Size: int64(len(normalData)),
		Mode: 0644,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(normalData); err != nil {
		t.Fatal(err)
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	// Create FS with preload
	config := &Config{
		UseARCCache:  true,
		ARCCacheSize: 128,
		PreloadFiles: []string{"preload.txt"},
	}

	fs, err := NewFromReaderWithConfig(buf, config)
	if err != nil {
		t.Fatal(err)
	}

	tfs := fs.(*tarFs)

	// Verify preload.txt is marked as preloaded
	if !tfs.preloaded["/preload.txt"] {
		t.Error("preload.txt should be marked as preloaded")
	}

	// Verify normal.txt is not preloaded
	if tfs.preloaded["/normal.txt"] {
		t.Error("normal.txt should not be marked as preloaded")
	}

	// Both files should be accessible
	f, err := fs.Open("/preload.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(data, preloadData) {
		t.Error("Preloaded file data mismatch")
	}
}

func TestCompressionWithCache(t *testing.T) {
	// Test combination of compression and caching
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	// Large file that should be compressed
	largeData := bytes.Repeat([]byte("Compress me! "), 2000)

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

	// Enable both compression and caching
	config := &Config{
		LargeFileCompression: 10 * 1024,
		UseARCCache:          true,
		ARCCacheSize:         128,
	}

	fs, err := NewFromReaderWithConfig(buf, config)
	if err != nil {
		t.Fatal(err)
	}

	// Read file multiple times to test cache
	for i := 0; i < 3; i++ {
		f, err := fs.Open("/large.txt")
		if err != nil {
			t.Fatalf("Iteration %d: %v", i, err)
		}

		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("Iteration %d: %v", i, err)
		}
		f.Close()

		if !bytes.Equal(data, largeData) {
			t.Errorf("Iteration %d: Data mismatch", i)
		}
	}
}
