[![GoDoc](https://godoc.org/github.com/digitaldata-cz/tarfs?status.svg)](https://godoc.org/github.com/digitaldata-cz/tarfs)
[![Go](https://github.com/digitaldata-cz/tarfs/actions/workflows/go.yml/badge.svg)](https://github.com/digitaldata-cz/tarfs/actions/workflows/go.yml)
[![CodeQL](https://github.com/digitaldata-cz/tarfs/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/digitaldata-cz/tarfs/actions/workflows/codeql-analysis.yml)

# tarfs

In-memory http.FileSystem from tar archives.

## Usage with Gin framework

```go
package main

import (
  "github.com/digitaldata-cz/tarfs"
  "github.com/gin-gonic/gin"
)

func main() {
  r := gin.Default()

  web, err := tarfs.NewFromBzip2File("web.tbz2")
  if err != nil {
    panic(err)
  }
  r.StaticFS("/", web)

  r.Run() // listen and serve on
}
```

## LICENSE

  [MIT](LICENSE)
