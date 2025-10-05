package tarfs

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
)

// compressedData holds compressed file data with metadata
type compressedData struct {
	compressed   []byte
	originalSize int64
	isCompressed bool
	algorithm    CompressionAlgorithm // Store algorithm for decompression
}

// compressData compresses data using the specified algorithm and level
func compressData(data []byte, algorithm CompressionAlgorithm, level int) *compressedData {
	if len(data) == 0 {
		return &compressedData{
			compressed:   []byte{},
			originalSize: 0,
			isCompressed: false,
			algorithm:    algorithm,
		}
	}

	var buf bytes.Buffer
	var w io.WriteCloser
	var err error

	// Create appropriate writer based on algorithm
	switch algorithm {
	case CompressionDeflate:
		w, err = flate.NewWriter(&buf, level)
	case CompressionGzip:
		w, err = gzip.NewWriterLevel(&buf, level)
	case CompressionZlib:
		w, err = zlib.NewWriterLevel(&buf, level)
	default:
		// Unknown algorithm, return uncompressed
		return &compressedData{
			compressed:   data,
			originalSize: int64(len(data)),
			isCompressed: false,
			algorithm:    algorithm,
		}
	}

	// Handle errors and write data (common code for all algorithms)
	if err != nil {
		return &compressedData{
			compressed:   data,
			originalSize: int64(len(data)),
			isCompressed: false,
			algorithm:    algorithm,
		}
	}
	defer w.Close()

	if _, err := w.Write(data); err != nil {
		return &compressedData{
			compressed:   data,
			originalSize: int64(len(data)),
			isCompressed: false,
			algorithm:    algorithm,
		}
	}

	if err := w.Close(); err != nil {
		return &compressedData{
			compressed:   data,
			originalSize: int64(len(data)),
			isCompressed: false,
			algorithm:    algorithm,
		}
	}

	compressed := buf.Bytes()

	return &compressedData{
		compressed:   compressed,
		originalSize: int64(len(data)),
		isCompressed: true,
		algorithm:    algorithm,
	}
}

func wrapUncompressedData(data []byte, size int64) *compressedData {
	if data == nil {
		data = []byte{}
	}
	return &compressedData{
		compressed:   data,
		originalSize: size,
		isCompressed: false,
		algorithm:    CompressionNone,
	}
}

// decompressData decompresses data using the stored algorithm
func decompressData(cd *compressedData) ([]byte, error) {
	if !cd.isCompressed {
		return cd.compressed, nil
	}

	if len(cd.compressed) == 0 {
		return []byte{}, nil
	}

	var r io.ReadCloser
	var err error

	// Create appropriate reader based on algorithm
	switch cd.algorithm {
	case CompressionDeflate:
		r = flate.NewReader(bytes.NewReader(cd.compressed))
	case CompressionGzip:
		r, err = gzip.NewReader(bytes.NewReader(cd.compressed))
		if err != nil {
			return nil, err
		}
	case CompressionZlib:
		r, err = zlib.NewReader(bytes.NewReader(cd.compressed))
		if err != nil {
			return nil, err
		}
	default:
		// Unknown algorithm, return compressed data as-is
		return cd.compressed, nil
	}
	defer r.Close()

	decompressed, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return decompressed, nil
}

// shouldCompress determines if a file should be compressed based on size threshold
func shouldCompress(size int64, threshold int64) bool {
	return threshold > 0 && size >= threshold
}
