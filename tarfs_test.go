package tarfs

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	// Create testing archive
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	var files = []struct {
		Name        string
		Body        string
		AccessNames []string
	}{
		{
			Name: "readme.txt",
			Body: "This archive contains some text files.",
			AccessNames: []string{
				"readme.txt",
				"/readme.txt",
				"./readme.txt",
				"././readme.txt",
				"../readme.txt",
			},
		},
		{
			Name: "/gopher.txt",
			Body: "Gopher names:\nGeorge\nGeoffrey\nGonzo",
			AccessNames: []string{
				"gopher.txt",
				"/gopher.txt",
				"./gopher.txt",
				"././gopher.txt",
				"../gopher.txt",
			},
		},
		{
			Name: "./todo.txt",
			Body: "Get animal handling licence.",
			AccessNames: []string{
				"todo.txt",
				"/todo.txt",
				"./todo.txt",
				"././todo.txt",
				"../todo.txt",
			},
		},
	}

	for _, file := range files {
		if err := tw.WriteHeader(&tar.Header{Name: file.Name, Size: int64(len(file.Body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(file.Body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "archive.tar")
	if err := os.WriteFile(archivePath, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	filesystem, err := NewFromFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		for _, name := range file.AccessNames {
			handle := openTestFile(t, filesystem, name)
			content, err := io.ReadAll(handle)
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != file.Body {
				t.Fatalf("%q: content %q, want %q", name, content, file.Body)
			}
			info, err := handle.Stat()
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() != int64(len(file.Body)) {
				t.Fatalf("%q: size %d, want %d", name, info.Size(), len(file.Body))
			}
			if err := handle.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}
