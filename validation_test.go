package tarfs

import (
	"archive/tar"
	"bytes"
	"compress/flate"
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
	"time"
)

func TestConfigValidationAndSnapshot(t *testing.T) {
	for _, variant := range []struct {
		name   string
		config Config
	}{
		{"threshold", Config{LargeFileCompression: -1}},
		{"capacity", Config{UseARCCache: true, ARCCacheSize: -1}},
		{"algorithm", Config{LargeFileCompression: 1, CompressionAlgorithm: 55}},
		{"level", Config{LargeFileCompression: 1, CompressionAlgorithm: CompressionDeflate, CompressionLevel: 10}},
		{"preload nul", Config{UseARCCache: true, PreloadFiles: []string{"bad\x00name"}}},
		{"preload missing", Config{UseARCCache: true, PreloadFiles: []string{"missing"}}},
		{"preload directory", Config{UseARCCache: true, PreloadFiles: []string{"/"}}},
	} {
		t.Run(variant.name, func(t *testing.T) {
			if filesystem, err := NewFromReaderWithConfig(buildTar(t, nil), &variant.config); err == nil || filesystem != nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
	if _, err := NewFromReader(nil); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
	config := DefaultConfig()
	config.LargeFileCompression = 1
	config.UseARCCache = true
	config.PreloadFiles = []string{"file"}
	content := strings.Repeat("payload", 1000)
	filesystem, err := NewFromReaderWithConfig(buildTar(t, map[string]string{"file": content}), config)
	if err != nil {
		t.Fatal(err)
	}
	config.PreloadFiles[0] = "missing"
	config.UseARCCache = false
	config.CompressionAlgorithm = 100
	if got, err := io.ReadAll(openTestFile(t, filesystem, "file")); err != nil || string(got) != content {
		t.Fatalf("config mutation affected FS: %v", err)
	}
	if entry := filesystem.(*tarFs).files["/file"]; entry.compressed != nil || string(entry.data) != content {
		t.Fatal("preloaded payload is not pinned decoded")
	}
}

func TestCompressionNoneAndIncompressibleData(t *testing.T) {
	data := make([]byte, 2048)
	_, _ = rand.New(rand.NewSource(8)).Read(data)
	for _, algorithm := range []CompressionAlgorithm{CompressionNone, CompressionDeflate, CompressionGzip, CompressionZlib} {
		compressed, err := compressData(data, algorithm, 5)
		if err != nil {
			t.Fatal(err)
		}
		if compressed.isCompressed {
			t.Fatal("compression enlarged incompressible payload")
		}
		got, err := decompressData(compressed)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("roundtrip: %v", err)
		}
	}
	empty, err := compressData(nil, CompressionDeflate, 5)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := decompressData(empty); err != nil || len(got) != 0 {
		t.Fatalf("empty payload: %v", err)
	}
	config := &Config{LargeFileCompression: 1, CompressionAlgorithm: CompressionNone}
	filesystem, err := NewFromReaderWithConfig(buildTar(t, map[string]string{"file": strings.Repeat("A", 1024)}), config)
	if err != nil {
		t.Fatal(err)
	}
	if filesystem.(*tarFs).files["/file"].compressed != nil {
		t.Fatal("CompressionNone was overridden")
	}
}

func TestCorruptCompressedPayloads(t *testing.T) {
	if _, err := decompressData(nil); err == nil {
		t.Fatal("missing payload accepted")
	}
	for _, algorithm := range []CompressionAlgorithm{CompressionDeflate, CompressionGzip, CompressionZlib} {
		original := bytes.Repeat([]byte("abcdef"), 1024)
		compressed, err := compressData(original, algorithm, 5)
		if err != nil {
			t.Fatal(err)
		}
		for _, variant := range []string{"short", "size small", "size large", "negative", "overflow", "unknown"} {
			t.Run(fmt.Sprintf("%d/%s", algorithm, variant), func(t *testing.T) {
				copy := &compressedData{compressed: append([]byte(nil), compressed.compressed...), originalSize: compressed.originalSize, isCompressed: true, algorithm: algorithm}
				switch variant {
				case "short":
					copy.compressed = copy.compressed[:2]
				case "size small":
					copy.originalSize--
				case "size large":
					copy.originalSize++
				case "negative":
					copy.originalSize = -1
				case "overflow":
					copy.originalSize = math.MaxInt64
				case "unknown":
					copy.algorithm = 99
				}
				if _, err := decompressData(copy); err == nil {
					t.Fatal("corrupt payload accepted")
				}
			})
		}
	}
	if _, err := decompressData(&compressedData{compressed: []byte("x"), originalSize: 2}); err == nil {
		t.Fatal("raw size mismatch accepted")
	}
}

func TestMetadataSeeksAndHeadDoNotDecode(t *testing.T) {
	config := DefaultConfig()
	config.UseARCCache = true
	config.LargeFileCompression = 1
	filesystem, err := NewFromReaderWithConfig(buildTar(t, map[string]string{"file.txt": strings.Repeat("abc", 1000)}), config)
	if err != nil {
		t.Fatal(err)
	}
	f := openTestFile(t, filesystem, "file.txt")
	for _, request := range []struct {
		offset int64
		whence int
		want   int64
	}{{5, io.SeekStart, 5}, {-2, io.SeekCurrent, 3}, {-2, io.SeekEnd, 2998}, {math.MaxInt64, io.SeekStart, math.MaxInt64}, {0, io.SeekCurrent, math.MaxInt64}} {
		if got, err := f.Seek(request.offset, request.whence); err != nil || got != request.want {
			t.Fatalf("seek: %d,%v want %d", got, err, request.want)
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	http.FileServer(filesystem).ServeHTTP(response, httptest.NewRequest(http.MethodHead, "/file.txt", nil))
	if response.Code != http.StatusOK || response.Body.Len() != 0 || response.Header().Get("Content-Length") != "3000" {
		t.Fatalf("HEAD: %v", response)
	}
	if filesystem.(*tarFs).cache.Len() != 0 {
		t.Fatal("metadata access decompressed an asset")
	}
	if _, err := f.Seek(7, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 4)
	if _, err := io.ReadFull(f, got); err != nil || string(got) != "bcab" {
		t.Fatalf("position lost when decoding: %q,%v", got, err)
	}
	if filesystem.(*tarFs).cache.Len() != 1 {
		t.Fatal("decoded payload not cached")
	}
}

func TestArchiveTypesAndMetadata(t *testing.T) {
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeFifo, tar.TypeChar} {
		var archive bytes.Buffer
		w := tar.NewWriter(&archive)
		if err := w.WriteHeader(&tar.Header{Name: "unsupported", Typeflag: kind, Linkname: "target"}); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := NewFromReader(&archive); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("unsupported entry: %v", err)
		}
	}
	var archive bytes.Buffer
	w := tar.NewWriter(&archive)
	stamp := time.Unix(1000, 0)
	for _, header := range []*tar.Header{
		{Name: "./", Typeflag: tar.TypeDir, Mode: 0750, ModTime: stamp},
		{Name: "dir/", Typeflag: tar.TypeDir, Mode: 0700, ModTime: stamp},
		{Name: "dir/file", Size: 1, Mode: 0600},
		{Name: "dir/file", Size: 1, Mode: 0644},
		{Name: "日本語/česky.txt", Size: 1, Mode: 0644},
	} {
		if err := w.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag != tar.TypeDir {
			if _, err := w.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	filesystem, err := NewFromReader(&archive)
	if err != nil {
		t.Fatal(err)
	}
	root, err := openTestFile(t, filesystem, "/").Stat()
	if err != nil {
		t.Fatal(err)
	}
	if root.Mode().Perm() != 0750 || !root.ModTime().Equal(stamp) {
		t.Fatal("root metadata lost")
	}
	dir := openTestFile(t, filesystem, "dir")
	entries, err := dir.Readdir(-1)
	if err != nil || len(entries) != 1 || entries[0].Mode().Perm() != 0644 {
		t.Fatal("duplicate entry did not replace metadata")
	}
	entries[0] = nil
	fresh, err := openTestFile(t, filesystem, "dir").Readdir(-1)
	if err != nil || len(fresh) != 1 || fresh[0] == nil {
		t.Fatal("caller mutated directory index")
	}
	if got, err := io.ReadAll(openTestFile(t, filesystem, "日本語/česky.txt")); err != nil || string(got) != "x" {
		t.Fatal("PAX/Unicode name lost")
	}
}

func TestTruncatedTarFailsAndReaderErrorPropagates(t *testing.T) {
	archive := buildTar(t, map[string]string{"file": strings.Repeat("A", 1024)}).Bytes()
	if _, err := NewFromReader(bytes.NewReader(archive[:600])); err == nil {
		t.Fatal("truncated body accepted")
	}
	sentinel := errors.New("reader failed")
	if _, err := NewFromReader(failedReader{sentinel}); !errors.Is(err, sentinel) {
		t.Fatalf("source error lost: %v", err)
	}
}

type failedReader struct{ err error }

func (r failedReader) Read([]byte) (int, error) { return 0, r.err }

func TestBzip2ContentAndTrailer(t *testing.T) {
	input, err := os.ReadFile("testdata/archive.tar.bz2")
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"valid", "corrupt", "truncated"} {
		t.Run(variant, func(t *testing.T) {
			data := append([]byte(nil), input...)
			switch variant {
			case "corrupt":
				data[len(data)-5] ^= 0x20
			case "truncated":
				data = data[:len(data)-5]
			}
			name := filepath.Join(t.TempDir(), "archive.tbz2")
			if err := os.WriteFile(name, data, 0600); err != nil {
				t.Fatal(err)
			}
			filesystem, err := NewFromBzip2File(name)
			if variant != "valid" {
				if err == nil {
					t.Fatal("invalid bzip2 trailer accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(openTestFile(t, filesystem, "nested/file.txt"))
			if err != nil || string(got) != "bzip fixture\n" {
				t.Fatalf("bzip2 read: %q,%v", got, err)
			}
		})
	}
}

func TestARCConcurrentClearAndResidency(t *testing.T) {
	c := newARCCache(8)
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				key := fmt.Sprint((id + i) % 32)
				c.Put(key, []byte(key))
				c.Get(key)
				if c.Len() > 8 {
					t.Error("capacity exceeded")
				}
				if i%113 == 0 {
					c.Clear()
				}
			}
		}(worker)
	}
	wg.Wait()
	checkARC(t, c)
	c.Clear()
	if c.Len() != 0 || len(c.ghosts) != 0 || len(c.inT2) != 0 || len(c.ghostList) != 0 || c.p != 0 {
		t.Fatal("clear retained cache state")
	}
}

func FuzzArchive(f *testing.F) {
	f.Add([]byte{})
	f.Add(buildTar(f, map[string]string{"dir/a": "abc", "file": "xyz"}).Bytes())
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 128*1024 {
			t.Skip()
		}
		filesystem, err := NewFromReader(bytes.NewReader(data))
		if err != nil {
			return
		}
		var visit func(string)
		visit = func(name string) {
			handle := openTestFile(t, filesystem, name)
			info, err := handle.Stat()
			if err != nil {
				t.Fatal(err)
			}
			if !info.IsDir() {
				content, err := io.ReadAll(handle)
				if err != nil || int64(len(content)) != info.Size() {
					t.Fatalf("payload: %v", err)
				}
				return
			}
			entries, err := handle.Readdir(-1)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				child := strings.TrimSuffix(name, "/") + "/" + entry.Name()
				visit(child)
			}
		}
		visit("/")
	})
}

func FuzzCompressionRoundTrip(f *testing.F) {
	f.Add([]byte("compress me"), uint8(0), int8(5))
	f.Add([]byte{}, uint8(1), int8(0))
	f.Fuzz(func(t *testing.T, data []byte, algorithm uint8, level int8) {
		if len(data) > 64*1024 {
			t.Skip()
		}
		selected := CompressionAlgorithm(1 + algorithm%3)
		compressionLevel := (int(level)+128)%12 + flate.HuffmanOnly
		compressed, err := compressData(data, selected, compressionLevel)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decompressData(compressed)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("roundtrip: %v", err)
		}
	})
}

func TestSharedHandleSerializesDirectoryAndFileCursors(t *testing.T) {
	files := make(map[string]string)
	for i := 0; i < 100; i++ {
		files[fmt.Sprintf("file%03d", i)] = "x"
	}
	files["payload"] = strings.Repeat("x", 100)
	filesystem, err := NewFromReader(buildTar(t, files))
	if err != nil {
		t.Fatal(err)
	}
	directory := openTestFile(t, filesystem, "/")
	payload := openTestFile(t, filesystem, "payload")
	names := make(chan string, 101)
	lengths := make(chan int, 101)
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for {
				entries, err := directory.Readdir(1)
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Error(err)
					return
				}
				names <- entries[0].Name()
			}
			var one [1]byte
			for {
				n, err := payload.Read(one[:])
				if n > 0 {
					lengths <- n
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	group.Wait()
	close(names)
	close(lengths)
	seen := map[string]bool{}
	for name := range names {
		if seen[name] {
			t.Errorf("entry delivered twice: %s", name)
		}
		seen[name] = true
	}
	if len(seen) != 101 {
		t.Errorf("directory delivered %d entries", len(seen))
	}
	total := 0
	for n := range lengths {
		total += n
	}
	if total != 100 {
		t.Errorf("file delivered %d bytes", total)
	}
}

func TestDeepDirectoryIndex(t *testing.T) {
	name := strings.Repeat("a/", 512) + "file"
	filesystem, err := NewFromReader(buildTar(t, map[string]string{name: "deep"}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(openTestFile(t, filesystem, name))
	if err != nil || string(got) != "deep" {
		t.Fatalf("deep file: %q,%v", got, err)
	}
}
