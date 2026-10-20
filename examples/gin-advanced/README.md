# Gin example: TarFS compression and decoded cache

The example serves `web.tbz2` using the local v2 library. It enables DEFLATE
compression at 10 KiB, an ARC cache of up to 100 decoded files and decoded
preloads for the three bundled assets. Source payloads remain in memory;
ARC does not make archive loading lazy or replace the source storage.

```sh
GOWORK=off go run .
```

Visit `/`, `/ping` and `/stats` on port 8080. Change the archive and configuration
in `main.go` to measure a realistic workload. This tiny example does not establish
a general memory-saving percentage. See the root README and PERFORMANCE.md.
