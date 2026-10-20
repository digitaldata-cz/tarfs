// Package tarfs loads TAR archives into an immutable, in-memory HTTP filesystem.
// A filesystem can be shared between requests. Each Open returns an independent
// handle; operations on a handle are serialized. No files are extracted to disk.
package tarfs

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"time"
)

// FileSystem extends http.FileSystem with a file/directory presence check.
type FileSystem interface {
	http.FileSystem
	Exists(name string) bool
}

// Indexes and payloads are immutable; handle cursors and decoded-cache state
// are synchronized separately.
type tarFs struct {
	files map[string]*fileEntry
	dirs  map[string][]os.FileInfo
	cache *arcCache
}

type fileEntry struct {
	data       []byte
	compressed *compressedData
	fileInfo   *tarFileInfo
}

// Only the handle owns the reader, directory cursor and closed state.
type tarFsFile struct {
	mu     sync.Mutex
	fs     *tarFs
	entry  *fileEntry
	reader bytes.Reader
	name   string
	dirPos int
	loaded bool
	closed bool
}

type tarFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

func (info *tarFileInfo) Name() string       { return info.name }
func (info *tarFileInfo) Size() int64        { return info.size }
func (info *tarFileInfo) Mode() fs.FileMode  { return info.mode }
func (info *tarFileInfo) ModTime() time.Time { return info.modTime }
func (info *tarFileInfo) IsDir() bool        { return info.mode.IsDir() }
func (info *tarFileInfo) Sys() any           { return nil }

// NewFromFile loads an uncompressed TAR archive with the default configuration.
func NewFromFile(name string) (FileSystem, error) {
	return NewFromFileWithConfig(name, nil)
}

// NewFromFileWithConfig loads an uncompressed TAR archive. The file is closed
// before this function returns; subsequent reads never reopen the archive.
func NewFromFileWithConfig(name string, config *Config) (FileSystem, error) {
	file, err := os.Open(name) // #nosec G304 -- caller explicitly selects the archive.
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return newFSWithConfig(file, config)
}

// NewFromGzipFile loads a gzip-compressed TAR archive with the default configuration.
func NewFromGzipFile(name string) (FileSystem, error) {
	return NewFromGzipFileWithConfig(name, nil)
}

// NewFromGzipFileWithConfig loads a gzip-compressed TAR archive and verifies the
// complete gzip stream, including its checksum and trailer.
func NewFromGzipFileWithConfig(name string, config *Config) (FileSystem, error) {
	file, err := os.Open(name) // #nosec G304 -- caller explicitly selects the archive.
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return loadCompressedArchive(reader, config)
}

// NewFromBzip2File loads a bzip2-compressed TAR archive with the default configuration.
func NewFromBzip2File(name string) (FileSystem, error) {
	return NewFromBzip2FileWithConfig(name, nil)
}

// NewFromBzip2FileWithConfig loads and verifies a bzip2-compressed TAR archive.
func NewFromBzip2FileWithConfig(name string, config *Config) (FileSystem, error) {
	file, err := os.Open(name) // #nosec G304 -- caller explicitly selects the archive.
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return loadCompressedArchive(bzip2.NewReader(file), config)
}

// TAR ends before the surrounding compression stream. Reading to its end is
// necessary to detect a truncated trailer or checksum failure.
func loadCompressedArchive(reader io.Reader, config *Config) (FileSystem, error) {
	filesystem, err := newFSWithConfig(reader, config)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return nil, fmt.Errorf("tarfs: finish compressed archive: %w", err)
	}
	return filesystem, nil
}

// NewFromReader loads a TAR archive with the default configuration. It does not
// close reader. The reader is no longer needed after this function returns.
func NewFromReader(reader io.Reader) (FileSystem, error) {
	return NewFromReaderWithConfig(reader, nil)
}

// NewFromReaderWithConfig loads a TAR archive without taking ownership of reader.
// A nil configuration selects DefaultConfig. A zero Config disables compression
// and caching. Invalid settings fail construction, even for an empty archive.
func NewFromReaderWithConfig(reader io.Reader, config *Config) (FileSystem, error) {
	return newFSWithConfig(reader, config)
}

func newFSWithConfig(reader io.Reader, config *Config) (FileSystem, error) {
	if reader == nil {
		return nil, fmt.Errorf("tarfs: nil archive reader: %w", fs.ErrInvalid)
	}
	if config == nil {
		config = DefaultConfig()
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	filesystem := &tarFs{
		files: make(map[string]*fileEntry), dirs: make(map[string][]os.FileInfo),
	}
	if config.UseARCCache {
		filesystem.cache = newARCCache(config.ARCCacheSize)
	}
	preload := make(map[string]bool)
	if config.UseARCCache {
		for _, name := range config.PreloadFiles {
			if strings.ContainsRune(name, '\x00') {
				return nil, fmt.Errorf("tarfs: preload path: %w", fs.ErrInvalid)
			}
			preload[normalizePath(name)] = true
		}
	}
	filesystem.files["/"] = directoryEntry("/", 0755, time.Time{})
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			return nil, fmt.Errorf("tarfs: unsupported entry %q (type %d): %w", header.Name, header.Typeflag, fs.ErrInvalid)
		}
		if strings.ContainsRune(header.Name, '\x00') {
			return nil, fmt.Errorf("tarfs: entry path: %w", fs.ErrInvalid)
		}
		name := normalizePath(header.Name)
		isDir := header.Typeflag == tar.TypeDir
		if old, exists := filesystem.files[name]; exists && old.fileInfo.IsDir() != isDir {
			return nil, fmt.Errorf("tarfs: file/directory conflict at %q: %w", name, fs.ErrInvalid)
		}
		for parent := parentPath(name); ; parent = parentPath(parent) {
			if entry, exists := filesystem.files[parent]; exists {
				if !entry.fileInfo.IsDir() {
					return nil, fmt.Errorf("tarfs: parent %q is a file: %w", parent, fs.ErrInvalid)
				}
			} else {
				filesystem.files[parent] = directoryEntry(parent, 0755, time.Time{})
			}
			if parent == "/" {
				break
			}
		}
		if isDir {
			filesystem.files[name] = directoryEntry(name, header.FileInfo().Mode(), header.ModTime)
			continue
		}
		data, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, fmt.Errorf("tarfs: read %q: %w", name, err)
		}
		entry := &fileEntry{data: data, fileInfo: &tarFileInfo{
			name: path.Base(name), size: header.Size, mode: header.FileInfo().Mode(), modTime: header.ModTime,
		}}
		// Preloads keep decoded bytes in the entry, outside ARC.
		if !preload[name] && config.CompressionAlgorithm != CompressionNone && shouldCompress(header.Size, config.LargeFileCompression) {
			compressed, err := compressData(data, config.CompressionAlgorithm, config.CompressionLevel)
			if err != nil {
				return nil, fmt.Errorf("tarfs: compress %q: %w", name, err)
			}
			if compressed.isCompressed {
				entry.data, entry.compressed = nil, compressed
			}
		}
		// TAR permits repeated entries. The last entry of the same kind wins.
		filesystem.files[name] = entry
	}
	for name := range preload {
		entry, exists := filesystem.files[name]
		if !exists {
			return nil, &fs.PathError{Op: "preload", Path: name, Err: fs.ErrNotExist}
		}
		if entry.fileInfo.IsDir() {
			return nil, &fs.PathError{Op: "preload", Path: name, Err: fs.ErrInvalid}
		}
	}
	// Build once, without an O(n^2) duplicate scan. Consumers get copied slices.
	for name, entry := range filesystem.files {
		if entry.fileInfo.IsDir() {
			filesystem.dirs[name] = nil
		}
	}
	for name, entry := range filesystem.files {
		if name != "/" {
			parent := parentPath(name)
			filesystem.dirs[parent] = append(filesystem.dirs[parent], entry.fileInfo)
		}
	}
	for _, entries := range filesystem.dirs {
		slices.SortFunc(entries, func(a, b os.FileInfo) int { return strings.Compare(a.Name(), b.Name()) })
	}
	return filesystem, nil
}

func directoryEntry(name string, mode fs.FileMode, modTime time.Time) *fileEntry {
	return &fileEntry{fileInfo: &tarFileInfo{name: path.Base(name), mode: mode | fs.ModeDir, modTime: modTime}}
}

// Open returns an independent handle. Paths use a virtual root: leading slashes,
// dot components and Windows backslashes are normalized on every platform.
func (filesystem *tarFs) Open(name string) (http.File, error) {
	if strings.ContainsRune(name, '\x00') {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	clean := normalizePath(name)
	entry, exists := filesystem.files[clean]
	if !exists {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	file := &tarFsFile{fs: filesystem, entry: entry, name: clean}
	if entry.compressed == nil {
		file.reader.Reset(entry.data)
		file.loaded = true
	}
	return file, nil
}

// Exists reports whether a regular file or directory is present.
func (filesystem *tarFs) Exists(name string) bool {
	if strings.ContainsRune(name, '\x00') {
		return false
	}
	_, exists := filesystem.files[normalizePath(name)]
	return exists
}

// prepare decodes at most once per handle. ARC holds decoded bytes across opens;
// the immutable compressed payload remains the filesystem's source of truth.
func (file *tarFsFile) prepare() error {
	if file.loaded {
		return nil
	}
	var data []byte
	if file.fs.cache != nil {
		data = file.fs.cache.Get(file.name)
		if data == nil {
			// Serialize misses for this payload only; unrelated files decode
			// concurrently and cache hits need no payload lock.
			file.entry.compressed.decodeMu.Lock()
			defer file.entry.compressed.decodeMu.Unlock()
			data = file.fs.cache.Get(file.name)
		}
	}
	if data == nil {
		var err error
		data, err = decompressData(file.entry.compressed)
		if err != nil {
			return &fs.PathError{Op: "read", Path: file.name, Err: err}
		}
		if file.fs.cache != nil {
			file.fs.cache.Put(file.name, data)
		}
	}
	position, _ := file.reader.Seek(0, io.SeekCurrent)
	file.reader.Reset(data)
	_, _ = file.reader.Seek(position, io.SeekStart) // position was already validated by Seek.
	file.loaded = true
	return nil
}

func (file *tarFsFile) stateError(op string, directoryOperation bool) error {
	if file.closed {
		return &fs.PathError{Op: op, Path: file.name, Err: fs.ErrClosed}
	}
	if file.entry.fileInfo.IsDir() != directoryOperation {
		return &fs.PathError{Op: op, Path: file.name, Err: fs.ErrInvalid}
	}
	return nil
}

func (file *tarFsFile) Read(data []byte) (int, error) {
	file.mu.Lock()
	defer file.mu.Unlock()
	if err := file.stateError("read", false); err != nil {
		return 0, err
	}
	if len(data) == 0 {
		return 0, nil
	}
	if err := file.prepare(); err != nil {
		return 0, err
	}
	return file.reader.Read(data)
}

func (file *tarFsFile) Seek(offset int64, whence int) (int64, error) {
	file.mu.Lock()
	defer file.mu.Unlock()
	if err := file.stateError("seek", false); err != nil {
		return 0, err
	}
	// Seeking uses metadata and must not decode a payload, including for HEAD requests.
	var base int64
	switch whence {
	case io.SeekStart:
		base = 0
	case io.SeekCurrent:
		base, _ = file.reader.Seek(0, io.SeekCurrent)
	case io.SeekEnd:
		base = file.entry.fileInfo.Size()
	default:
		return 0, &fs.PathError{Op: "seek", Path: file.name, Err: fs.ErrInvalid}
	}
	position := base + offset
	if position < 0 || (offset > 0 && position < base) {
		return 0, &fs.PathError{Op: "seek", Path: file.name, Err: fs.ErrInvalid}
	}
	return file.reader.Seek(position, io.SeekStart)
}

// Close releases this handle's decoded data. Closing twice is harmless; subsequent
// Read, Seek, Readdir and Stat calls return fs.ErrClosed.
func (file *tarFsFile) Close() error {
	file.mu.Lock()
	defer file.mu.Unlock()
	file.closed = true
	file.reader.Reset(nil)
	return nil
}

// Readdir returns the next count entries in lexical name order. For count <= 0,
// it consumes all remaining entries. A positive read returns io.EOF only when no
// entries remain, matching os.File.Readdir.
func (file *tarFsFile) Readdir(count int) ([]os.FileInfo, error) {
	file.mu.Lock()
	defer file.mu.Unlock()
	if err := file.stateError("readdir", true); err != nil {
		return nil, err
	}
	entries := file.fs.dirs[file.name]
	remaining := len(entries) - file.dirPos
	if count > 0 && remaining == 0 {
		return nil, io.EOF
	}
	length := remaining
	if count > 0 && count < length {
		length = count
	}
	result := make([]os.FileInfo, length)
	copy(result, entries[file.dirPos:file.dirPos+length])
	file.dirPos += length
	return result, nil
}

func (file *tarFsFile) Stat() (os.FileInfo, error) {
	file.mu.Lock()
	defer file.mu.Unlock()
	if file.closed {
		return nil, &fs.PathError{Op: "stat", Path: file.name, Err: fs.ErrClosed}
	}
	return file.entry.fileInfo, nil
}

func normalizePath(name string) string {
	if strings.ContainsRune(name, '\\') {
		name = strings.ReplaceAll(name, "\\", "/")
	}
	if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}
	return path.Clean(name)
}

// name is already canonical. path.Dir would clean each successively shorter
// prefix again, making a deeply nested archive quadratic in path depth.
func parentPath(name string) string {
	separator := strings.LastIndexByte(name, '/')
	if separator <= 0 {
		return "/"
	}
	return name[:separator]
}
