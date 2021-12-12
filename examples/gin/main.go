package main

import (
	"net/http"

	"github.com/digitaldata-cz/tarfs"
	"github.com/gin-gonic/gin"
)

func main() {

	// load web archive
	web, err := tarfs.NewFromBzip2File("web.tbz2")
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

	r.Run() // listen and serve on
}
