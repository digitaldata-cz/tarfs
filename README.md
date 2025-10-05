[![GoDoc](https://godoc.org/github.com/digitaldata-cz/tarfs?status.svg)](https://godoc.org/github.com/digitaldata-cz/tarfs)
[![Go](https://github.com/digitaldata-cz/tarfs/actions/workflows/go.yml/badge.svg)](https://github.com/digitaldata-cz/tarfs/actions/workflows/go.yml)
[![CodeQL](https://github.com/digitaldata-cz/tarfs/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/digitaldata-cz/tarfs/actions/workflows/codeql-analysis.yml)

# TarFS - Virtual Filesystem for TAR Archives

Go module implementing a virtual filesystem over TAR archives. Ideal for embedding static files (web assets, documentation) into Go applications.

## ✨ Features

### Core Capabilities
- **Memory-efficient**: Custom lightweight `FileInfo` implementation
- **Thread-safe**: Safe for concurrent reads with `sync.RWMutex`
- **Fast directory listings**: O(1) access via pre-computed maps
- **Automatic parent directory creation**: No need for explicit directory entries in TAR
- **Multiple compression formats**: Support for `.tar`, `.tar.gz`, and `.tar.bz2`

### Advanced Features

#### 🗜️ Large File Compression
Automatically compress large files in memory using DEFLATE compression (Go standard library):
- **Configurable threshold**: Set minimum file size for compression
- **Transparent decompression**: Files are automatically decompressed on read
- **Memory savings**: Up to 60-80% for text files (HTML, CSS, JS, JSON)
- **Zero dependencies**: Uses only Go standard library

#### 💾 ARC Cache (Adaptive Replacement Cache)
Intelligent caching that balances between recently and frequently used files:
- **Adaptive algorithm**: Automatically adjusts between LRU and LFU strategies
- **On-demand loading**: Files loaded only when accessed (when cache enabled)
- **Configurable size**: Set maximum number of files in cache
- **Preload support**: Specify files to keep in memory permanently (bypass cache)

### Performance
```
BenchmarkOpen-14         11.6M ops   90.57 ns/op   128 B/op   4 allocs/op
BenchmarkReaddir-14      706k ops    1539 ns/op    4547 B/op  10 allocs/op  
BenchmarkRead-14         1M ops      1031 ns/op    10368 B/op 5 allocs/op
```

## 🚀 What's New in v2.0

### Major Features Added

#### 1. Large File Compression (60-80% memory savings)
Automatic in-memory compression using DEFLATE algorithm for files above configurable threshold:
- Standard library: no external dependencies
- Transparent operation: automatic decompression on read
- Ideal for HTML, CSS, JS, JSON files

#### 2. ARC Cache (90%+ memory savings)
Intelligent adaptive caching with on-demand file loading:
- Balances recency vs. frequency automatically
- Configurable cache size
- Preload critical files
- Perfect for large archives with sparse access

#### 3. Combined Optimization (99% memory reduction)
Use both features together for maximum efficiency:
```go
config := &tarfs.Config{
    LargeFileCompression: 100 * 1024,
    UseARCCache:          true,
    ARCCacheSize:         256,
    PreloadFiles:         []string{"index.html"},
}
```

### Memory Usage Comparison

**Scenario**: 10,000 files, 150KB average size

| Configuration | Memory Usage | Savings |
|--------------|--------------|---------|
| Default (v1.x) | 1.5 GB | 0% |
| Compression only | 450 MB | 70% |
| Cache only | 38 MB | 97% |
| **Compression + Cache** | **11 MB** | **99%** |

### Backward Compatibility

✅ **100% backward compatible** - All existing code continues to work:
```go
// Old code still works without changes
fs, err := tarfs.NewFromBzip2File("web.tbz2")
```

New features are opt-in via `*WithConfig` functions:
```go
// New configuration-based API
fs, err := tarfs.NewFromBzip2FileWithConfig("web.tbz2", config)
```

## Installation

```bash
go get github.com/digitaldata-cz/tarfs
```

## Usage

### Basic Usage

```go
package main

import (
    "log"
    "net/http"
    "github.com/digitaldata-cz/tarfs"
)

func main() {
    // Load TAR archive
    fs, err := tarfs.NewFromBzip2File("web.tbz2")
    if err != nil {
        log.Fatal(err)
    }

    // Serve files via HTTP
    http.Handle("/", http.FileServer(fs))
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

### With Compression (Memory Optimization)

Compress large files (>50KB) in memory to reduce memory footprint:

```go
config := &tarfs.Config{
    LargeFileCompression: 50 * 1024, // Compress files larger than 50KB
    CompressionAlgorithm: tarfs.CompressionDeflate, // DEFLATE (default), GZIP, or ZLIB
    CompressionLevel:     5, // 1-9, default is 5 (good balance)
}

fs, err := tarfs.NewFromBzip2FileWithConfig("web.tbz2", config)
if err != nil {
    log.Fatal(err)
}
```

fs, err := tarfs.NewFromBzip2FileWithConfig("web.tbz2", config)
if err != nil {
    log.Fatal(err)
}
```

**Use case**: Large archives with many big files (images, videos, large JSON/HTML files)

**Memory savings example**:
- 100 HTML files, 200KB each = 20MB uncompressed
- With compression: ~4-6MB (70-80% savings)

### With ARC Cache (For Very Large Archives)

Use ARC cache for archives where not all files will be accessed:

```go
config := &tarfs.Config{
    UseARCCache:  true,
    ARCCacheSize: 128, // Keep max 128 files in cache
    PreloadFiles: []string{
        "index.html",      // Always keep in memory
        "style.css",       // Always keep in memory
        "app.js",          // Always keep in memory
    },
}

fs, err := tarfs.NewFromBzip2FileWithConfig("web.tbz2", config)
if err != nil {
    log.Fatal(err)
}
```

**Use case**: Archives with thousands of files where only a subset is frequently accessed

**Preload files**: Critical files (index.html, main CSS/JS) are loaded immediately and kept in memory permanently, bypassing the cache

**How it works**:
1. At startup: Only metadata and preloaded files are loaded into memory
2. On first access: File is loaded and cached
3. Cache full: ARC algorithm evicts least valuable files (balancing recency vs frequency)
4. Preloaded files: Never evicted, always available

### Combined: Compression + Cache

For maximum memory efficiency with large archives:

```go
config := &tarfs.Config{
    LargeFileCompression: 100 * 1024, // Compress files >100KB
    UseARCCache:          true,
    ARCCacheSize:         256,
    PreloadFiles: []string{
        "index.html",
        "style.css",
    },
}

fs, err := tarfs.NewFromFileWithConfig("large-archive.tar", config)
if err != nil {
    log.Fatal(err)
}
```

**Best for**: Very large archives (>100MB) with mixed file sizes where only a portion is frequently accessed

### Configuration Options

```go
type Config struct {
    // LargeFileCompression: minimum file size (bytes) for compression
    // Files >= this size will be compressed in memory using S2
    // Set to 0 to disable (default)
    LargeFileCompression int64

    // UseARCCache: enable Adaptive Replacement Cache
    // When enabled, files loaded on-demand and cached
    // When disabled, all files loaded into memory at startup (default)
    UseARCCache bool

    // ARCCacheSize: maximum number of files in cache
    // Only used when UseARCCache is true
    // Default: 128
    ARCCacheSize int

    // PreloadFiles: files to load immediately and keep permanently
    // Only used when UseARCCache is true
    // These bypass the cache and remain in memory always
    PreloadFiles []string
}
```

## Use Cases & Recommendations

### Small to Medium Archives (< 10MB)
**Recommendation**: Default configuration
```go
fs, err := tarfs.NewFromBzip2File("assets.tbz2")
```
- Simple and fast
- Low overhead
- All files in memory

### Large Archives with Big Files (10-100MB)
**Recommendation**: Enable compression
```go
config := &tarfs.Config{
    LargeFileCompression: 50 * 1024, // 50KB threshold
}
fs, err := tarfs.NewFromBzip2FileWithConfig("media.tbz2", config)
```
- Reduces memory usage by 60-80% for compressible files
- Fast decompression (S2)
- Good for HTML, CSS, JS, JSON, XML, SVG

### Very Large Archives (> 100MB)
**Recommendation**: Use ARC cache
```go
config := &tarfs.Config{
    UseARCCache:  true,
    ARCCacheSize: 256,
    PreloadFiles: []string{"index.html", "main.css"},
}
fs, err := tarfs.NewFromFileWithConfig("huge.tar", config)
```
- Only frequently accessed files in memory
- Adaptive caching algorithm
- Critical files always available

### Mixed Workload
**Recommendation**: Compression + Cache
```go
config := &tarfs.Config{
    LargeFileCompression: 100 * 1024,
    UseARCCache:          true,
    ARCCacheSize:         512,
    PreloadFiles:         []string{"critical-file.html"},
}
```
- Maximum memory efficiency
- Intelligent caching
- Fast access to important files

## Examples

### Example 1: Static Web Server

```go
package main

import (
    "log"
    "net/http"
    "github.com/digitaldata-cz/tarfs"
    "github.com/gin-gonic/gin"
)

func main() {
    // Load web assets with compression
    config := &tarfs.Config{
        LargeFileCompression: 10 * 1024, // Compress files >10KB
        PreloadFiles: []string{
            "/index.html",
            "/favicon.ico",
        },
        UseARCCache:  true,
        ARCCacheSize: 100,
    }

    fs, err := tarfs.NewFromBzip2FileWithConfig("web.tbz2", config)
    if err != nil {
        log.Fatal(err)
    }

    r := gin.Default()
    
    // Serve static files
    r.NoRoute(func(c *gin.Context) {
        http.FileServer(fs).ServeHTTP(c.Writer, c.Request)
    })

    r.Run(":8080")
}
```

### Example 2: Documentation Server

```go
// Large documentation archive with search functionality
config := &tarfs.Config{
    UseARCCache:  true,
    ARCCacheSize: 200,
    PreloadFiles: []string{
        "/index.html",
        "/search.html",
        "/toc.html",
    },
    LargeFileCompression: 50 * 1024,
}

docs, err := tarfs.NewFromGzipFileWithConfig("docs.tar.gz", config)
```

## Performance Characteristics

| Operation | Time/op | Bytes/op | Allocs/op |
|-----------|---------|----------|-----------|
| Open      | 90 ns   | 128 B    | 4         |
| Readdir   | 1539 ns | 4547 B   | 10        |
| Read      | 1031 ns | 10368 B  | 5         |

### Memory Usage

Without compression:
- Metadata: ~180 bytes/file
- Data: Full file size
- Total: File size + 180 bytes

With compression (S2):
- Metadata: ~180 bytes/file  
- Data: 20-40% of original size (text files)
- Total: Compressed size + 180 bytes

With ARC cache:
- Metadata: ~180 bytes/file (all files)
- Data: Only cached files in memory
- Preloaded: Full size in memory (bypasses cache)

## Advanced Topics

### Thread Safety
All operations are thread-safe for concurrent reads. Multiple goroutines can safely call `Open()`, `Exists()`, and `Readdir()` simultaneously.

### Compression Algorithm
TarFS supports multiple compression algorithms from Go standard library:

**Available Algorithms:**
- `CompressionDeflate` - DEFLATE (default, `compress/flate`)
- `CompressionGzip` - GZIP (`compress/gzip`)
- `CompressionZlib` - ZLIB (`compress/zlib`)

**Compression Levels:** 1-9 (default: 5)
- `1` (`flate.BestSpeed`) - Fastest compression, larger size
- `5` (default) - Good balance between speed and size
- `9` (`flate.BestCompression`) - Best compression, slower
- `-1` (`flate.DefaultCompression`) - Library default (usually 6)

All algorithms provide:
- **Good compression ratio**: Industry-standard algorithms
- **Fast decompression**: Highly optimized in Go runtime
- **Zero external dependencies**: Part of Go standard library
- **Production proven**: Widely used and battle-tested

### ARC Cache Algorithm
The Adaptive Replacement Cache maintains four lists:
- **T1**: Recently accessed items (LRU)
- **T2**: Frequently accessed items (LFU)
- **B1/B2**: Ghost entries for adaptive tuning

The algorithm automatically adjusts between recency and frequency based on access patterns, providing better hit rates than simple LRU.

## Documentation

- [ADVANCED_FEATURES.md](ADVANCED_FEATURES.md) - Advanced features guide (compression & caching)
- [CHANGELOG.md](CHANGELOG.md) - Version history and migration guide
- [GoDoc](https://godoc.org/github.com/digitaldata-cz/tarfs) - API documentation

## License

See [LICENSE](LICENSE) file.
