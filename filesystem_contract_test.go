package tarfs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"container/list"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func openTestFile(t testing.TB, filesystem FileSystem, name string) http.File {
	t.Helper()
	f, err := filesystem.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestIndependentHandles(t *testing.T) {
	filesystem, err := NewFromReader(buildTar(t, map[string]string{"file": "ABC"}))
	if err != nil {
		t.Fatal(err)
	}
	first := openTestFile(t, filesystem, "file")
	one := make([]byte, 1)
	if _, err := first.Read(one); err != nil || string(one) != "A" {
		t.Fatalf("first read: %q, %v", one, err)
	}
	second := openTestFile(t, filesystem, "file")
	if _, err := first.Read(one); err != nil || string(one) != "B" {
		t.Fatalf("opening second handle changed first: %q, %v", one, err)
	}
	if _, err := second.Read(one); err != nil || string(one) != "A" {
		t.Fatalf("second cursor: %q, %v", one, err)
	}
}

func TestReaddirCursorAndOrdering(t *testing.T) {
	filesystem, err := NewFromReader(buildTar(t, map[string]string{"z": "", "a": "", "m": ""}))
	if err != nil {
		t.Fatal(err)
	}
	dir := openTestFile(t, filesystem, "/")
	first, err := dir.Readdir(1)
	if err != nil || len(first) != 1 || first[0].Name() != "a" {
		t.Fatalf("first entry: %v, %v", first, err)
	}
	remaining, err := dir.Readdir(0)
	if err != nil || len(remaining) != 2 || remaining[0].Name() != "m" || remaining[1].Name() != "z" {
		t.Fatalf("remaining entries: %v, %v", remaining, err)
	}
	empty, err := dir.Readdir(-1)
	if err != nil || len(empty) != 0 {
		t.Fatalf("read-all at EOF: %v, %v", empty, err)
	}
	if _, err := dir.Readdir(1); !errors.Is(err, io.EOF) {
		t.Fatalf("positive read at EOF: %v", err)
	}
	fresh := openTestFile(t, filesystem, "/")
	entries, err := fresh.Readdir(3)
	if err != nil || len(entries) != 3 {
		t.Fatalf("exact page should succeed: %v, %v", entries, err)
	}
}

func TestEmptyDirectoryPositiveReadReturnsEOF(t *testing.T) {
	filesystem, err := NewFromReader(buildTar(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openTestFile(t, filesystem, "/").Readdir(1); !errors.Is(err, io.EOF) {
		t.Fatalf("empty directory: %v", err)
	}
}

func TestClosedHandleAndSeekOverflow(t *testing.T) {
	filesystem, err := NewFromReader(buildTar(t, map[string]string{"file": "ABC"}))
	if err != nil {
		t.Fatal(err)
	}
	f := openTestFile(t, filesystem, "file")
	if _, err := f.Seek(math.MaxInt64, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(1, io.SeekCurrent); err == nil {
		t.Fatal("overflowing seek accepted")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Read(make([]byte, 1)); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("read after close: %v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("seek after close: %v", err)
	}
	if _, err := f.Stat(); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("stat after close: %v", err)
	}
	if _, err := f.Readdir(1); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("readdir after close: %v", err)
	}
	other := openTestFile(t, filesystem, "file")
	if content, err := io.ReadAll(other); err != nil || string(content) != "ABC" {
		t.Fatalf("close affected other handle: %q, %v", content, err)
	}
}

func TestVirtualPathsArePlatformIndependent(t *testing.T) {
	filesystem, err := NewFromReader(buildTar(t, map[string]string{"dir/file": "data"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dir/file", "/dir/file", `dir\file`, `\dir\file`, "./dir/file", "../../dir/file"} {
		if !filesystem.Exists(name) {
			t.Errorf("Exists(%q) = false", name)
		}
		f := openTestFile(t, filesystem, name)
		if got, err := io.ReadAll(f); err != nil || string(got) != "data" {
			t.Errorf("Open(%q): %q, %v", name, got, err)
		}
	}
	if _, err := filesystem.Open("bad\x00name"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("invalid path: %v", err)
	}
	if _, err := filesystem.Open("missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
	if _, err := openTestFile(t, filesystem, "dir/file").Readdir(1); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("readdir file: %v", err)
	}
}

func TestCompressedReadsReuseDecodedPayload(t *testing.T) {
	for _, useCache := range []bool{false, true} {
		t.Run(fmt.Sprint(useCache), func(t *testing.T) {
			config := DefaultConfig()
			config.LargeFileCompression = 1
			config.UseARCCache = useCache
			filesystem, err := NewFromReaderWithConfig(buildTar(t, map[string]string{"file": strings.Repeat("ABC", 8192)}), config)
			if err != nil {
				t.Fatal(err)
			}
			f := openTestFile(t, filesystem, "file")
			one := make([]byte, 1)
			if _, err := f.Read(one); err != nil {
				t.Fatal(err)
			}
			allocs := testing.AllocsPerRun(20, func() {
				if _, err := f.Seek(0, io.SeekStart); err != nil {
					panic(err)
				}
				if _, err := f.Read(one); err != nil {
					panic(err)
				}
			})
			if allocs != 0 {
				t.Fatalf("each read allocates %.0f times after first decompression", allocs)
			}
		})
	}
}

func TestConcurrentHTTPFileAndRangeReads(t *testing.T) {
	for _, algorithm := range []CompressionAlgorithm{CompressionNone, CompressionDeflate, CompressionGzip, CompressionZlib} {
		for _, cached := range []bool{false, true} {
			t.Run(fmt.Sprintf("algorithm=%d/cache=%t", algorithm, cached), func(t *testing.T) {
				content := strings.Repeat("0123456789ABCDEF", 2048)
				config := DefaultConfig()
				config.LargeFileCompression = 1
				config.CompressionAlgorithm = algorithm
				config.UseARCCache = cached
				config.ARCCacheSize = 1
				filesystem, err := NewFromReaderWithConfig(buildTar(t, map[string]string{"asset.txt": content, "other": "other", "dir/nested": "nested"}), config)
				if err != nil {
					t.Fatal(err)
				}
				handler := http.FileServer(filesystem)
				var wg sync.WaitGroup
				for i := 0; i < 12; i++ {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						for j := 0; j < 8; j++ {
							request := httptest.NewRequest(http.MethodGet, "/asset.txt", nil)
							expected, status := content, http.StatusOK
							if (i+j)%2 == 0 {
								request.Header.Set("Range", "bytes=5-28")
								expected, status = content[5:29], http.StatusPartialContent
							}
							response := httptest.NewRecorder()
							handler.ServeHTTP(response, request)
							if response.Code != status || response.Body.String() != expected {
								t.Errorf("HTTP read: status %d, length %d", response.Code, response.Body.Len())
								return
							}
							f, err := filesystem.Open("other")
							if err != nil {
								t.Error(err)
								return
							}
							if _, err := io.ReadAll(f); err != nil {
								t.Error(err)
							}
							_ = f.Close()
						}
					}(i)
				}
				wg.Wait()
			})
		}
	}
}

func TestGzipTrailerValidation(t *testing.T) {
	var archive bytes.Buffer
	w := gzip.NewWriter(&archive)
	if _, err := w.Write(buildTar(t, map[string]string{"file": "data"}).Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"checksum", "truncated"} {
		t.Run(variant, func(t *testing.T) {
			data := append([]byte(nil), archive.Bytes()...)
			if variant == "checksum" {
				data[len(data)-8] ^= 1
			} else {
				data = data[:len(data)-5]
			}
			name := filepath.Join(t.TempDir(), "archive.tar.gz")
			if err := os.WriteFile(name, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewFromGzipFile(name); err == nil {
				t.Fatal("invalid gzip trailer accepted")
			}
		})
	}
}

func TestArchiveNamespaceConflicts(t *testing.T) {
	for _, names := range [][]string{{"a", "a/b"}, {"a/b", "a"}} {
		var archive bytes.Buffer
		w := tar.NewWriter(&archive)
		for _, name := range names {
			if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: 1}); err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := NewFromReader(&archive); err == nil {
			t.Errorf("file/directory conflict accepted: %v", names)
		}
	}
}

func checkARC(t testing.TB, c *arcCache) {
	t.Helper()
	if c.p < 0 || c.p > c.capacity || len(c.cache) > c.capacity {
		t.Fatalf("invalid capacity/p: %d/%d/%d", c.p, len(c.cache), c.capacity)
	}
	if len(c.elements) != len(c.cache) || len(c.inT2) != len(c.cache) || len(c.cache) != c.t1.Len()+c.t2.Len() {
		t.Fatal("resident bookkeeping retained evicted entries")
	}
	if len(c.ghosts) != len(c.ghostList) || len(c.ghosts) != c.b1.Len()+c.b2.Len() || len(c.cache)+len(c.ghosts) > 2*c.capacity || c.t1.Len()+c.b1.Len() > c.capacity {
		t.Fatal("invalid ghost bounds")
	}
	seen := map[string]bool{}
	for _, list := range []*list.List{c.t1, c.t2, c.b1, c.b2} {
		for e := list.Front(); e != nil; e = e.Next() {
			key, ghost := e.Value.(string)
			if !ghost {
				key = e.Value.(*arcEntry).key
			}
			if seen[key] {
				t.Fatalf("key %q in multiple lists", key)
			}
			seen[key] = true
			if ghost {
				if c.ghosts[key] != e {
					t.Fatal("ghost index mismatch")
				}
			} else {
				if c.elements[key] != e || c.cache[key] != e.Value.(*arcEntry) || c.inT2[key] != (list == c.t2) {
					t.Fatal("resident index mismatch")
				}
			}
		}
	}
}

func TestARCBookkeepingAndAdaptation(t *testing.T) {
	for _, capacity := range []int{1, 2, 7, 32} {
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			c := newARCCache(capacity)
			r := rand.New(rand.NewSource(572))
			for i := 0; i < 3000; i++ {
				key := fmt.Sprint(r.Intn(capacity*4 + 1))
				if r.Intn(3) == 0 {
					c.Get(key)
				} else {
					want := c.p
					if _, exists := c.ghosts[key]; exists {
						if c.ghostInB1(key) {
							want = min(capacity, c.p+max(1, c.b2.Len()/c.b1.Len()))
						} else {
							want = max(0, c.p-max(1, c.b1.Len()/c.b2.Len()))
						}
					}
					c.Put(key, []byte(key))
					if c.p != want {
						t.Fatalf("ARC target: got %d, want %d", c.p, want)
					}
				}
				checkARC(t, c)
			}
		})
	}
}
