package tarfs

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"sync"
)

type compressedData struct {
	decodeMu     sync.Mutex // Coalesces concurrent cache misses for this payload.
	compressed   []byte
	originalSize int64
	isCompressed bool
	algorithm    CompressionAlgorithm
}

func compressData(data []byte, algorithm CompressionAlgorithm, level int) (*compressedData, error) {
	if err := validateCompression(algorithm, level); err != nil {
		return nil, err
	}
	result := &compressedData{compressed: data, originalSize: int64(len(data)), algorithm: algorithm}
	if len(data) == 0 || algorithm == CompressionNone {
		return result, nil
	}
	var buffer bytes.Buffer
	var writer io.WriteCloser
	var err error
	switch algorithm {
	case CompressionDeflate:
		writer, err = flate.NewWriter(&buffer, level)
	case CompressionGzip:
		writer, err = gzip.NewWriterLevel(&buffer, level)
	case CompressionZlib:
		writer, err = zlib.NewWriterLevel(&buffer, level)
	}
	if err != nil {
		return nil, err
	}
	if _, err = writer.Write(data); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	// Compression is a storage optimization, not a reason to enlarge an asset.
	if buffer.Len() < len(data) {
		result.compressed = buffer.Bytes()
		result.isCompressed = true
	}
	return result, nil
}

func decompressData(data *compressedData) ([]byte, error) {
	if data == nil {
		return nil, fmt.Errorf("tarfs: missing compressed payload")
	}
	if !data.isCompressed {
		if int64(len(data.compressed)) != data.originalSize {
			return nil, fmt.Errorf("tarfs: payload size mismatch")
		}
		return data.compressed, nil
	}
	if data.originalSize < 0 || uint64(data.originalSize) >= uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("tarfs: invalid decoded size")
	}
	var reader io.ReadCloser
	var err error
	switch data.algorithm {
	case CompressionDeflate:
		reader = flate.NewReader(bytes.NewReader(data.compressed))
	case CompressionGzip:
		reader, err = gzip.NewReader(bytes.NewReader(data.compressed))
	case CompressionZlib:
		reader, err = zlib.NewReader(bytes.NewReader(data.compressed))
	default:
		return nil, fmt.Errorf("tarfs: unknown compression algorithm %d", data.algorithm)
	}
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	// originalSize is the length of bytes already loaded and compressed by
	// this package. Allocate once; growing ReadAll buffers copied every asset.
	decoded := make([]byte, int(data.originalSize))
	if _, err := io.ReadFull(reader, decoded); err != nil {
		return nil, err
	}
	var extra [1]byte
	if n, err := io.ReadFull(reader, extra[:]); n != 0 {
		return nil, fmt.Errorf("tarfs: decoded size exceeds %d", data.originalSize)
	} else if err != io.EOF {
		return nil, err
	}
	return decoded, nil
}

func shouldCompress(size, threshold int64) bool { return threshold > 0 && size >= threshold }

func validateCompression(algorithm CompressionAlgorithm, level int) error {
	if algorithm < CompressionNone || algorithm > CompressionZlib {
		return fmt.Errorf("tarfs: unknown compression algorithm %d", algorithm)
	}
	if algorithm != CompressionNone && (level < flate.HuffmanOnly || level > flate.BestCompression) {
		return fmt.Errorf("tarfs: invalid compression level %d", level)
	}
	return nil
}
