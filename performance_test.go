package tarfs

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
)

func BenchmarkCompressedStream(b *testing.B) {
	data := bytes.Repeat([]byte("compressed web asset\n"), 13108)
	var archive bytes.Buffer
	w := tar.NewWriter(&archive)
	if err := w.WriteHeader(&tar.Header{Name: "asset", Mode: 0600, Size: int64(len(data))}); err != nil {
		b.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		b.Fatal(err)
	}
	if err := w.Close(); err != nil {
		b.Fatal(err)
	}
	config := DefaultConfig()
	config.LargeFileCompression = 1
	filesystem, err := NewFromReaderWithConfig(&archive, config)
	if err != nil {
		b.Fatal(err)
	}
	buf := make([]byte, 8192)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, err := filesystem.Open("asset")
		if err != nil {
			b.Fatal(err)
		}
		for {
			_, err := f.Read(buf)
			if err == io.EOF {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
		}
		if err := f.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDirectoryPages exposes rescanning an entire directory for each page.
func BenchmarkDirectoryPages(b *testing.B) {
	for _, count := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			filesystem := createTestFS(b, count, 8)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				dir, err := filesystem.Open("/")
				if err != nil {
					b.Fatal(err)
				}
				for {
					_, err = dir.Readdir(16)
					if err == io.EOF {
						break
					}
					if err != nil {
						b.Fatal(err)
					}
				}
				if err := dir.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkArchiveIndex separates archive construction from filesystem loading.
func BenchmarkArchiveIndex(b *testing.B) {
	for _, count := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			var archive bytes.Buffer
			w := tar.NewWriter(&archive)
			for i := 0; i < count; i++ {
				if err := w.WriteHeader(&tar.Header{Name: fmt.Sprintf("asset/%06d", i), Mode: 0600, Size: 1}); err != nil {
					b.Fatal(err)
				}
				if _, err := w.Write([]byte("x")); err != nil {
					b.Fatal(err)
				}
			}
			if err := w.Close(); err != nil {
				b.Fatal(err)
			}
			data := archive.Bytes()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				filesystem, err := NewFromReader(bytes.NewReader(data))
				if err != nil {
					b.Fatal(err)
				}
				if !filesystem.Exists("asset/000000") {
					b.Fatal("missing entry")
				}
			}
		})
	}
}

func BenchmarkCachedParallelReads(b *testing.B) {
	for _, cached := range []bool{false, true} {
		b.Run(fmt.Sprint(cached), func(b *testing.B) {
			data := []byte(strings.Repeat("parallel compressed asset\n", 11000))
			var archive bytes.Buffer
			w := tar.NewWriter(&archive)
			if err := w.WriteHeader(&tar.Header{Name: "asset", Mode: 0600, Size: int64(len(data))}); err != nil {
				b.Fatal(err)
			}
			if _, err := w.Write(data); err != nil {
				b.Fatal(err)
			}
			if err := w.Close(); err != nil {
				b.Fatal(err)
			}
			config := DefaultConfig()
			config.LargeFileCompression = 1
			config.UseARCCache = cached
			filesystem, err := NewFromReaderWithConfig(&archive, config)
			if err != nil {
				b.Fatal(err)
			}
			// Warm a shared cache so this measures reads rather than first publication.
			initial, err := filesystem.Open("asset")
			if err != nil {
				b.Fatal(err)
			}
			if _, err := io.Copy(io.Discard, initial); err != nil {
				b.Fatal(err)
			}
			if err := initial.Close(); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				buf := make([]byte, 8192)
				for pb.Next() {
					f, err := filesystem.Open("asset")
					if err != nil {
						b.Error(err)
						return
					}
					for {
						_, err = f.Read(buf)
						if err == io.EOF {
							break
						}
						if err != nil {
							b.Error(err)
							return
						}
					}
					if err := f.Close(); err != nil {
						b.Error(err)
						return
					}
				}
			})
		})
	}
}
