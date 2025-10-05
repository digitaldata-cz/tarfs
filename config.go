package tarfs

// CompressionAlgorithm specifies the compression algorithm to use
type CompressionAlgorithm int

const (
	// CompressionNone disables compression
	CompressionNone CompressionAlgorithm = iota
	// CompressionDeflate uses DEFLATE algorithm (compress/flate)
	CompressionDeflate
	// CompressionGzip uses GZIP algorithm (compress/gzip)
	CompressionGzip
	// CompressionZlib uses ZLIB algorithm (compress/zlib)
	CompressionZlib
)

// Config provides configuration options for TarFS
type Config struct {
	// LargeFileCompression specifies the minimum file size (in bytes) for compression.
	// Files larger than this threshold will be compressed in memory.
	// Set to 0 to disable compression (default).
	LargeFileCompression int64

	// CompressionAlgorithm specifies which compression algorithm to use.
	// Options: CompressionDeflate (default), CompressionGzip, CompressionZlib
	// Only used when LargeFileCompression > 0.
	CompressionAlgorithm CompressionAlgorithm

	// CompressionLevel specifies the compression level (1-9).
	// 1 = fastest/least compression, 9 = slowest/best compression
	// Special values:
	//   flate.DefaultCompression (-1) = default level (6)
	//   flate.NoCompression (0) = no compression
	//   flate.BestSpeed (1) = fastest
	//   flate.BestCompression (9) = best compression
	// Default: 5 (good balance between speed and compression ratio)
	// Only used when LargeFileCompression > 0.
	CompressionLevel int

	// UseARCCache enables the Adaptive Replacement Cache for file data.
	// When enabled, files are loaded on-demand and cached using an ARC algorithm
	// which balances between recently and frequently used items.
	// When disabled, all files are loaded into memory at initialization.
	UseARCCache bool

	// ARCCacheSize specifies the maximum number of files to keep in the ARC cache.
	// Only used when UseARCCache is true.
	// Default: 128
	ARCCacheSize int

	// PreloadFiles is a list of file paths that should be loaded immediately
	// and kept in memory permanently (not subject to cache eviction).
	// Only used when UseARCCache is true.
	// These files bypass the cache and remain in memory always.
	PreloadFiles []string
}

// DefaultConfig returns a Config with default values
func DefaultConfig() *Config {
	return &Config{
		LargeFileCompression: 0,
		CompressionAlgorithm: CompressionDeflate,
		CompressionLevel:     5, // Good balance between speed and compression
		UseARCCache:          false,
		ARCCacheSize:         128,
		PreloadFiles:         []string{},
	}
}
