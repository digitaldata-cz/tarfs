# TarFS Gin Advanced Example

This example demonstrates how to use TarFS with advanced features like compression and ARC cache in a Gin web application.

## Features Demonstrated

1. **Large File Compression**: Files larger than 10KB are automatically compressed in memory
2. **ARC Cache**: Intelligent caching with on-demand loading
3. **Preloaded Files**: Critical files kept in memory permanently
4. **Stats Endpoint**: View configuration details

## Configuration Used

```go
config := &tarfs.Config{
    LargeFileCompression: 10 * 1024,  // Compress files >10KB
    UseARCCache:          true,         // Enable ARC cache
    ARCCacheSize:         100,          // Cache up to 100 files
    PreloadFiles: []string{             // Always keep in memory
        "index.html",
        "style.css",
        "hello-world.png",
    },
}
```

## Memory Impact

**Without optimization**:
- Archive size: ~20KB (3 files)
- Memory usage: ~20KB + overhead

**With optimization** (if archive was larger):
- Compressed files: 30-70% smaller
- Cached files: Only frequently accessed in memory
- Preloaded files: Always available instantly

## Running

```bash
go run main.go
```

Then visit:
- http://localhost:8080/ - Static files from archive
- http://localhost:8080/ping - API endpoint
- http://localhost:8080/stats - Configuration stats

## Comparison with Basic Example

**Basic example** (`examples/gin/main.go`):
```go
fs, err := tarfs.NewFromBzip2File("web.tbz2")
```
- Simple
- All files in memory
- Good for small archives

**Advanced example** (this):
```go
fs, err := tarfs.NewFromBzip2FileWithConfig("web.tbz2", config)
```
- Optimized for memory
- Intelligent caching
- Good for large archives

## When to Use This Approach

Use advanced configuration when:
- Archive is > 10MB
- Archive contains many large files
- Only subset of files frequently accessed
- Running in memory-constrained environment
- Want to minimize memory footprint

## Files

- `main.go` - Application with advanced TarFS configuration
- `go.mod` - Module dependencies
- `web.tbz2` - (symlink to ../gin/web.tbz2) Sample archive

## See Also

- [ADVANCED_FEATURES.md](../../ADVANCED_FEATURES.md) - Detailed feature documentation
- [README.md](../../README.md) - Main TarFS documentation
