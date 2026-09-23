package profiledump

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"unicode/utf8"
)

// See FORMAT.md for version 1 and the header layout.
const (
	Magic      = "PYRDUMP\n"
	Version    = 1
	HeaderSize = len(Magic) + 2 + 4
)

// Inspection contains validated metadata and declared sizes, without checking payload bytes.
type Inspection struct {
	Metadata      Metadata
	PayloadOffset int64
	ObjectSize    int64
}

// Encode streams one envelope within positive maxObjectSize, including overhead.
// payload must reach EOF at PayloadSize. Keep inputs stable until return.
// Inputs are not retained or closed. Discard output on error.
func Encode(w io.Writer, metadata Metadata, payload io.Reader, maxObjectSize int64) error {
	if err := encodeHeader(w, metadata, maxObjectSize); err != nil {
		return err
	}
	return copyPayload(w, payload, metadata.PayloadSize)
}

// encodeHeader is shared with the recorder's direct-to-buffer serializer.
func encodeHeader(w io.Writer, metadata Metadata, maxObjectSize int64) error {
	if err := metadata.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}
	if _, err := envelopeSize(int64(len(b)), metadata.PayloadSize, maxObjectSize); err != nil {
		return err
	}
	var header [HeaderSize]byte
	copy(header[:], Magic)
	binary.BigEndian.PutUint16(header[len(Magic):], metadata.SchemaVersion)
	binary.BigEndian.PutUint32(header[len(Magic)+2:], uint32(len(b))) // Bounded above by MaxMetadataSize.
	if err := writeBytes(w, header[:]); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if err := writeBytes(w, b); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	return nil
}

// Inspect reads only header and metadata, checking declared size against maxObjectSize.
// A range reader may end after metadata. The result owns its metadata.
// r is left at the payload and is neither retained nor closed.
func Inspect(r io.Reader, maxObjectSize int64) (Inspection, error) {
	if maxObjectSize < int64(HeaderSize) {
		return Inspection{}, fmt.Errorf("object size limit is smaller than header")
	}
	var header [HeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return Inspection{}, fmt.Errorf("read header: %w", err)
	}
	if string(header[:len(Magic)]) != Magic {
		return Inspection{}, fmt.Errorf("invalid envelope magic")
	}
	version := binary.BigEndian.Uint16(header[len(Magic):])
	if version != Version {
		return Inspection{}, fmt.Errorf("unsupported envelope version %d", version)
	}
	metadataSize := int64(binary.BigEndian.Uint32(header[len(Magic)+2:]))
	// Validate before conversion to int, allocation, or reading any metadata.
	if _, err := envelopeSize(metadataSize, 0, maxObjectSize); err != nil {
		return Inspection{}, err
	}
	b := make([]byte, int(metadataSize))
	if _, err := io.ReadFull(r, b); err != nil {
		return Inspection{}, fmt.Errorf("read metadata: %w", err)
	}
	if !utf8.Valid(b) {
		return Inspection{}, fmt.Errorf("metadata is not UTF-8")
	}
	// A pointer distinguishes a missing/null payload_size from a valid zero.
	var wire struct {
		Metadata
		PayloadSize *int64 `json:"payload_size"`
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&wire); err != nil {
		return Inspection{}, fmt.Errorf("decode metadata: %w", err)
	}
	if wire.SchemaVersion != version {
		return Inspection{}, fmt.Errorf("header and schema versions differ")
	}
	if wire.PayloadSize == nil {
		return Inspection{}, fmt.Errorf("payload_size is required")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return Inspection{}, fmt.Errorf("trailing data in metadata")
	}
	wire.Metadata.PayloadSize = *wire.PayloadSize
	if err := wire.Validate(); err != nil {
		return Inspection{}, fmt.Errorf("invalid metadata: %w", err)
	}
	size, err := envelopeSize(metadataSize, wire.Metadata.PayloadSize, maxObjectSize)
	if err != nil {
		return Inspection{}, err
	}
	return Inspection{Metadata: wire.Metadata, PayloadOffset: int64(HeaderSize) + metadataSize, ObjectSize: size}, nil
}

// Decode checks framing and size, streaming the payload to w. r must end at EOF
// after one object. Use io.Discard to validate without extraction. Discard output
// on error. Inputs are not retained or closed. Payload contents are not validated.
func Decode(r io.Reader, w io.Writer, maxObjectSize int64) (Metadata, error) {
	info, err := Inspect(r, maxObjectSize)
	if err != nil {
		return Metadata{}, err
	}
	if err := copyPayload(w, r, info.Metadata.PayloadSize); err != nil {
		return Metadata{}, err
	}
	return info.Metadata, nil
}

func envelopeSize(metadataSize, payloadSize, maxObjectSize int64) (int64, error) {
	if metadataSize <= 0 || metadataSize > MaxMetadataSize {
		return 0, fmt.Errorf("metadata length must be in [1, %d]", MaxMetadataSize)
	}
	offset := int64(HeaderSize) + metadataSize // Bounded metadata makes this safe.
	if payloadSize < 0 || payloadSize > math.MaxInt64-offset {
		return 0, fmt.Errorf("invalid or overflowing payload size")
	}
	size := offset + payloadSize
	if maxObjectSize <= 0 || size > maxObjectSize {
		return 0, fmt.Errorf("envelope exceeds object size limit")
	}
	return size, nil
}

func copyPayload(w io.Writer, r io.Reader, size int64) error {
	if _, err := io.CopyN(w, r, size); err != nil {
		return fmt.Errorf("copy payload: %w", err)
	}
	return requireEOF(r)
}

func requireEOF(r io.Reader) error {
	var extra [1]byte
	n, err := io.ReadFull(r, extra[:])
	if n != 0 {
		return fmt.Errorf("trailing bytes after declared payload")
	}
	if err != io.EOF {
		return fmt.Errorf("read payload end: %w", err)
	}
	return nil
}

func writeBytes(w io.Writer, b []byte) error {
	n, err := w.Write(b)
	if err == nil && n != len(b) {
		return io.ErrShortWrite
	}
	return err
}
