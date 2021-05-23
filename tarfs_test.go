package tarfs

import (
	"archive/tar"
	"bytes"
	"io/ioutil"
	"log"
	"os"
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
		hdr := &tar.Header{
			Name: file.Name,
			Size: int64(len(file.Body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			log.Fatalln(err.Error())
		}
		if _, err := tw.Write([]byte(file.Body)); err != nil {
			log.Fatalln(err.Error())
		}
	}
	if err := tw.Close(); err != nil {
		log.Fatalln(err)
	}
	f, err := ioutil.TempFile("", "tarfs_")
	if err != nil {
		log.Fatalln(err.Error())
	}
	_, err = f.Write(buf.Bytes())
	if err != nil {
		log.Fatalln(err.Error())
	}
	err = f.Close()
	if err != nil {
		log.Fatalln(err.Error())
	}
	defer func() {
		err = os.Remove(f.Name())
		if err != nil {
			log.Fatalln(err.Error())
		}
	}()

	// Open the tar archive for reading.
	fs, err := NewFromFile(f.Name())
	if err != nil {
		t.Fatal(err.Error())
	}

	// Test files in archive
	for _, file := range files {
		for _, path := range file.AccessNames {
			f, err := fs.Open(path)
			if err != nil {
				t.Fatal(err.Error())
			}
			content, _ := ioutil.ReadAll(f)
			if string(content) != file.Body {
				t.Fatalf("For '%s'\nExpected:\n%s\nGot:\n%s\n", file.Name, file.Body, content)
			}

			var (
				s, _ = f.Stat()
				size = int64(len(file.Body))
				got  = s.Size()
			)

			if size != got {
				t.Fatalf("For '%s'\nExpected Size:\n%v\nGot:\n%v\n", file.Name, size, got)
			}
		}
	}
}
