package phlaredb

import (
	"fmt"
	"os"
	"testing"
	"time"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress"
	"github.com/parquet-go/parquet-go/compress/gzip"
	"github.com/parquet-go/parquet-go/compress/lz4"
	"github.com/parquet-go/parquet-go/compress/snappy"
	"github.com/parquet-go/parquet-go/compress/zstd"
	"github.com/stretchr/testify/require"
)

var (
	testLocationFile = "testdata/01HHYG6245NWHZWVP27V8WJRT7/symbols/locations.parquet"
)

// TestLocationCompressionFunctionality tests the correctness of compression functionality
func TestLocationCompressionFunctionality(t *testing.T) {
	// Input file path
	// Check if input file exists
	if _, err := os.Stat(testLocationFile); os.IsNotExist(err) {
		t.Skipf("Input file does not exist: %s", testLocationFile)
		return
	}

	// Read original data
	originalRows, err := parquet.ReadFile[profilev1.Location](testLocationFile)
	require.NoError(t, err)
	originalRowCount := len(originalRows)
	t.Logf("Original data row count: %d", originalRowCount)

	// Get original file size
	srcInfo, err := os.Stat(testLocationFile)
	require.NoError(t, err)
	originalSize := srcInfo.Size()
	t.Logf("Original file size: %d bytes", originalSize)

	// Test functionality correctness of different compression algorithms
	compressionTests := []struct {
		name  string
		codec compress.Codec
	}{
		{"GZIP Best Compression", &gzip.Codec{Level: gzip.BestCompression}},
		{"GZIP Default Compression", &gzip.Codec{Level: gzip.DefaultCompression}},
		{"ZSTD Default Compression", &zstd.Codec{Level: zstd.DefaultLevel}},
		{"Snappy Compression", &snappy.Codec{}},
		{"LZ4 Compression", &lz4.Codec{Level: lz4.DefaultLevel}},
	}

	for _, test := range compressionTests {
		t.Run(test.name, func(t *testing.T) {
			// Create temporary file
			tempFile, err := os.CreateTemp("", "locations_func_test_*.parquet")
			require.NoError(t, err)
			outputFile := tempFile.Name()
			tempFile.Close()
			defer os.Remove(outputFile) // Clean up file after test completion

			// Create compressed file
			destFile, err := os.Create(outputFile)
			require.NoError(t, err)
			defer destFile.Close()

			// Configure compression options
			options := []parquet.WriterOption{
				parquet.Compression(test.codec),
			}
			writer := parquet.NewGenericWriter[profilev1.Location](destFile, options...)

			// Write compressed data
			_, err = writer.Write(originalRows)
			require.NoError(t, err)
			err = writer.Close()
			require.NoError(t, err)
			destFile.Close()

			// Verify compressed file can be read correctly
			compressedRows, err := parquet.ReadFile[profilev1.Location](outputFile)
			require.NoError(t, err)
			require.Equal(t, originalRowCount, len(compressedRows), "Row count after compression should match original data")
			// Verify data content consistency (sample check of first few rows)
			sampleSize := 10
			if originalRowCount < sampleSize {
				sampleSize = originalRowCount
			}
			for i := 0; i < sampleSize; i++ {
				require.Equal(t, originalRows[i].Id, compressedRows[i].Id,
					"ID of row %d should be the same", i)
				require.Equal(t, originalRows[i].MappingId, compressedRows[i].MappingId,
					"MappingId of row %d should be the same", i)
				require.Equal(t, originalRows[i].Address, compressedRows[i].Address,
					"Address of row %d should be the same", i)
			}

			// Check compression effectiveness
			compressedInfo, err := os.Stat(outputFile)
			require.NoError(t, err)
			compressedSize := compressedInfo.Size()
			compressionRatio := float64(compressedSize) / float64(originalSize)
			t.Logf("%s: compressed size=%d bytes, compression ratio=%.2f%%",
				test.name, compressedSize, compressionRatio*100)

			// Ensure compression actually reduces file size (unless original file is very small)
			if originalSize > 1024 { // If original file is larger than 1KB
				require.Less(t, compressedSize, originalSize, "Compression should reduce file size")
			}
		})
	}
	t.Log("All compression algorithms functionality tests passed")
}

// CompressionResult stores the results of compression testing
type CompressionResult struct {
	Algorithm         string
	CompressedSize    int64
	CompressionTime   time.Duration
	DecompressionTime time.Duration
	CompressionRatio  float64
}

// TestLocationCompressionPerformance tests the performance of compression algorithms
func TestLocationCompressionPerformance(t *testing.T) {
	// Check if input file exists
	if _, err := os.Stat(testLocationFile); os.IsNotExist(err) {
		t.Skipf("Input file does not exist: %s", testLocationFile)
		return
	}
	// Read data
	rows, err := parquet.ReadFile[profilev1.Location](testLocationFile)
	require.NoError(t, err)
	numRows := len(rows)
	t.Logf("Test data row count: %d", numRows)

	// Get source file size
	srcInfo, err := os.Stat(testLocationFile)
	require.NoError(t, err)
	srcSize := srcInfo.Size()
	t.Logf("Source file size: %d bytes", srcSize)

	// Define compression algorithms and levels for performance testing
	performanceTests := []struct {
		name  string
		codec compress.Codec
	}{
		{"No Compression", nil},
		{"Snappy", &snappy.Codec{}},
		{"LZ4 Fastest", &lz4.Codec{Level: lz4.Fastest}},
		{"LZ4 Default", &lz4.Codec{Level: lz4.DefaultLevel}},
		{"LZ4 Best", &lz4.Codec{Level: lz4.Level9}},
		{"GZIP Fastest", &gzip.Codec{Level: gzip.BestSpeed}},
		{"GZIP Default", &gzip.Codec{Level: gzip.DefaultCompression}},
		{"GZIP Best", &gzip.Codec{Level: gzip.BestCompression}},
		{"ZSTD Fastest", &zstd.Codec{Level: zstd.SpeedFastest}},
		{"ZSTD Default", &zstd.Codec{Level: zstd.DefaultLevel}},
		{"ZSTD Best", &zstd.Codec{Level: zstd.SpeedBestCompression}},
	}

	// Store results
	var results []CompressionResult

	// Performance testing parameters
	const iterations = 10 // Number of runs for each algorithm
	// Performance test for each compression algorithm
	for _, test := range performanceTests {
		t.Run(test.name+"_Performance", func(t *testing.T) {
			var totalCompressTime time.Duration
			var totalDecompressTime time.Duration
			var totalCompressedSize int64
			for i := 0; i < iterations; i++ {
				// Create temporary file
				tempFile, err := os.CreateTemp("", "locations_perf_test_*.parquet")
				require.NoError(t, err)
				outputFile := tempFile.Name()
				tempFile.Close()
				defer os.Remove(outputFile) // Ensure cleanup of temporary files

				// Measure compression time and file size
				compressStart := time.Now()
				compressedSize, err := compressWithAlgorithm(rows, outputFile, test.codec)
				compressTime := time.Since(compressStart)
				require.NoError(t, err)

				// Measure decompression time
				decompressStart := time.Now()
				_, err = parquet.ReadFile[profilev1.Location](outputFile)
				decompressTime := time.Since(decompressStart)
				require.NoError(t, err)

				// Accumulate results
				totalCompressTime += compressTime
				totalDecompressTime += decompressTime
				totalCompressedSize += compressedSize
			}
			// Calculate averages
			avgCompressTime := totalCompressTime / iterations
			avgDecompressTime := totalDecompressTime / iterations
			avgCompressedSize := totalCompressedSize / iterations
			avgRatio := float64(avgCompressedSize) / float64(srcSize)

			// Record results
			result := CompressionResult{
				Algorithm:         test.name,
				CompressedSize:    avgCompressedSize,
				CompressionTime:   avgCompressTime,
				DecompressionTime: avgDecompressTime,
				CompressionRatio:  avgRatio,
			}
			results = append(results, result)

			t.Logf("Algorithm: %s, avg compressed size: %d bytes, compression ratio: %.2f%%, avg compression time: %v, avg decompression time: %v",
				test.name, avgCompressedSize, avgRatio*100, avgCompressTime, avgDecompressTime)
		})
	}

	// Output complete performance comparison table
	t.Log("\n=== Compression Algorithm Performance Comparison Results ===")
	fmt.Printf("\n%-15s | %-12s | %-10s | %-12s | %-12s\n",
		"Algorithm", "Size(KB)", "Ratio(%)", "Compress Time", "Decompress Time")
	fmt.Printf("%-15s-+-%-12s-+-%-10s-+-%-12s-+-%-12s\n",
		"---------------", "------------", "----------", "------------", "------------")

	for _, result := range results {
		fmt.Printf("%-15s | %-12.1f | %-10.2f | %-12v | %-12v\n",
			result.Algorithm,
			float64(result.CompressedSize)/1024,
			result.CompressionRatio*100,
			result.CompressionTime,
			result.DecompressionTime)
	}
}

// compressWithAlgorithm compresses data using specified algorithm and returns file size
func compressWithAlgorithm(rows []profilev1.Location, outputFile string, codec compress.Codec) (int64, error) {
	destFile, err := os.Create(outputFile)
	if err != nil {
		return 0, err
	}
	defer destFile.Close()

	// Create writer options
	var options []parquet.WriterOption
	if codec != nil {
		options = append(options, parquet.Compression(codec))
	}

	// Write compressed data
	writer := parquet.NewGenericWriter[profilev1.Location](destFile, options...)
	_, err = writer.Write(rows)
	if err != nil {
		return 0, err
	}
	err = writer.Close()
	if err != nil {
		return 0, err
	}

	// Get file size
	info, err := os.Stat(outputFile)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
