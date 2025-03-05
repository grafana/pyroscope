package symbolizer

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
)

// detectCompression reads the beginning of the input to determine if it's compressed,
// and if so, returns a ReaderAt that decompresses the data.
func detectCompression(r io.Reader) (io.ReaderAt, error) {
	// Use bufio.Reader to peek at the first few bytes without consuming them
	br := bufio.NewReader(r)

	// Peek at the first 4 bytes to check compression signatures
	header, err := br.Peek(4)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return nil, fmt.Errorf("peek header: %w", err)
	}

	// Check compression signatures
	if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b { // gzip
		// Create a gzip reader that reads from the bufio.Reader
		gr, err := gzip.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}

		// Read all decompressed data
		var decompressed bytes.Buffer
		if _, err := decompressed.ReadFrom(gr); err != nil {
			gr.Close()
			return nil, fmt.Errorf("decompress gzip data: %w", err)
		}
		gr.Close()

		return bytes.NewReader(decompressed.Bytes()), nil
	} else if len(header) >= 4 && header[0] == 0x28 && header[1] == 0xb5 && header[2] == 0x2f && header[3] == 0xfd { // zstd
		// Create a zstd reader
		zr, err := zstd.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("create zstd reader: %w", err)
		}

		// Read all decompressed data
		var decompressed bytes.Buffer
		if _, err := decompressed.ReadFrom(zr); err != nil {
			zr.Close()
			return nil, fmt.Errorf("decompress zstd data: %w", err)
		}
		zr.Close()

		return bytes.NewReader(decompressed.Bytes()), nil
	}

	// Not compressed or unknown format
	// Read all data into memory since we need to return an io.ReaderAt
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(br); err != nil {
		return nil, fmt.Errorf("read data: %w", err)
	}

	return bytes.NewReader(buf.Bytes()), nil
}
