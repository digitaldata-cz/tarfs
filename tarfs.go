package tarfs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type (
	tarFs struct {
		files map[string]*tarFsFile
	}

	FileSystem interface {
		http.FileSystem
		Exists(name string) bool
	}

	tarFsFile struct {
		*bytes.Reader
		data  []byte
		fi    os.FileInfo
		files []os.FileInfo
	}
)

// NewFromFile returns an http.FileSystem that holds all the files in the tar, created from file
func NewFromFile(tarFile string) (FileSystem, error) {
	reader, err := os.Open(tarFile)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return newFS(reader)
}

// NewFromGzipFile returns an http.FileSystem that holds all the files in the tar.gz, created from file
func NewFromGzipFile(tarFile string) (FileSystem, error) {
	fileReader, err := os.Open(tarFile)
	if err != nil {
		return nil, err
	}
	defer fileReader.Close()
	reader, err := gzip.NewReader(fileReader)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return newFS(reader)
}

// NewFromReader returns an http.FileSystem that holds all the files in the tar, created from io.Reader
func NewFromReader(reader io.Reader) (FileSystem, error) {
	return newFS(reader)
}

func newFS(reader io.Reader) (FileSystem, error) {
	tarReader := tar.NewReader(reader)
	tarFs := tarFs{files: make(map[string]*tarFsFile)}
	for {
		fileHeader, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		data, err := ioutil.ReadAll(tarReader)
		if err != nil {
			return nil, err
		}
		tarFs.files[path.Join("/", fileHeader.Name)] = &tarFsFile{
			data: data,
			fi:   fileHeader.FileInfo(),
		}
	}
	return &tarFs, nil
}

// Open file in tarfs and returns file handle
func (tf *tarFs) Open(name string) (http.File, error) {
	if filepath.Separator != '/' && strings.ContainsRune(name, filepath.Separator) ||
		strings.Contains(name, "\x00") {
		return nil, errors.New("invalid character in file path")
	}
	f, ok := tf.files[path.Join("/", name)]
	if !ok {
		return nil, os.ErrNotExist
	}
	if f.fi.IsDir() {
		f.files = []os.FileInfo{}
		for path, tarFsFile := range tf.files {
			if strings.HasPrefix(path, name) {
				s, _ := tarFsFile.Stat()
				f.files = append(f.files, s)
			}
		}

	}
	f.Reader = bytes.NewReader(f.data)
	return f, nil
}

// Check if file exists in tarfs
func (tf *tarFs) Exists(name string) bool {
	if filepath.Separator != '/' && strings.ContainsRune(name, filepath.Separator) ||
		strings.Contains(name, "\x00") {
		return false
	}
	_, ok := tf.files[path.Join("/", name)]
	return ok
}

// Close file handle
func (f *tarFsFile) Close() error {
	return nil
}

// Readdir
func (f *tarFsFile) Readdir(count int) ([]os.FileInfo, error) {
	if f.fi.IsDir() && f.files != nil {
		return f.files, nil
	}
	return nil, os.ErrNotExist
}

// Stat
func (f *tarFsFile) Stat() (os.FileInfo, error) {
	return f.fi, nil
}
