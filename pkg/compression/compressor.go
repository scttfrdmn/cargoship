// Package compression provides advanced compression algorithms for CargoShip
package compression

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/klauspost/compress/s2"
	"github.com/pierrec/lz4/v4"
)

// Algorithm represents a compression algorithm
type Algorithm string

const (
	AlgorithmNone Algorithm = "none"
	AlgorithmGzip Algorithm = "gzip"
	AlgorithmZlib Algorithm = "zlib"
	AlgorithmZstd Algorithm = "zstd"
	AlgorithmLZ4  Algorithm = "lz4"
	AlgorithmS2   Algorithm = "s2"
)

// Level represents compression level
type Level int

const (
	LevelFastest Level = 1
	LevelFast    Level = 3
	LevelDefault Level = 5
	LevelBetter  Level = 7
	LevelBest    Level = 9
)

// Compressor provides compression and decompression functionality
type Compressor struct {
	algorithm Algorithm
	level     Level
	blockSize int64

	// Reusable pools for better performance
	gzipWriterPool *sync.Pool
	gzipReaderPool *sync.Pool
	zlibWriterPool *sync.Pool
	zlibReaderPool *sync.Pool
	zstdEncoder    *zstd.Encoder
	zstdDecoder    *zstd.Decoder
	s2WriterPool   *sync.Pool
	s2ReaderPool   *sync.Pool
	lz4WriterPool  *sync.Pool
	lz4ReaderPool  *sync.Pool
}

// CompressionResult contains compression statistics
type CompressionResult struct {
	Algorithm       Algorithm `json:"algorithm"`
	Level           Level     `json:"level"`
	OriginalSize    int64     `json:"original_size"`
	CompressedSize  int64     `json:"compressed_size"`
	CompressionRatio float64  `json:"compression_ratio"`
	CompressionTime  int64    `json:"compression_time_ms"`
	Throughput      float64   `json:"throughput_mbps"`
}

// NewCompressor creates a new compressor with the specified algorithm and level
func NewCompressor(algorithm Algorithm, level Level) (*Compressor, error) {
	c := &Compressor{
		algorithm: algorithm,
		level:     level,
		blockSize: 64 * 1024, // 64KB default block size
	}

	// Initialize pools based on algorithm
	switch algorithm {
	case AlgorithmGzip:
		c.initGzipPools()
	case AlgorithmZlib:
		c.initZlibPools()
	case AlgorithmZstd:
		if err := c.initZstdCodec(); err != nil {
			return nil, fmt.Errorf("failed to initialize zstd codec: %w", err)
		}
	case AlgorithmS2:
		c.initS2Pools()
	case AlgorithmLZ4:
		c.initLZ4Pools()
	case AlgorithmNone:
		// No initialization needed
	default:
		return nil, fmt.Errorf("unsupported compression algorithm: %s", algorithm)
	}

	return c, nil
}

// Compress compresses data using the configured algorithm
func (c *Compressor) Compress(data io.Reader) (io.Reader, *CompressionResult, error) {
	startTime := time.Now()
	
	var buf bytes.Buffer
	var originalSize int64
	var err error

	switch c.algorithm {
	case AlgorithmNone:
		originalSize, err = io.Copy(&buf, data)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to copy data: %w", err)
		}
	case AlgorithmGzip:
		originalSize, err = c.compressGzip(data, &buf)
	case AlgorithmZlib:
		originalSize, err = c.compressZlib(data, &buf)
	case AlgorithmZstd:
		originalSize, err = c.compressZstd(data, &buf)
	case AlgorithmS2:
		originalSize, err = c.compressS2(data, &buf)
	case AlgorithmLZ4:
		originalSize, err = c.compressLZ4(data, &buf)
	default:
		return nil, nil, fmt.Errorf("unsupported compression algorithm: %s", c.algorithm)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("compression failed: %w", err)
	}

	compressionTime := time.Since(startTime)
	compressedSize := int64(buf.Len())
	
	result := &CompressionResult{
		Algorithm:        c.algorithm,
		Level:           c.level,
		OriginalSize:    originalSize,
		CompressedSize:  compressedSize,
		CompressionRatio: float64(originalSize) / float64(compressedSize),
		CompressionTime:  compressionTime.Milliseconds(),
		Throughput:      float64(originalSize) / (1024 * 1024) / compressionTime.Seconds(),
	}

	return bytes.NewReader(buf.Bytes()), result, nil
}

// Decompress decompresses data using the configured algorithm
func (c *Compressor) Decompress(data io.Reader) (io.Reader, error) {
	var buf bytes.Buffer
	var err error

	switch c.algorithm {
	case AlgorithmNone:
		_, err = io.Copy(&buf, data)
	case AlgorithmGzip:
		err = c.decompressGzip(data, &buf)
	case AlgorithmZlib:
		err = c.decompressZlib(data, &buf)
	case AlgorithmZstd:
		err = c.decompressZstd(data, &buf)
	case AlgorithmS2:
		err = c.decompressS2(data, &buf)
	case AlgorithmLZ4:
		err = c.decompressLZ4(data, &buf)
	default:
		return nil, fmt.Errorf("unsupported compression algorithm: %s", c.algorithm)
	}

	if err != nil {
		return nil, fmt.Errorf("decompression failed: %w", err)
	}

	return bytes.NewReader(buf.Bytes()), nil
}

// GetSupportedAlgorithms returns a list of supported compression algorithms
func GetSupportedAlgorithms() []Algorithm {
	return []Algorithm{
		AlgorithmNone,
		AlgorithmGzip,
		AlgorithmZlib,
		AlgorithmZstd,
		AlgorithmLZ4,
		AlgorithmS2,
	}
}

// BenchmarkCompression benchmarks different compression algorithms on sample data
func BenchmarkCompression(data io.Reader, dataSize int64) ([]CompressionResult, error) {
	// Read data into memory for benchmarking
	dataBytes, err := io.ReadAll(data)
	if err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}

	algorithms := []Algorithm{AlgorithmGzip, AlgorithmZlib, AlgorithmZstd, AlgorithmLZ4, AlgorithmS2}
	levels := []Level{LevelFast, LevelDefault, LevelBest}
	
	var results []CompressionResult

	for _, alg := range algorithms {
		for _, level := range levels {
			// Skip levels that don't make sense for certain algorithms
			if alg == AlgorithmS2 && level != LevelDefault {
				continue // S2 doesn't have configurable levels in our implementation
			}

			compressor, err := NewCompressor(alg, level)
			if err != nil {
				continue // Skip unsupported combinations
			}

			dataReader := bytes.NewReader(dataBytes)
			_, result, err := compressor.Compress(dataReader)
			if err != nil {
				continue // Skip failed compressions
			}

			results = append(results, *result)
		}
	}

	return results, nil
}

// RecommendAlgorithm recommends the best compression algorithm based on data characteristics
func RecommendAlgorithm(dataType string, priority string) Algorithm {
	switch priority {
	case "speed":
		return AlgorithmLZ4
	case "size":
		return AlgorithmZstd
	case "balanced":
		return AlgorithmS2
	default:
		// Recommend based on data type
		switch dataType {
		case "text", "log", "json", "xml", "csv":
			return AlgorithmZstd // Best compression for text
		case "image", "video", "audio":
			return AlgorithmNone // Already compressed
		case "binary", "executable":
			return AlgorithmS2 // Good balance for binary data
		case "database", "backup":
			return AlgorithmZstd // Maximum compression for archives
		default:
			return AlgorithmZstd // Safe default
		}
	}
}

// initGzipPools initializes gzip writer and reader pools
func (c *Compressor) initGzipPools() {
	c.gzipWriterPool = &sync.Pool{
		New: func() interface{} {
			w, _ := gzip.NewWriterLevel(nil, int(c.level))
			return w
		},
	}

	c.gzipReaderPool = &sync.Pool{
		New: func() interface{} {
			r, _ := gzip.NewReader(nil)
			return r
		},
	}
}

// initZlibPools initializes zlib writer and reader pools
func (c *Compressor) initZlibPools() {
	c.zlibWriterPool = &sync.Pool{
		New: func() interface{} {
			w, _ := zlib.NewWriterLevel(nil, int(c.level))
			return w
		},
	}

	c.zlibReaderPool = &sync.Pool{
		New: func() interface{} {
			r, _ := zlib.NewReader(nil)
			return r
		},
	}
}

// initZstdCodec initializes zstd encoder and decoder
func (c *Compressor) initZstdCodec() error {
	// Convert our level to zstd level
	var zstdLevel zstd.EncoderLevel
	switch c.level {
	case LevelFastest:
		zstdLevel = zstd.SpeedFastest
	case LevelFast:
		zstdLevel = zstd.SpeedDefault
	case LevelDefault:
		zstdLevel = zstd.SpeedDefault
	case LevelBetter:
		zstdLevel = zstd.SpeedBetterCompression
	case LevelBest:
		zstdLevel = zstd.SpeedBestCompression
	default:
		zstdLevel = zstd.SpeedDefault
	}

	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstdLevel))
	if err != nil {
		return fmt.Errorf("failed to create zstd encoder: %w", err)
	}
	c.zstdEncoder = encoder

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return fmt.Errorf("failed to create zstd decoder: %w", err)
	}
	c.zstdDecoder = decoder

	return nil
}

// initS2Pools initializes S2 writer and reader pools
func (c *Compressor) initS2Pools() {
	c.s2WriterPool = &sync.Pool{
		New: func() interface{} {
			return s2.NewWriter(nil)
		},
	}

	c.s2ReaderPool = &sync.Pool{
		New: func() interface{} {
			return s2.NewReader(nil)
		},
	}
}

// initLZ4Pools initializes LZ4 writer and reader pools
func (c *Compressor) initLZ4Pools() {
	c.lz4WriterPool = &sync.Pool{
		New: func() interface{} {
			return lz4.NewWriter(nil)
		},
	}

	c.lz4ReaderPool = &sync.Pool{
		New: func() interface{} {
			return lz4.NewReader(nil)
		},
	}
}

// Compression implementation methods
func (c *Compressor) compressGzip(src io.Reader, dst io.Writer) (int64, error) {
	w := c.gzipWriterPool.Get().(*gzip.Writer)
	defer c.gzipWriterPool.Put(w)

	w.Reset(dst)
	defer w.Close()

	written, err := io.Copy(w, src)
	if err != nil {
		return 0, err
	}

	return written, w.Close()
}

func (c *Compressor) decompressGzip(src io.Reader, dst io.Writer) error {
	r := c.gzipReaderPool.Get().(*gzip.Reader)
	defer c.gzipReaderPool.Put(r)

	if err := r.Reset(src); err != nil {
		return err
	}
	defer r.Close()

	_, err := io.Copy(dst, r)
	return err
}

func (c *Compressor) compressZlib(src io.Reader, dst io.Writer) (int64, error) {
	w := c.zlibWriterPool.Get().(*zlib.Writer)
	defer c.zlibWriterPool.Put(w)

	w.Reset(dst)
	defer w.Close()

	written, err := io.Copy(w, src)
	if err != nil {
		return 0, err
	}

	return written, w.Close()
}

func (c *Compressor) decompressZlib(src io.Reader, dst io.Writer) error {
	r := c.zlibReaderPool.Get().(io.ReadCloser)
	defer c.zlibReaderPool.Put(r)

	if resetter, ok := r.(interface{ Reset(io.Reader, []byte) error }); ok {
		if err := resetter.Reset(src, nil); err != nil {
			return err
		}
	}
	defer r.Close()

	_, err := io.Copy(dst, r)
	return err
}

func (c *Compressor) compressZstd(src io.Reader, dst io.Writer) (int64, error) {
	c.zstdEncoder.Reset(dst)

	written, err := io.Copy(c.zstdEncoder, src)
	if err != nil {
		return 0, err
	}

	return written, c.zstdEncoder.Close()
}

func (c *Compressor) decompressZstd(src io.Reader, dst io.Writer) error {
	if err := c.zstdDecoder.Reset(src); err != nil {
		return err
	}

	_, err := io.Copy(dst, c.zstdDecoder)
	return err
}

func (c *Compressor) compressS2(src io.Reader, dst io.Writer) (int64, error) {
	w := c.s2WriterPool.Get().(*s2.Writer)
	defer c.s2WriterPool.Put(w)

	w.Reset(dst)
	defer w.Close()

	written, err := io.Copy(w, src)
	if err != nil {
		return 0, err
	}

	return written, w.Close()
}

func (c *Compressor) decompressS2(src io.Reader, dst io.Writer) error {
	r := c.s2ReaderPool.Get().(*s2.Reader)
	defer c.s2ReaderPool.Put(r)

	r.Reset(src)

	_, err := io.Copy(dst, r)
	return err
}

func (c *Compressor) compressLZ4(src io.Reader, dst io.Writer) (int64, error) {
	w := c.lz4WriterPool.Get().(*lz4.Writer)
	defer c.lz4WriterPool.Put(w)

	w.Reset(dst)
	defer w.Close()

	written, err := io.Copy(w, src)
	if err != nil {
		return 0, err
	}

	return written, w.Close()
}

func (c *Compressor) decompressLZ4(src io.Reader, dst io.Writer) error {
	r := c.lz4ReaderPool.Get().(*lz4.Reader)
	defer c.lz4ReaderPool.Put(r)

	r.Reset(src)

	_, err := io.Copy(dst, r)
	return err
}