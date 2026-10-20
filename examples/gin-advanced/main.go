package main

import (
	"log"
	"net/http"

	"github.com/digitaldata-cz/tarfs/v2"
	"github.com/gin-gonic/gin"
)

func main() {
	// Configure TarFS with compression and ARC cache
	config := &tarfs.Config{
		// Compress files larger than 10KB in memory
		// Saves memory for large HTML/JS/CSS files
		LargeFileCompression: 10 * 1024, // 10KB threshold
		CompressionAlgorithm: tarfs.CompressionDeflate,
		CompressionLevel:     5,

		// Enable ARC cache for efficient memory usage
		// Retain decoded compressed files across opens
		UseARCCache: true,

		// Cache up to 100 files
		ARCCacheSize: 100,

		// Preload critical files (always in memory, bypass cache)
		PreloadFiles: []string{
			"index.html",
			"style.css",
			"hello-world.png",
		},
	}

	// Load web archive with configuration
	web, err := tarfs.NewFromBzip2FileWithConfig("web.tbz2", config)
	if err != nil {
		panic(err)
	}

	r := gin.Default()

	// If there is no defined route, try to serve static file
	r.NoRoute(func(c *gin.Context) {
		http.FileServer(web).ServeHTTP(c.Writer, c.Request)
	})

	// Example api call
	r.GET("/ping", func(c *gin.Context) {
		c.String(200, "Pong!")
	})

	// Stats endpoint to show configuration
	r.GET("/stats", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"compression_enabled":   config.LargeFileCompression > 0,
			"compression_threshold": config.LargeFileCompression,
			"arc_cache_enabled":     config.UseARCCache,
			"arc_cache_size":        config.ARCCacheSize,
			"preloaded_files":       config.PreloadFiles,
		})
	})

	log.Println("Server starting on :8080")
	log.Printf("Configuration:")
	log.Printf("  - Compression threshold: %d bytes", config.LargeFileCompression)
	log.Printf("  - ARC cache: %v (size: %d)", config.UseARCCache, config.ARCCacheSize)
	log.Printf("  - Preloaded files: %v", config.PreloadFiles)

	err = r.Run() // listen and serve on :8080
	if err != nil {
		panic(err)
	}
}
