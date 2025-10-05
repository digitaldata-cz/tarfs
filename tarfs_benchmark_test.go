package tarfs

import (
	"archive/tar"
	"bytes"
	"fmt"
	"testing"
)

// BenchmarkOpen tests the performance of opening files
func BenchmarkOpen(b *testing.B) {
	fs := createTestFS(b, 100, 1024) // 100 files, 1KB each

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, err := fs.Open("/file_50.txt")
		if err != nil {
			b.Fatal(err)
		}
		f.Close()
	}
}

// BenchmarkReaddir tests the performance of reading directories
func BenchmarkReaddir(b *testing.B) {
	fs := createTestFS(b, 100, 1024) // 100 files, 1KB each

	// Check if root directory exists
	if !fs.Exists("/") {
		b.Fatal("root directory does not exist")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, err := fs.Open("/")
		if err != nil {
			b.Fatalf("failed to open root: %v", err)
		}
		_, err = f.Readdir(-1)
		if err != nil {
			b.Fatalf("failed to readdir: %v", err)
		}
		f.Close()
	}
}

// BenchmarkRead tests the performance of reading file contents
func BenchmarkRead(b *testing.B) {
	fs := createTestFS(b, 10, 10*1024) // 10 files, 10KB each

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, err := fs.Open("/file_5.txt")
		if err != nil {
			b.Fatal(err)
		}
		buf := make([]byte, 10*1024)
		_, err = f.Read(buf)
		if err != nil {
			b.Fatal(err)
		}
		f.Close()
	}
}

// BenchmarkMemoryUsage tests memory usage for different file counts
func BenchmarkMemoryUsage(b *testing.B) {
	sizes := []struct {
		files    int
		fileSize int
	}{
		{10, 1024},        // 10KB total
		{100, 1024},       // 100KB total
		{1000, 1024},      // 1MB total
		{100, 100 * 1024}, // 10MB total
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("files=%d_size=%d", size.files, size.fileSize), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = createTestFS(b, size.files, size.fileSize)
			}
		})
	}
}

// Helper function to create a test filesystem
func createTestFS(b *testing.B, numFiles, fileSize int) FileSystem {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	data := make([]byte, fileSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	for i := 0; i < numFiles; i++ {
		hdr := &tar.Header{
			Name: fmt.Sprintf("file_%d.txt", i),
			Size: int64(fileSize),
			Mode: 0644,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			b.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			b.Fatal(err)
		}
	}

	if err := tw.Close(); err != nil {
		b.Fatal(err)
	}

	fs, err := NewFromReader(buf)
	if err != nil {
		b.Fatal(err)
	}

	return fs
}
