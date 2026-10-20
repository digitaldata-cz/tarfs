package tarfs

import "fmt"

// CompressionAlgorithm selects the in-memory representation of file contents.
type CompressionAlgorithm int

const (
	// CompressionNone keeps the original bytes, including with a nonzero threshold.
	CompressionNone CompressionAlgorithm = iota
	CompressionDeflate
	CompressionGzip
	CompressionZlib
)

// Config controls immutable payload storage and the optional decoded-file cache.
// The filesystem does not retain Config or PreloadFiles after construction.
type Config struct {
	// LargeFileCompression is the minimum size in bytes to attempt compression.
	// Zero disables it. Use DefaultConfig to start with DEFLATE at level 5.
	LargeFileCompression int64
	// CompressionAlgorithm is only used when LargeFileCompression is positive.
	// The zero value, CompressionNone, explicitly disables compression.
	CompressionAlgorithm CompressionAlgorithm
	// CompressionLevel accepts -2 (Huffman only), -1 (standard default), 0 (no
	// compression), and 1 through 9. DefaultConfig selects 5.
	CompressionLevel int
	// UseARCCache retains decoded compressed files across opens. All source
	// payloads remain in memory; this is not disk-backed or lazy archive loading.
	UseARCCache bool
	// ARCCacheSize bounds the number of decoded files, not their bytes. Zero uses
	// 128; negative values are invalid when the cache is enabled.
	ARCCacheSize int
	// PreloadFiles pins decoded files outside ARC. Only used with UseARCCache.
	// Missing files and directory names are errors. Entries are virtual paths.
	PreloadFiles []string
}

// DefaultConfig disables compression and caching, with settings ready for opt-in.
func DefaultConfig() *Config {
	return &Config{CompressionAlgorithm: CompressionDeflate, CompressionLevel: 5, ARCCacheSize: 128}
}

func (config *Config) validate() error {
	if config.LargeFileCompression < 0 {
		return fmt.Errorf("tarfs: negative compression threshold")
	}
	if config.UseARCCache && config.ARCCacheSize < 0 {
		return fmt.Errorf("tarfs: negative cache capacity")
	}
	if config.LargeFileCompression > 0 {
		return validateCompression(config.CompressionAlgorithm, config.CompressionLevel)
	}
	return nil
}
