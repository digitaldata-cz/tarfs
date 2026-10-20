# Compression and ARC configuration

TarFS is an in-memory filesystem. It finishes reading the archive before returning
and keeps each source payload, optionally compressed. It never reopens the archive
for a cache miss. See [README](README.md) for the full API and migration guide.

```go
config := tarfs.DefaultConfig()
config.LargeFileCompression = 50 * 1024
config.CompressionAlgorithm = tarfs.CompressionDeflate
config.CompressionLevel = 5
config.UseARCCache = true
config.ARCCacheSize = 128
config.PreloadFiles = []string{"index.html"}
```

Compression is useful for sufficiently compressible assets. It increases archive
loading work and adds first-read decoding; test actual assets before enabling it.
Incompressible content stays uncompressed. `CompressionNone` explicitly disables
compression, even with a nonzero threshold. Invalid enabled settings are errors.

Without ARC, each opened compressed handle owns one decoded copy until it closes.
With ARC, repeated opens share decoded data. Cache entries are bounded by count,
not total bytes; source compressed bytes and active-handle references also remain
live. Cache-only configuration cannot remove the filesystem's source storage.

Preloads stay decoded outside ARC, so opening them needs no decompression. Missing
preloads and directory names fail construction. There is no separate filesystem
Close operation; constructors release source descriptors, and normal Go lifetimes
own filesystem/cache memory. File handle Close releases its decoded reference.

Choose an explicit archive/configuration based on the workload; no failure silently
switches to another compression algorithm or storage implementation. Performance
measurements and reproduction commands are in [PERFORMANCE.md](PERFORMANCE.md).
