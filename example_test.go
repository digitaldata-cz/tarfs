package tarfs_test

import (
	"fmt"
	"log"
	"net/http"

	"github.com/digitaldata-cz/tarfs"
)

// Example demonstrates basic usage of TarFS with HTTP server
func Example() {
	// Load web archive
	fs, err := tarfs.NewFromBzip2File("web.tbz2")
	if err != nil {
		log.Fatal(err)
	}

	// Check if file exists
	if fs.Exists("/index.html") {
		fmt.Println("index.html found")
	}

	// Serve files via HTTP
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(fs)))
}

// ExampleNewFromFile demonstrates loading from uncompressed TAR
func ExampleNewFromFile() {
	fs, err := tarfs.NewFromFile("archive.tar")
	if err != nil {
		log.Fatal(err)
	}

	// Open and read a file
	f, err := fs.Open("/readme.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// Read file contents
	buf := make([]byte, 1024)
	n, err := f.Read(buf)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Read %d bytes\n", n)
}

// ExampleNewFromGzipFile demonstrates loading from compressed TAR
func ExampleNewFromGzipFile() {
	fs, err := tarfs.NewFromGzipFile("archive.tar.gz")
	if err != nil {
		log.Fatal(err)
	}

	// List directory contents
	dir, err := fs.Open("/")
	if err != nil {
		log.Fatal(err)
	}
	defer dir.Close()

	files, err := dir.Readdir(-1) // -1 means read all
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		fmt.Printf("%s (%d bytes)\n", file.Name(), file.Size())
	}
}

// ExampleFileSystem_Exists demonstrates checking file existence
func ExampleFileSystem_Exists() {
	fs, err := tarfs.NewFromFile("archive.tar")
	if err != nil {
		log.Fatal(err)
	}

	// Check multiple files
	files := []string{"/index.html", "/style.css", "/notfound.txt"}
	for _, file := range files {
		if fs.Exists(file) {
			fmt.Printf("%s exists\n", file)
		} else {
			fmt.Printf("%s not found\n", file)
		}
	}
}
