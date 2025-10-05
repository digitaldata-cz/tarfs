# Advanced Features Guide

This document describes the advanced features of TarFS: Large File Compression and ARC Cache.

## Large File Compression

### Overview
Large File Compression automatically compresses files in memory that exceed a configurable size threshold. TarFS supports multiple compression algorithms from Go standard library:

**Available Algorithms:**
- **DEFLATE** (default): Good balance, widely compatible
- **GZIP**: Includes header/checksum, slightly more overhead
- **ZLIB**: Includes header/checksum, similar to GZIP

**Compression Levels:** 1-9
- Level 1: Fastest compression
- Level 5: Default - good balance
- Level 9: Best compression

**Benefits:**
- **Good compression ratio**: 60-80% for text files (HTML, CSS, JS, JSON)
- **Configurable**: Choose algorithm and level based on needs
- **Fast decompression**: ~200-500 MB/s throughput
- **Zero dependencies**: Part of Go standard library
- **Transparent**: Files are automatically decompressed when read

### Configuration

```go
// Basic configuration with defaults
config := &tarfs.Config{
    LargeFileCompression: 50 * 1024, // Compress files >= 50KB
}

// Custom algorithm and level
config := &tarfs.Config{
    LargeFileCompression: 50 * 1024,
    CompressionAlgorithm: tarfs.CompressionDeflate, // or CompressionGzip, CompressionZlib
    CompressionLevel:     flate.BestCompression,    // Maximum compression
}

// Fast compression for quick startup
config := &tarfs.Config{
    LargeFileCompression: 30 * 1024,
    CompressionAlgorithm: tarfs.CompressionDeflate,
    CompressionLevel:     flate.BestSpeed, // Fast but less compression
}

fs, err := tarfs.NewFromFileWithConfig("archive.tar", config)
```

### When to Use

**Good candidates for compression:**
- HTML files
- JavaScript files
- CSS files
- JSON/XML data files
- SVG images
- Text documentation

**Not recommended for:**
- Already compressed files (JPEG, PNG, GIF, ZIP, GZIP)
- Binary executables
- Small files (< 10KB) - overhead not worth it

### Performance Impact

**Compression time**: Only at startup, files are compressed once when loading the archive.

**Decompression time**: 
- 100KB file: ~0.1ms
- 1MB file: ~1ms
- 10MB file: ~10ms

**Memory savings example:**
```
Original archive: 1000 HTML files × 200KB = 200MB
Compressed in RAM: 1000 files × 40KB = 40MB
Savings: 160MB (80% reduction)
```

### Example Use Case

```go
// Web application with large HTML/JS/CSS files
config := &tarfs.Config{
    LargeFileCompression: 30 * 1024, // 30KB threshold
}

fs, err := tarfs.NewFromBzip2FileWithConfig("webapp.tbz2", config)
if err != nil {
    log.Fatal(err)
}

// Files < 30KB: stored uncompressed
// Files >= 30KB: stored compressed, decompressed on read
http.Handle("/", http.FileServer(fs))
```

## ARC Cache (Adaptive Replacement Cache)

### Overview
ARC Cache enables on-demand loading of files with intelligent caching. Instead of loading all files into memory at startup, files are loaded only when first accessed and kept in a cache with adaptive eviction.

### Key Concepts

**1. On-demand Loading**
- At startup: Only metadata is loaded
- First access: File data is loaded and cached
- Subsequent access: Served from cache (if still cached)

**2. Adaptive Algorithm**
The cache maintains two strategies:
- **LRU (Recent)**: Recently accessed files
- **LFU (Frequent)**: Frequently accessed files
- Algorithm automatically balances between them based on access patterns

**3. Preloaded Files**
Files specified in `PreloadFiles` are:
- Loaded immediately at startup
- Kept in memory permanently
- Never evicted from cache
- Always instantly available

### Configuration

```go
config := &tarfs.Config{
    UseARCCache:  true,
    ARCCacheSize: 128,      // Keep up to 128 files in cache
    PreloadFiles: []string{
        "index.html",       // Critical files
        "app.css",
        "app.js",
    },
}

fs, err := tarfs.NewFromFileWithConfig("large.tar", config)
```

### Cache Behavior

**Scenario 1: File in preload list**
```
Request → File already in memory → Serve immediately
```

**Scenario 2: File not preloaded, first access**
```
Request → Load from archive → Add to cache → Serve
```

**Scenario 3: File in cache**
```
Request → Found in cache → Move to front (update LRU/LFU) → Serve
```

**Scenario 4: Cache full, new file needed**
```
Request → Cache full → Evict least valuable file → Load new file → Serve
```

### When to Use ARC Cache

**Ideal for:**
- **Large archives** (>100MB) with thousands of files
- **Sparse access patterns**: Only 10-20% of files regularly accessed
- **Predictable critical files**: You know which files are always needed
- **Memory-constrained environments**: Want to minimize memory usage

**Not recommended for:**
- Small archives (<10MB): Overhead not worth it
- Archives where all files are accessed: Just load everything
- Unpredictable access patterns: Cache won't help much

### Memory Usage Comparison

**Without cache** (default):
```
1000 files × 100KB average = 100MB in memory
Startup: Load all 100MB
```

**With cache** (ARCCacheSize=100, 3 preloaded):
```
Preloaded: 3 files × 100KB = 300KB
Cache: Up to 100 files × 100KB = 10MB
Total max: ~10.3MB
Savings: 89.7MB (90% reduction)
```

### Performance Characteristics

**Startup time:**
- Without cache: O(n) - read all files
- With cache: O(n) - index all files, load only preloaded

**First access (cache miss):**
- Additional overhead: File must be loaded from archive
- For io.Reader based archives: Not supported (need file path)
- Impact: ~1-5ms depending on file size

**Cache hit:**
- Same as without cache: Instant access

**Cache eviction:**
- Happens automatically when cache is full
- Negligible overhead (~1μs)

### Example: Documentation Server

```go
// Large documentation archive with 10,000 pages
// Only 100-200 pages accessed regularly
config := &tarfs.Config{
    UseARCCache:  true,
    ARCCacheSize: 200,  // Keep 200 most accessed pages
    PreloadFiles: []string{
        "/index.html",      // Always needed
        "/search.html",     // Always needed
        "/toc.html",        // Always needed
    },
    LargeFileCompression: 50 * 1024,  // Compress large pages
}

docs, err := tarfs.NewFromGzipFileWithConfig("docs.tar.gz", config)
if err != nil {
    log.Fatal(err)
}

// Memory usage:
// - Without cache: ~500MB (all pages)
// - With cache: ~20-30MB (preloaded + cached pages)
// - Savings: ~470MB (94%)
```

## Combining Compression and Cache

For maximum efficiency, use both features together:

```go
config := &tarfs.Config{
    // Compress large files
    LargeFileCompression: 100 * 1024,  // 100KB threshold

    // Use cache for on-demand loading
    UseARCCache:  true,
    ARCCacheSize: 256,

    // Preload critical files
    PreloadFiles: []string{
        "index.html",
        "main.css",
        "app.js",
    },
}

fs, err := tarfs.NewFromFileWithConfig("app.tar", config)
```

**How it works:**
1. At startup: Load and compress preloaded files
2. On first access: Load file, compress if needed, add to cache
3. On cache eviction: Remove from memory
4. On subsequent access: Load again if needed

**Memory calculation:**
```
Preloaded: 3 files × 200KB × 30% (compressed) = 180KB
Cache: 256 files × 100KB × 30% (compressed) = 7.7MB
Total max: ~7.9MB

Without compression/cache:
Total: 10,000 files × 150KB = 1.5GB
Savings: 1.49GB (99.5%)
```

## Best Practices

### 1. Start with Defaults
```go
// Simple case
fs, err := tarfs.NewFromFile("app.tar")
```

### 2. Add Compression for Large Archives
```go
// If archive >10MB with compressible files
config := &tarfs.Config{
    LargeFileCompression: 50 * 1024,
}
fs, err := tarfs.NewFromFileWithConfig("app.tar", config)
```

### 3. Add Cache for Very Large Archives
```go
// If archive >100MB and sparse access
config := &tarfs.Config{
    LargeFileCompression: 50 * 1024,
    UseARCCache:  true,
    ARCCacheSize: 200,
    PreloadFiles: []string{"index.html"},
}
fs, err := tarfs.NewFromFileWithConfig("app.tar", config)
```

### 4. Tune Based on Metrics
Monitor your application:
- Memory usage
- Cache hit rate
- Request latency

Adjust:
- `LargeFileCompression`: Based on file size distribution
- `ARCCacheSize`: Based on working set size
- `PreloadFiles`: Based on access logs

## Troubleshooting

### High Memory Usage
**Problem**: Memory usage still high with cache enabled

**Solutions:**
1. Reduce `ARCCacheSize`
2. Increase `LargeFileCompression` threshold
3. Review `PreloadFiles` list (too many files?)

### Slow First Access
**Problem**: First access to files is slow

**Solutions:**
1. Add frequently accessed files to `PreloadFiles`
2. Increase `ARCCacheSize` to reduce evictions
3. Consider preloading more files if memory allows

### Poor Compression
**Problem**: Files not compressing well

**Solutions:**
1. Check file types (already compressed?)
2. Increase threshold (smaller files compress poorly)
3. Disable compression for specific file types

## Monitoring

### Memory Usage
```go
import "runtime"

var m runtime.MemStats
runtime.ReadMemStats(&m)
log.Printf("Alloc = %v MB", m.Alloc / 1024 / 1024)
```

### Cache Statistics
```go
// Access internal cache (if needed for monitoring)
tfs := fs.(*tarfs.TarFs)  // Type assertion
if tfs.cache != nil {
    log.Printf("Cache size: %d", tfs.cache.Len())
}
```

## See Also

- [README.md](README.md) - Main documentation
- [CHANGELOG.md](CHANGELOG.md) - Version history
- [compress/flate](https://pkg.go.dev/compress/flate) - DEFLATE compression (Go standard library)
