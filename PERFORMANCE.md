# Performance verification for v2.0.1

Measured on 2026-10-20, Apple M3 Max, darwin/arm64, Go 1.27.2,
GOMAXPROCS=14. Baseline: commit `09cf74edf3a69101062ea04dcaeec623db04b184`
(the original v2.0.0 tag). Revised code: v2.0.1. Both versions used the exact
same `performance_test.go` benchmark source, which was copied into the baseline
checkout, plus the existing benchmarks. Runs were sequential, with five samples
per case and `-benchtime=200ms`. Values below are sample medians, not a
statistical significance claim.

| Benchmark | Before ns/op | After ns/op | Before B/op | After B/op | Allocations before → after |
|---|---:|---:|---:|---:|---:|
| `CompressedStream` | 2,692,047.00 | 50,998.00 | 24,029,229 | 319,161 | 877 → 9 |
| `DirectoryPages/100` | 8,637.00 | 384.20 | 31,344 | 1,696 | 57 → 8 |
| `DirectoryPages/1000` | 793,466.00 | 3,513.00 | 2,216,708 | 16,096 | 694 → 64 |
| `DirectoryPages/10000` | 100,636,250.00 | 33,980.00 | 416,230,768 | 160,096 | 11257 → 626 |
| `ArchiveIndex/100` | 183,499.00 | 173,637.00 | 121,456 | 116,326 | 1734 → 1732 |
| `ArchiveIndex/1000` | 2,550,633.00 | 1,825,648.00 | 1,232,709 | 1,183,568 | 17050 → 17048 |
| `ArchiveIndex/10000` | 101,545,833.00 | 18,661,479.00 | 12,355,512 | 11,866,855 | 170119 → 170114 |
| `CachedParallelReads/false` | 2,569,103.00 | 31,562.00 | 25,012,337 | 327,377 | 905 → 9 |
| `CachedParallelReads/true` | 2,624,098.00 | 728.00 | 25,012,246 | 104 | 904 → 2 |
| `Open` | 56.47 | 49.09 | 96 | 96 | 1 → 1 |
| `Readdir` | 1,364.00 | 256.80 | 4,560 | 1,888 | 9 → 2 |

## What changed

- A compressed file is decoded once per handle, into one pre-sized buffer.
  Previously every chunk read (including the EOF check) decoded the full file
  again. The approximately 256 KiB streaming case is about 53 times faster,
  with roughly 75 times fewer cumulatively allocated bytes.
- Directory metadata is indexed and sorted once. `Readdir` copies only the next
  page. The old implementation rebuilt the whole directory for every page;
  reading a 10,000-entry directory in pages of 16 allocated over 416 MB.
- Archive indexing no longer scans a growing sibling list to deduplicate each
  already-unique map entry. Deep parent paths also avoid repeated cleaning.
- ARC holds decoded bytes rather than another reference to compressed source
  bytes. Ghost operations use direct indexes, adaptive ratios are corrected,
  and evictions remove all bookkeeping. Concurrent cold reads of one asset
  share a single decode.
- Normalized absolute paths retain the fast, one-allocation `Open` path.

`CachedParallelReads` uses a warmed cache and 14 benchmark workers. Its ns/op
is aggregate benchmark throughput, not single-request latency. Assets are
synthetic and compressible; the streaming buffer is 8 KiB. These measurements
contain no network, TLS, browser rendering or application startup. In particular,
DPSystem's default configuration does not enable in-memory compression, so the
compressed-stream ratio is not a measured DPSystem speedup.

`B/op` measures cumulative allocations. It is not retained heap or RSS. Source
payloads remain in memory even with ARC; a cache entry limit is not a byte limit.

## Reproduce

```sh
GOWORK=off go test -run '^$' \
  -bench 'Benchmark(Open|Readdir|CompressedStream|DirectoryPages|ArchiveIndex|CachedParallelReads)$' \
  -benchmem -benchtime=200ms -count=5
```

For a historical comparison, create a separate checkout of the baseline and copy
this release's `performance_test.go` into it. Run the command in each checkout
with the same Go toolchain and without other builds or fuzz workers competing.
Do not treat these host-specific numbers as portable regression thresholds.
The test suite instead enforces zero new allocations on reads after a handle's
first decode, independent file cursors, correct paginated listings and cache
bounds; CI includes a benchmark smoke run.
