package tarfs_test

import (
	"compress/flate"
	"fmt"
	"io"
	"log"

	"github.com/digitaldata-cz/tarfs"
)

// Example showing different compression algorithms
func ExampleConfig_compressionAlgorithms() {
	// DEFLATE with best compression
	deflateConfig := &tarfs.Config{
		LargeFileCompression: 50 * 1024,
		CompressionAlgorithm: tarfs.CompressionDeflate,
		CompressionLevel:     flate.BestCompression,
	}

	fs1, err := tarfs.NewFromFileWithConfig("archive.tar", deflateConfig)
	if err != nil {
		log.Fatal(err)
	}
	_ = fs1

	// GZIP with default level
	gzipConfig := &tarfs.Config{
		LargeFileCompression: 50 * 1024,
		CompressionAlgorithm: tarfs.CompressionGzip,
		CompressionLevel:     5,
	}

	fs2, err := tarfs.NewFromFileWithConfig("archive.tar", gzipConfig)
	if err != nil {
		log.Fatal(err)
	}
	_ = fs2

	// ZLIB with fast compression
	zlibConfig := &tarfs.Config{
		LargeFileCompression: 50 * 1024,
		CompressionAlgorithm: tarfs.CompressionZlib,
		CompressionLevel:     flate.BestSpeed,
	}

	fs3, err := tarfs.NewFromFileWithConfig("archive.tar", zlibConfig)
	if err != nil {
		log.Fatal(err)
	}
	_ = fs3
}

// Example showing compression level tuning
func ExampleConfig_compressionLevels() {
	// Fast compression - good for development/testing
	fastConfig := &tarfs.Config{
		LargeFileCompression: 30 * 1024,
		CompressionLevel:     flate.BestSpeed, // Level 1
	}

	// Balanced compression - good for production (default)
	balancedConfig := &tarfs.Config{
		LargeFileCompression: 50 * 1024,
		CompressionLevel:     5, // Default
	}

	// Best compression - good when memory is critical
	bestConfig := &tarfs.Config{
		LargeFileCompression: 50 * 1024,
		CompressionLevel:     flate.BestCompression, // Level 9
	}

	fmt.Printf("Fast: level %d\n", fastConfig.CompressionLevel)
	fmt.Printf("Balanced: level %d\n", balancedConfig.CompressionLevel)
	fmt.Printf("Best: level %d\n", bestConfig.CompressionLevel)

	// Output:
	// Fast: level 1
	// Balanced: level 5
	// Best: level 9
}

// Example showing combined compression and caching
func ExampleConfig_compressionWithCache() {
	config := &tarfs.Config{
		// Compression settings
		LargeFileCompression: 100 * 1024,
		CompressionAlgorithm: tarfs.CompressionDeflate,
		CompressionLevel:     6,

		// Cache settings
		UseARCCache:  true,
		ARCCacheSize: 256,
		PreloadFiles: []string{
			"index.html",
			"app.js",
		},
	}

	fs, err := tarfs.NewFromFileWithConfig("webapp.tar", config)
	if err != nil {
		log.Fatal(err)
	}

	// Files are compressed and cached
	f, err := fs.Open("/index.html")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Read %d bytes\n", len(data))
}
