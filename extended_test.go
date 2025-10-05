package tarfs

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"testing"
	"time"
)

// Test Seek functionality
func TestSeek(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	data := []byte("0123456789ABCDEFGHIJ")
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

	fs, err := NewFromReader(buf)
	if err != nil {
		t.Fatal(err)
	}

	f, err := fs.Open("/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Test SeekStart
	pos, err := f.Seek(5, io.SeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if pos != 5 {
		t.Errorf("Expected position 5, got %d", pos)
	}

	buf2 := make([]byte, 5)
	n, err := f.Read(buf2)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("Expected to read 5 bytes, got %d", n)
	}
	if string(buf2) != "56789" {
		t.Errorf("Expected '56789', got '%s'", string(buf2))
	}

	// Test SeekCurrent
	pos, err = f.Seek(2, io.SeekCurrent)
	if err != nil {
		t.Fatal(err)
	}
	if pos != 12 {
		t.Errorf("Expected position 12, got %d", pos)
	}

	// Test SeekEnd
	pos, err = f.Seek(-5, io.SeekEnd)
	if err != nil {
		t.Fatal(err)
	}
	if pos != 15 {
		t.Errorf("Expected position 15, got %d", pos)
	}

	buf3 := make([]byte, 5)
	_, err = f.Read(buf3)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf3) != "FGHIJ" {
		t.Errorf("Expected 'FGHIJ', got '%s'", string(buf3))
	}

	// Test negative seek
	_, err = f.Seek(-100, io.SeekStart)
	if err == nil {
		t.Error("Expected error for negative seek position")
	}

	// Test invalid whence
	_, err = f.Seek(0, 999)
	if err == nil {
		t.Error("Expected error for invalid whence")
	}
}

// Test directory operations
func TestDirectorySeek(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	hdr := &tar.Header{
		Name:     "testdir/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	fs, err := NewFromReader(buf)
	if err != nil {
		t.Fatal(err)
	}

	f, err := fs.Open("/testdir")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Seek on directory should fail
	_, err = f.Seek(0, io.SeekStart)
	if err == nil {
		t.Error("Expected error when seeking on directory")
	}

	// Read on directory should fail
	buf2 := make([]byte, 10)
	_, err = f.Read(buf2)
	if err == nil {
		t.Error("Expected error when reading directory")
	}
}

// Test Readdir pagination
func TestReaddirPagination(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	// Create 10 files
	for i := 0; i < 10; i++ {
		data := []byte("file content")
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

	fs, err := NewFromReader(buf)
	if err != nil {
		t.Fatal(err)
	}

	f, err := fs.Open("/")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Read 3 files at a time
	files1, err := f.Readdir(3)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if len(files1) != 3 {
		t.Errorf("Expected 3 files, got %d", len(files1))
	}

	files2, err := f.Readdir(3)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if len(files2) != 3 {
		t.Errorf("Expected 3 files, got %d", len(files2))
	}

	// Read remaining files
	files3, err := f.Readdir(10)
	if err != io.EOF {
		t.Errorf("Expected EOF, got %v", err)
	}
	if len(files3) != 4 {
		t.Errorf("Expected 4 files, got %d", len(files3))
	}

	// Further reads should return EOF
	files4, err := f.Readdir(1)
	if err != io.EOF {
		t.Error("Expected EOF after reading all files")
	}
	if files4 != nil {
		t.Error("Expected nil result after EOF")
	}
}

// Test Exists method
func TestExists(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	data := []byte("test")
	hdr := &tar.Header{
		Name: "exists.txt",
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

	fs, err := NewFromReader(buf)
	if err != nil {
		t.Fatal(err)
	}

	// File exists
	if !fs.Exists("/exists.txt") {
		t.Error("File should exist")
	}

	// File doesn't exist
	if fs.Exists("/notexists.txt") {
		t.Error("File should not exist")
	}

	// Invalid path with null byte
	if fs.Exists("/test\x00.txt") {
		t.Error("Path with null byte should not exist")
	}

	// Test various path formats
	if !fs.Exists("exists.txt") {
		t.Error("File should exist without leading slash")
	}
	if !fs.Exists("./exists.txt") {
		t.Error("File should exist with ./")
	}
}

// Test file stat
func TestStat(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	now := time.Now().Truncate(time.Second)
	data := []byte("test data")
	hdr := &tar.Header{
		Name:    "stat.txt",
		Size:    int64(len(data)),
		Mode:    0644,
		ModTime: now,
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

	f, err := fs.Open("/stat.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if info.Name() != "stat.txt" {
		t.Errorf("Expected name 'stat.txt', got '%s'", info.Name())
	}
	if info.Size() != int64(len(data)) {
		t.Errorf("Expected size %d, got %d", len(data), info.Size())
	}
	if info.IsDir() {
		t.Error("File should not be a directory")
	}
	if !info.ModTime().Equal(now) {
		t.Errorf("Expected modtime %v, got %v", now, info.ModTime())
	}
	if info.Sys() != nil {
		t.Error("Sys() should return nil")
	}
}

// Test directory stat
func TestDirectoryStat(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	hdr := &tar.Header{
		Name:     "dir/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	fs, err := NewFromReader(buf)
	if err != nil {
		t.Fatal(err)
	}

	f, err := fs.Open("/dir")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if !info.IsDir() {
		t.Error("Should be a directory")
	}
	if info.Size() != 0 {
		t.Error("Directory size should be 0")
	}
}

// Test Close method
func TestClose(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	data := []byte("test")
	hdr := &tar.Header{
		Name: "close.txt",
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

	fs, err := NewFromReader(buf)
	if err != nil {
		t.Fatal(err)
	}

	f, err := fs.Open("/close.txt")
	if err != nil {
		t.Fatal(err)
	}

	// Close should not return error
	if err := f.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Multiple closes should be safe
	if err := f.Close(); err != nil {
		t.Errorf("Second close returned error: %v", err)
	}
}

// Test opening non-existent file
func TestOpenNonExistent(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	fs, err := NewFromReader(buf)
	if err != nil {
		t.Fatal(err)
	}

	_, err = fs.Open("/nonexistent.txt")
	if err == nil {
		t.Error("Expected error when opening non-existent file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("Expected os.ErrNotExist, got %v", err)
	}
}

// Test invalid file paths
func TestInvalidPaths(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	fs, err := NewFromReader(buf)
	if err != nil {
		t.Fatal(err)
	}

	// Path with null byte
	_, err = fs.Open("/test\x00.txt")
	if err == nil {
		t.Error("Expected error for path with null byte")
	}
}

// Test nested directories
func TestNestedDirectories(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	// Create nested file without explicit directory entries
	data := []byte("nested content")
	hdr := &tar.Header{
		Name: "a/b/c/file.txt",
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

	fs, err := NewFromReader(buf)
	if err != nil {
		t.Fatal(err)
	}

	// All parent directories should be auto-created
	if !fs.Exists("/a") {
		t.Error("Parent directory /a should exist")
	}
	if !fs.Exists("/a/b") {
		t.Error("Parent directory /a/b should exist")
	}
	if !fs.Exists("/a/b/c") {
		t.Error("Parent directory /a/b/c should exist")
	}
	if !fs.Exists("/a/b/c/file.txt") {
		t.Error("File should exist")
	}

	// Open parent directory and list contents
	f, err := fs.Open("/a/b/c")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	files, err := f.Readdir(-1)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 file in directory, got %d", len(files))
	}
}

// Test reading past EOF
func TestReadEOF(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	data := []byte("short")
	hdr := &tar.Header{
		Name: "eof.txt",
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

	fs, err := NewFromReader(buf)
	if err != nil {
		t.Fatal(err)
	}

	f, err := fs.Open("/eof.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Read all data
	buf2 := make([]byte, 100)
	n, err := f.Read(buf2)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(data) {
		t.Errorf("Expected to read %d bytes, got %d", len(data), n)
	}

	// Next read should return EOF
	n, err = f.Read(buf2)
	if err != io.EOF {
		t.Errorf("Expected EOF, got %v", err)
	}
	if n != 0 {
		t.Errorf("Expected 0 bytes, got %d", n)
	}
}

// Test empty directory
func TestEmptyDirectory(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	hdr := &tar.Header{
		Name:     "emptydir/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	fs, err := NewFromReader(buf)
	if err != nil {
		t.Fatal(err)
	}

	f, err := fs.Open("/emptydir")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	files, err := f.Readdir(-1)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 0 {
		t.Errorf("Expected empty directory, got %d files", len(files))
	}
}

// Test Readdir on non-directory
func TestReaddirOnFile(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	data := []byte("file")
	hdr := &tar.Header{
		Name: "file.txt",
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

	fs, err := NewFromReader(buf)
	if err != nil {
		t.Fatal(err)
	}

	f, err := fs.Open("/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	_, err = f.Readdir(-1)
	if err == nil {
		t.Error("Expected error when calling Readdir on file")
	}
}
