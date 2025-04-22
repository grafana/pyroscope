package lidia_test

import (
	"archive/zip"
	"debug/elf"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/lidia"
)

const compressedTestBinaryFile = "testdata/test-binary.zip"
const decompressedDir = "decompressed"

// TestCreateLidia tests the CreateLidia function with the test binary
func TestCreateLidia(t *testing.T) {
	binaryPath := decompressBinary(t)
	if binaryPath == "" {
		return
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.lidia")

	err := lidia.CreateLidia(binaryPath, outputPath,
		lidia.WithCRC(), lidia.WithFiles(), lidia.WithLines())
	require.NoError(t, err)

	fileInfo, err := os.Stat(outputPath)
	require.NoError(t, err)
	require.Greater(t, fileInfo.Size(), int64(0), "Lidia file should not be empty")
}

// TestCreateLidiaFromELF tests the CreateLidiaFromELF function with the test binary
func TestCreateLidiaFromELF(t *testing.T) {
	binaryPath := decompressBinary(t)
	if binaryPath == "" {
		return
	}

	elfFile, err := elf.Open(binaryPath)
	require.NoError(t, err)
	defer elfFile.Close()

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.lidia")
	outputFile, err := os.Create(outputPath)
	require.NoError(t, err)
	defer outputFile.Close()

	err = lidia.CreateLidiaFromELF(elfFile, outputFile,
		lidia.WithCRC(), lidia.WithFiles(), lidia.WithLines())
	require.NoError(t, err)

	fileInfo, err := outputFile.Stat()
	require.NoError(t, err)
	require.Greater(t, fileInfo.Size(), int64(0), "Lidia file should not be empty")
}

// TestCreateReadLookup is a comprehensive test that creates a lidia file,
// reads it back, and tests lookups against the same file
func TestCreateReadLookup(t *testing.T) {
	binaryPath := decompressBinary(t)
	if binaryPath == "" {
		return
	}

	tmpDir := t.TempDir()
	lidiaPath := filepath.Join(tmpDir, "test.lidia")

	// Create the lidia file
	err := lidia.CreateLidia(binaryPath, lidiaPath,
		lidia.WithCRC(), lidia.WithFiles(), lidia.WithLines())
	require.NoError(t, err)

	// Read the created lidia file
	bs, err := os.ReadFile(lidiaPath)
	require.NoError(t, err)

	// Open the lidia table
	var reader lidia.ReaderAtCloser = &bufferCloser{bs, 0}
	table, err := lidia.OpenReader(reader, lidia.WithCRC())
	require.NoError(t, err)
	defer table.Close()

	testCases := []struct {
		name           string
		addr           uint64
		expectFunction string
		expectFound    bool
	}{
		{
			name:           "Unknown address",
			addr:           0x100,
			expectFunction: "",
			expectFound:    false,
		},
		{
			name:           "Known function address",
			addr:           0x581ddc0,
			expectFunction: "github.com/prometheus/client_golang/prometheus..typeAssert.0",
			expectFound:    true,
		},
		{
			name:           "Known function address",
			addr:           0x581df40,
			expectFunction: "github.com/uber/jaeger-client-go/config..typeAssert.0",
			expectFound:    true,
		},
	}

	var results []lidia.SourceInfoFrame

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results, err := table.Lookup(results, tc.addr)
			require.NoError(t, err)
			if tc.expectFound {
				require.NotEmpty(t, results, "Expected to find a function at this address")
				require.Equal(t, tc.expectFunction, results[0].FunctionName)
			} else {
				require.Empty(t, results, "Expected no function at this address")
			}
		})
	}
}

// bufferCloser implements the lidia.ReaderAtCloser interface for testing
type bufferCloser struct {
	bs  []byte
	off int64
}

func (b *bufferCloser) Read(p []byte) (n int, err error) {
	res, err := b.ReadAt(p, b.off)
	b.off += int64(res)
	return res, err
}

func (b *bufferCloser) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(b.bs)) {
		return 0, io.EOF
	}
	n = copy(p, b.bs[off:])
	return n, nil
}

func (b *bufferCloser) Close() error {
	return nil
}

// decompressBinary decompresses the test binary from zip to a temporary location and returns its path
func decompressBinary(t *testing.T) string {
	// Skip if the compressed test file doesn't exist
	if _, err := os.Stat(compressedTestBinaryFile); os.IsNotExist(err) {
		t.Skip("Compressed test binary not found")
		return ""
	}

	// Create a temporary directory for the decompressed file
	tmpDir := t.TempDir()
	decompressedPath := filepath.Join(tmpDir, decompressedDir)
	err := os.MkdirAll(decompressedPath, 0755)
	require.NoError(t, err)

	outputPath := filepath.Join(decompressedPath, "test-binary")

	// Open the zip archive
	zipReader, err := zip.OpenReader(compressedTestBinaryFile)
	require.NoError(t, err)
	defer zipReader.Close()

	// We expect only one file in the archive - the binary
	if len(zipReader.File) == 0 {
		t.Skip("Zip archive is empty")
		return ""
	}

	// Get the first file from the archive
	zipFile := zipReader.File[0]
	zippedReader, err := zipFile.Open()
	require.NoError(t, err)
	defer zippedReader.Close()

	// Create output file
	outputFile, err := os.Create(outputPath)
	require.NoError(t, err)
	defer outputFile.Close()

	// Copy unzipped data to output file
	_, err = io.Copy(outputFile, zippedReader)
	require.NoError(t, err)

	// Ensure the file is executable
	err = os.Chmod(outputPath, 0755)
	require.NoError(t, err)

	return outputPath
}
