package tarfs

import (
	"archive/tar"
	"bytes"
	"compress/flate"
	"io"
	"testing"
)

// Test different compression algorithms
func TestCompressionAlgorithms(t *testing.T) {
	data := bytes.Repeat([]byte("test data for compression "), 100)

	tests := []struct {
		name      string
		algorithm CompressionAlgorithm
		level     int
	}{
		{"DEFLATE default", CompressionDeflate, 5},
		{"DEFLATE best speed", CompressionDeflate, flate.BestSpeed},
		{"DEFLATE best compression", CompressionDeflate, flate.BestCompression},
		{"GZIP default", CompressionGzip, 5},
		{"GZIP best speed", CompressionGzip, 1},
		{"GZIP best compression", CompressionGzip, 9},
		{"ZLIB default", CompressionZlib, 5},
		{"ZLIB best speed", CompressionZlib, 1},
		{"ZLIB best compression", CompressionZlib, 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compress
			compressed, err := compressData(data, tt.algorithm, tt.level)
			if err != nil {
				t.Fatal(err)
			}
			if !compressed.isCompressed {
				t.Error("Data should be compressed")
			}
			if compressed.algorithm != tt.algorithm {
				t.Errorf("Expected algorithm %d, got %d", tt.algorithm, compressed.algorithm)
			}

			// Verify compression actually reduced size
			if len(compressed.compressed) >= len(data) {
				t.Logf("Warning: Compressed size (%d) >= original size (%d) for %s",
					len(compressed.compressed), len(data), tt.name)
			}

			// Decompress
			decompressed, err := decompressData(compressed)
			if err != nil {
				t.Fatalf("Decompression failed: %v", err)
			}

			// Verify data integrity
			if !bytes.Equal(data, decompressed) {
				t.Error("Decompressed data does not match original")
			}
		})
	}
}

// Test compression with different levels
func TestCompressionLevels(t *testing.T) {
	data := bytes.Repeat([]byte("compressible text data "), 1000)

	levels := []int{
		flate.BestSpeed, // 1
		3,
		5,
		flate.DefaultCompression, // 6
		flate.BestCompression,    // 9
	}

	results := make(map[int]int)

	for _, level := range levels {
		compressed, err := compressData(data, CompressionDeflate, level)
		if err != nil {
			t.Fatal(err)
		}
		results[level] = len(compressed.compressed)

		// Verify decompression works
		decompressed, err := decompressData(compressed)
		if err != nil {
			t.Fatalf("Decompression failed for level %d: %v", level, err)
		}
		if !bytes.Equal(data, decompressed) {
			t.Errorf("Data mismatch for level %d", level)
		}
	}

	// Higher compression level should produce smaller or equal size
	// (not always guaranteed, but generally true for compressible data)
	t.Logf("Compression sizes by level:")
	for _, level := range levels {
		ratio := float64(results[level]) / float64(len(data)) * 100
		t.Logf("  Level %d: %d bytes (%.1f%% of original)", level, results[level], ratio)
	}
}

// Test filesystem with custom compression config
func TestFileSystemWithCompressionConfig(t *testing.T) {
	// Create TAR with compressible file
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	data := bytes.Repeat([]byte("This is highly compressible data! "), 1000)
	hdr := &tar.Header{
		Name: "large.txt",
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

	// Test with DEFLATE
	t.Run("DEFLATE", func(t *testing.T) {
		config := &Config{
			LargeFileCompression: 1024,
			CompressionAlgorithm: CompressionDeflate,
			CompressionLevel:     flate.BestCompression,
		}

		fs, err := NewFromReaderWithConfig(bytes.NewReader(buf.Bytes()), config)
		if err != nil {
			t.Fatal(err)
		}

		f, err := fs.Open("/large.txt")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(data, content) {
			t.Error("Content mismatch with DEFLATE compression")
		}
	})

	// Test with GZIP
	t.Run("GZIP", func(t *testing.T) {
		config := &Config{
			LargeFileCompression: 1024,
			CompressionAlgorithm: CompressionGzip,
			CompressionLevel:     6,
		}

		fs, err := NewFromReaderWithConfig(bytes.NewReader(buf.Bytes()), config)
		if err != nil {
			t.Fatal(err)
		}

		f, err := fs.Open("/large.txt")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(data, content) {
			t.Error("Content mismatch with GZIP compression")
		}
	})

	// Test with ZLIB
	t.Run("ZLIB", func(t *testing.T) {
		config := &Config{
			LargeFileCompression: 1024,
			CompressionAlgorithm: CompressionZlib,
			CompressionLevel:     5,
		}

		fs, err := NewFromReaderWithConfig(bytes.NewReader(buf.Bytes()), config)
		if err != nil {
			t.Fatal(err)
		}

		f, err := fs.Open("/large.txt")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(data, content) {
			t.Error("Content mismatch with ZLIB compression")
		}
	})
}

// Test default config has correct compression settings
func TestDefaultCompressionConfig(t *testing.T) {
	config := DefaultConfig()

	if config.CompressionAlgorithm != CompressionDeflate {
		t.Errorf("Expected default algorithm %d, got %d", CompressionDeflate, config.CompressionAlgorithm)
	}

	if config.CompressionLevel != 5 {
		t.Errorf("Expected default level 5, got %d", config.CompressionLevel)
	}

	if config.LargeFileCompression != 0 {
		t.Errorf("Expected compression disabled by default, got threshold %d", config.LargeFileCompression)
	}
}

// Test compression with invalid algorithm
func TestCompressionInvalidAlgorithm(t *testing.T) {
	if _, err := compressData([]byte("test data"), CompressionAlgorithm(999), 5); err == nil {
		t.Fatal("invalid algorithm accepted")
	}
}
