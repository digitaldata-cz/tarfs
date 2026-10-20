# TarFS

An immutable, in-memory HTTP filesystem for TAR, gzip TAR and bzip2 TAR archives.
The library uses only the Go standard library and supports Go 1.22 and newer.

## Install and upgrade

```sh
go get github.com/digitaldata-cz/tarfs/v2@v2.0.1
```

```go
import "github.com/digitaldata-cz/tarfs/v2"
```

Version 2 uses the `/v2` module path required by Go. The original `v2.0.0`
tag declared the v1 module path and cannot be consumed as a valid v2 Go module.
Use v2.0.1 or newer; existing tags are not rewritten. Consumers upgrading from
v1 must update their import path as well as their module requirement.

## Serve an archive

```go
web, err := tarfs.NewFromGzipFile("admin.tgz")
if err != nil {
    return err
}
http.Handle("/", http.FileServer(web))
```

`NewFromFile`, `NewFromGzipFile`, `NewFromBzip2File` and `NewFromReader`
load the complete archive before returning. Each has a `WithConfig` variant.
File constructors close their source file; reader constructors do not close the
caller's reader. Subsequent reads do not reopen or extract the archive.

Filesystems can be shared between requests. Each `Open` owns an independent
read/seek/directory cursor, and operations on a single handle are synchronized.
HTTP range requests work through `http.FileServer`.

## Compression and cache

```go
config := tarfs.DefaultConfig()
config.LargeFileCompression = 50 * 1024
config.UseARCCache = true
config.ARCCacheSize = 128
config.PreloadFiles = []string{"index.html"}
web, err := tarfs.NewFromGzipFileWithConfig("admin.tgz", config)
```

- `DefaultConfig()` disables compression by threshold, with DEFLATE level 5
  ready for opt-in. `nil` selects those defaults. A zero `Config{}` disables
  compression and caching; `CompressionNone` is an explicit choice, not an
  alias for DEFLATE.
- Compression supports DEFLATE, gzip and zlib. Levels are -2 (Huffman only),
  -1 (standard default), 0 and 1–9. Invalid enabled settings return errors.
  An asset stays uncompressed when its compressed representation is larger.
- Without ARC, a compressed asset is decoded once per opened handle, not once
  per `Read`. `Seek` and metadata access alone do not decode it.
- With ARC, decoded assets are shared across opens. Concurrent misses for one
  asset are coalesced; different assets can decode concurrently. Capacity
  bounds the number of cached decoded files, not their bytes. Zero uses 128.
- Preloaded files remain decoded outside ARC. Missing files and directories
  in `PreloadFiles` fail construction. Preloads only apply with ARC enabled.
- Closing a handle releases its decoded reference. Further operations return
  `fs.ErrClosed`; closing twice is harmless.

ARC is a decoded-data cache, **not lazy archive loading**. The source payload of
**every** file remains in memory, compressed where configured. Cache-only mode
cannot eliminate that source storage. Active handles can keep decoded bytes
alive after eviction. Memory savings depend on contents and workload; no fixed
percentage is guaranteed. Supply trusted, appropriately sized archives and bound
untrusted reader input at the application's boundary.

## Filesystem behavior

- Files and directories are indexed once. Missing parent directories are
  synthesized. Listings are sorted and paginated without rescanning the index.
- Paths are rooted virtually. Leading slashes, dot components and Windows
  backslashes normalize consistently on all platforms. Nothing is extracted.
- Repeated entries of the same kind use the last entry, as in TAR. A path
  cannot be both a file and a directory, including through implied parents.
- Regular files and directories are supported. Links, devices and other special
  entries fail explicitly. PAX/GNU metadata handled by `archive/tar` is accepted.
- Explicit directory modes/timestamps are preserved. Synthesized directory
  timestamps are zero, rather than the machine's current time.
- gzip and bzip2 constructors verify the outer stream to its end, including
  checksums/trailers after TAR's end marker. Truncated payloads fail loading.

The public API remains `http.FileSystem` plus `Exists`; it does not claim the
stricter path rules or the different `Open` return signature of `io/fs.FS`.

## Verification and performance

```sh
GOWORK=off go test -race -shuffle=on -count=1 -coverprofile=coverage.out ./...
GOWORK=off go vet ./...
GOWORK=off go test -run '^$' -bench . -benchmem -count=5
GOWORK=off go test -run '^$' -fuzz '^FuzzArchive$' -fuzztime=30s -parallel=2
GOWORK=off go test -run '^$' -fuzz '^FuzzCompressionRoundTrip$' -fuzztime=30s -parallel=2
```

Benchmarks cover opens, full/paged directory listings, archive indexing,
compressed streaming, and cached/uncached parallel reads. Compare the same Go
version, machine, fixture, buffer size and benchmark duration; `B/op` measures
cumulative allocations, not retained RSS. See [PERFORMANCE.md](PERFORMANCE.md)
for the measured comparison and reproduction commands.

CI runs race tests on Linux, Windows and macOS with current stable Go, plus
Go 1.22 on Linux and Windows. It also checks nested Gin examples, fuzz seeds/runs
and security analysis. The examples use a newer Go version required by Gin.
Gin is confined to the example modules and is not a library dependency.
