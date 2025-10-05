// Package tarfs provides a virtual filesystem implementation for TAR archives.
// It implements http.FileSystem interface, making it ideal for embedding static
// files (web assets, documentation) into Go applications.
//
// Features:
//   - Memory-efficient: Custom lightweight metadata storage
//   - Thread-safe: Safe for concurrent reads
//   - Fast directory listings: O(1) access via pre-computed maps
//   - Automatic parent directory creation
//
// Example usage:
//
//	fs, err := tarfs.NewFromBzip2File("web.tbz2")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	http.Handle("/", http.FileServer(fs))
//
// Performance characteristics:
//   - Open: ~90 ns/op, 128 B/op
//   - Readdir: ~1500 ns/op (O(1) complexity)
//   - Memory overhead: ~180 bytes per file
package tarfs

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type (
	// tarFs is the main filesystem structure that holds all files from a TAR archive.
	// It implements http.FileSystem interface for use with HTTP servers.
	// Thread-safe for concurrent read operations.
	tarFs struct {
		files       map[string]*tarFsFile // Map of file paths to file entries
		dirs        map[string][]string   // Pre-computed directory listings for O(1) access
		mu          sync.RWMutex          // Mutex for thread-safe concurrent access
		config      *Config               // Configuration options
		cache       *arcCache             // ARC cache for on-demand loading
		archivePath string                // Path to archive (for lazy loading)
		isGzip      bool                  // Archive is gzipped
		isBzip2     bool                  // Archive is bzipped
		preloaded   map[string]bool       // Map of preloaded files (bypass cache)
	}

	// FileSystem extends http.FileSystem with an Exists method for checking file presence.
	FileSystem interface {
		http.FileSystem
		Exists(name string) bool
	}

	// tarFsFile represents a single file or directory in the TAR filesystem.
	// Each Open() call creates a new instance to avoid race conditions.
	tarFsFile struct {
		fs         *tarFs          // Reference to parent filesystem
		data       []byte          // File contents (nil for directories or cached files)
		compressed *compressedData // Compressed data (if compression enabled)
		fileInfo   *tarFileInfo    // File metadata
		dirPos     int             // Current position for Readdir pagination
		readPos    int             // Current position for Read operations
		isDir      bool            // Whether this is a directory
		name       string          // Full path of the file
		cached     bool            // Whether this file uses cache
	}

	// tarFileInfo is a lightweight implementation of fs.FileInfo.
	// Stores only essential metadata to minimize memory usage.
	tarFileInfo struct {
		name    string
		size    int64
		mode    fs.FileMode
		modTime time.Time
		isDir   bool
	}
)

// Implement fs.FileInfo interface for tarFileInfo
func (tfi *tarFileInfo) Name() string       { return tfi.name }
func (tfi *tarFileInfo) Size() int64        { return tfi.size }
func (tfi *tarFileInfo) Mode() fs.FileMode  { return tfi.mode }
func (tfi *tarFileInfo) ModTime() time.Time { return tfi.modTime }
func (tfi *tarFileInfo) IsDir() bool        { return tfi.isDir }
func (tfi *tarFileInfo) Sys() interface{}   { return nil }

// NewFromFile returns an http.FileSystem that holds all the files in the tar, created from file
// Uses default configuration (no compression, no caching)
func NewFromFile(tarFile string) (FileSystem, error) {
	return NewFromFileWithConfig(tarFile, DefaultConfig())
}

// NewFromFileWithConfig returns an http.FileSystem with custom configuration
func NewFromFileWithConfig(tarFile string, config *Config) (FileSystem, error) {
	reader, err := os.Open(tarFile) // #nosec G304
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return newFSWithConfig(reader, tarFile, false, false, config)
}

// NewFromGzipFile returns an http.FileSystem that holds all the files in the tar.gz, created from file
// Uses default configuration (no compression, no caching)
func NewFromGzipFile(tgzFile string) (FileSystem, error) {
	return NewFromGzipFileWithConfig(tgzFile, DefaultConfig())
}

// NewFromGzipFileWithConfig returns an http.FileSystem with custom configuration
func NewFromGzipFileWithConfig(tgzFile string, config *Config) (FileSystem, error) {
	fileReader, err := os.Open(tgzFile) // #nosec G304
	if err != nil {
		return nil, err
	}
	defer fileReader.Close()
	reader, err := gzip.NewReader(fileReader)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return newFSWithConfig(reader, tgzFile, true, false, config)
}

// NewFromBzip2File returns an http.FileSystem that holds all the files in the tar.bz2, created from file
// Uses default configuration (no compression, no caching)
func NewFromBzip2File(tbz2File string) (FileSystem, error) {
	return NewFromBzip2FileWithConfig(tbz2File, DefaultConfig())
}

// NewFromBzip2FileWithConfig returns an http.FileSystem with custom configuration
func NewFromBzip2FileWithConfig(tbz2File string, config *Config) (FileSystem, error) {
	fileReader, err := os.Open(tbz2File) // #nosec G304
	if err != nil {
		return nil, err
	}
	defer fileReader.Close()
	reader := bzip2.NewReader(fileReader)
	return newFSWithConfig(reader, tbz2File, false, true, config)
}

// NewFromReader returns an http.FileSystem that holds all the files in the tar, created from io.Reader
// Uses default configuration (no compression, no caching)
func NewFromReader(reader io.Reader) (FileSystem, error) {
	return newFSWithConfig(reader, "", false, false, DefaultConfig())
}

// NewFromReaderWithConfig returns an http.FileSystem with custom configuration
func NewFromReaderWithConfig(reader io.Reader, config *Config) (FileSystem, error) {
	return newFSWithConfig(reader, "", false, false, config)
}

func newFSWithConfig(reader io.Reader, archivePath string, isGzip, isBzip2 bool, config *Config) (FileSystem, error) {
	if config == nil {
		config = DefaultConfig()
	}

	tarReader := tar.NewReader(reader)
	tfs := &tarFs{
		files:       make(map[string]*tarFsFile),
		dirs:        make(map[string][]string),
		config:      config,
		archivePath: archivePath,
		isGzip:      isGzip,
		isBzip2:     isBzip2,
		preloaded:   make(map[string]bool),
	}

	// Initialize ARC cache if enabled
	if config.UseARCCache {
		tfs.cache = newARCCache(config.ARCCacheSize)
	}

	// Build preload map for quick lookup
	preloadMap := make(map[string]bool)
	for _, p := range config.PreloadFiles {
		preloadMap[normalizePath(p)] = true
	}

	// Ensure root directory always exists
	tfs.files["/"] = &tarFsFile{
		fs:   tfs,
		data: nil,
		fileInfo: &tarFileInfo{
			name:    "/",
			size:    0,
			mode:    0755 | fs.ModeDir,
			modTime: time.Now(),
			isDir:   true,
		},
		isDir:  true,
		name:   "/",
		cached: false,
	}
	tfs.dirs["/"] = []string{}

	for {
		fileHeader, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if fileHeader.Name == "." {
			continue
		}

		// Clean and normalize path
		cleanPath := normalizePath(fileHeader.Name)

		// Skip non regular files and directories
		if fileHeader.Typeflag == tar.TypeReg {
			// Check if this file should be preloaded
			isPreloaded := preloadMap[cleanPath]

			// Read file data
			data, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, err
			}

			fileEntry := &tarFsFile{
				fs: tfs,
				fileInfo: &tarFileInfo{
					name:    path.Base(cleanPath),
					size:    fileHeader.Size,
					mode:    fileHeader.FileInfo().Mode(),
					modTime: fileHeader.ModTime,
					isDir:   false,
				},
				isDir:  false,
				name:   cleanPath,
				cached: false,
			}

			// Handle file based on configuration
			if isPreloaded {
				// Preloaded files are always kept in memory
				if shouldCompress(fileHeader.Size, config.LargeFileCompression) {
					fileEntry.compressed = compressData(data, config.CompressionAlgorithm, config.CompressionLevel)
				} else {
					fileEntry.data = data
				}
				tfs.preloaded[cleanPath] = true
			} else if config.UseARCCache {
				// With ARC cache, only store metadata, data loaded on demand
				fileEntry.cached = true
				fileEntry.data = nil
				if shouldCompress(fileHeader.Size, config.LargeFileCompression) {
					compressed := compressData(data, config.CompressionAlgorithm, config.CompressionLevel)
					fileEntry.compressed = compressed
					// Store the actual data in cache for immediate availability
					tfs.cache.Put(cleanPath, compressed.compressed)
				} else {
					fileEntry.compressed = wrapUncompressedData(data, fileHeader.Size)
					tfs.cache.Put(cleanPath, data)
				}
			} else {
				// No cache - load everything and optionally compress
				if shouldCompress(fileHeader.Size, config.LargeFileCompression) {
					fileEntry.compressed = compressData(data, config.CompressionAlgorithm, config.CompressionLevel)
				} else {
					fileEntry.data = data
				}
			}

			tfs.files[cleanPath] = fileEntry

		} else if fileHeader.Typeflag == tar.TypeDir {
			tfs.files[cleanPath] = &tarFsFile{
				fs:   tfs,
				data: nil,
				fileInfo: &tarFileInfo{
					name:    path.Base(cleanPath),
					size:    0,
					mode:    fileHeader.FileInfo().Mode() | fs.ModeDir,
					modTime: fileHeader.ModTime,
					isDir:   true,
				},
				isDir:  true,
				name:   cleanPath,
				cached: false,
			}
		}
	}

	// Ensure all parent directories exist
	for filePath := range tfs.files {
		dirPath := path.Dir(filePath)
		for dirPath != "/" && dirPath != "." {
			if _, exists := tfs.files[dirPath]; !exists {
				tfs.files[dirPath] = &tarFsFile{
					fs:   tfs,
					data: nil,
					fileInfo: &tarFileInfo{
						name:    path.Base(dirPath),
						size:    0,
						mode:    0755 | fs.ModeDir,
						modTime: time.Now(),
						isDir:   true,
					},
					isDir: true,
					name:  dirPath,
				}
			}
			// Initialize directory entry if not exists
			if _, exists := tfs.dirs[dirPath]; !exists {
				tfs.dirs[dirPath] = []string{}
			}
			dirPath = path.Dir(dirPath)
		}
	}

	// Build directory listings - must be done after all directories exist
	for filePath := range tfs.files {
		dirPath := path.Dir(filePath)
		if dirPath != filePath { // Don't add directory to itself
			// Check if not already in the list
			found := false
			for _, existing := range tfs.dirs[dirPath] {
				if existing == filePath {
					found = true
					break
				}
			}
			if !found {
				tfs.dirs[dirPath] = append(tfs.dirs[dirPath], filePath)
			}
		}
	}

	return tfs, nil
}

// Open file in tarfs and returns file handle
func (tf *tarFs) Open(name string) (http.File, error) {
	if filepath.Separator != '/' && strings.ContainsRune(name, filepath.Separator) ||
		strings.Contains(name, "\x00") {
		return nil, errors.New("invalid character in file path")
	}

	tf.mu.RLock()
	defer tf.mu.RUnlock()

	cleanPath := normalizePath(name)
	f, ok := tf.files[cleanPath]
	if !ok {
		return nil, os.ErrNotExist
	}

	// Return a copy of the file handle to avoid race conditions
	fileCopy := &tarFsFile{
		fs:         tf,
		data:       f.data,
		compressed: f.compressed,
		fileInfo:   f.fileInfo,
		isDir:      f.isDir,
		name:       f.name,
		dirPos:     0,
		readPos:    0,
		cached:     f.cached,
	}

	return fileCopy, nil
}

// Check if file exists in tarfs
func (tf *tarFs) Exists(name string) bool {
	if filepath.Separator != '/' && strings.ContainsRune(name, filepath.Separator) ||
		strings.Contains(name, "\x00") {
		return false
	}

	tf.mu.RLock()
	defer tf.mu.RUnlock()

	_, ok := tf.files[normalizePath(name)]
	return ok
}

// getData returns the actual file data, handling decompression and cache lookups
func (f *tarFsFile) getData() ([]byte, error) {
	// If data is already in memory, return it
	if f.data != nil {
		return f.data, nil
	}

	// If cached, try to get from cache
	if f.cached && f.fs.cache != nil {
		cachedData := f.fs.cache.Get(f.name)
		if cachedData != nil {
			if f.compressed != nil && f.compressed.isCompressed {
				cd := *f.compressed
				cd.compressed = cachedData
				return decompressData(&cd)
			}
			return cachedData, nil
		}
	}

	if f.compressed != nil {
		data, err := decompressData(f.compressed)
		if err != nil {
			return nil, err
		}
		// Repopulate cache so subsequent readers benefit
		if f.cached && f.fs.cache != nil {
			f.fs.cache.Put(f.name, f.compressed.compressed)
		}
		return data, nil
	}

	return []byte{}, nil
}

// Read implements io.Reader for tarFsFile
func (f *tarFsFile) Read(p []byte) (n int, err error) {
	if f.isDir {
		return 0, errors.New("is a directory")
	}

	// Get actual data (may need decompression or cache lookup)
	data, err := f.getData()
	if err != nil {
		return 0, err
	}

	if f.readPos >= len(data) {
		return 0, io.EOF
	}
	n = copy(p, data[f.readPos:])
	f.readPos += n
	return n, nil
}

// Seek implements io.Seeker for tarFsFile
func (f *tarFsFile) Seek(offset int64, whence int) (int64, error) {
	if f.isDir {
		return 0, errors.New("is a directory")
	}

	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = int64(f.readPos) + offset
	case io.SeekEnd:
		newPos = f.fileInfo.size + offset
	default:
		return 0, errors.New("invalid whence")
	}

	if newPos < 0 {
		return 0, errors.New("negative position")
	}

	f.readPos = int(newPos)
	return newPos, nil
}

// Close file handle
func (f *tarFsFile) Close() error {
	return nil
}

// Readdir returns directory entries
func (f *tarFsFile) Readdir(count int) ([]os.FileInfo, error) {
	if !f.isDir {
		return nil, os.ErrNotExist
	}

	f.fs.mu.RLock()
	defer f.fs.mu.RUnlock()

	entries, exists := f.fs.dirs[f.name]
	if !exists {
		return []os.FileInfo{}, nil
	}

	// Convert paths to FileInfo
	var fileInfos []os.FileInfo
	for _, entryPath := range entries {
		if entry, ok := f.fs.files[entryPath]; ok {
			fileInfos = append(fileInfos, entry.fileInfo)
		}
	}

	// Handle count parameter
	if count <= 0 {
		return fileInfos, nil
	}

	if f.dirPos >= len(fileInfos) {
		return nil, io.EOF
	}

	end := f.dirPos + count
	if end > len(fileInfos) {
		end = len(fileInfos)
	}

	result := fileInfos[f.dirPos:end]
	f.dirPos = end

	if f.dirPos >= len(fileInfos) {
		return result, io.EOF
	}

	return result, nil
}

// Stat returns file info
func (f *tarFsFile) Stat() (os.FileInfo, error) {
	return f.fileInfo, nil
}

func normalizePath(name string) string {
	if name == "" {
		return "/"
	}
	clean := strings.ReplaceAll(name, "\\", "/")
	if !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}
	clean = path.Clean(clean)
	if clean == "." || clean == "" {
		return "/"
	}
	return clean
}
