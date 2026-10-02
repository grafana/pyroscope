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

// Encode streams an envelope within maxObjectSize, including overhead, and checks
// that payload ends at PayloadSize. Inputs are borrowed and remain open.
// Output is valid only on success.
func Encode(w io.Writer, metadata Metadata, payload io.Reader, maxObjectSize int64) error {
	prepared, err := prepareEnvelope(metadata, maxObjectSize)
	if err != nil {
		return err
	}
	return prepared.encode(w, payload)
}

// preparedEnvelope owns serialized metadata for admission and encoding.
// objectSize is the persisted size, excluding buffers retained alongside it.
type preparedEnvelope struct {
	metadataJSON []byte
	payloadSize  int64
	objectSize   int64
}

func prepareEnvelope(metadata Metadata, maxObjectSize int64) (preparedEnvelope, error) {
	// Validation bounds the initial JSON allocation.
	if err := metadata.Validate(); err != nil {
		return preparedEnvelope{}, err
	}
	b, err := json.Marshal(metadata)
	if err != nil {
		return preparedEnvelope{}, fmt.Errorf("encode metadata: %w", err)
	}
	size, err := envelopeSize(int64(len(b)), metadata.PayloadSize, maxObjectSize)
	if err != nil {
		return preparedEnvelope{}, err
	}
	return preparedEnvelope{metadataJSON: b, payloadSize: metadata.PayloadSize, objectSize: size}, nil
}

func (p preparedEnvelope) encode(w io.Writer, payload io.Reader) error {
	if err := p.writeHeader(w); err != nil {
		return err
	}
	return copyPayload(w, payload, p.payloadSize)
}

func (p preparedEnvelope) writeHeader(w io.Writer) error {
	var header [HeaderSize]byte
	copy(header[:], Magic)
	binary.BigEndian.PutUint16(header[len(Magic):], Version)
	binary.BigEndian.PutUint32(header[len(Magic)+2:], uint32(len(p.metadataJSON))) // Bounded by MaxMetadataSize.
	if err := writeBytes(w, header[:]); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if err := writeBytes(w, p.metadataJSON); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	return nil
}

// Inspect reads the header and metadata, checking declared size against maxObjectSize.
// It returns owned metadata and leaves r open at the payload, which may be absent.
func Inspect(r io.Reader, maxObjectSize int64) (Inspection, error) {
	version, metadataSize, err := readEnvelopeHeader(r, maxObjectSize)
	if err != nil {
		return Inspection{}, err
	}
	b := make([]byte, int(metadataSize))
	if _, err := io.ReadFull(r, b); err != nil {
		return Inspection{}, fmt.Errorf("read metadata: %w", err)
	}
	metadata, err := decodeMetadata(b, version)
	if err != nil {
		return Inspection{}, err
	}
	size, err := envelopeSize(metadataSize, metadata.PayloadSize, maxObjectSize)
	if err != nil {
		return Inspection{}, err
	}
	return Inspection{Metadata: metadata, PayloadOffset: int64(HeaderSize) + metadataSize, ObjectSize: size}, nil
}

func readEnvelopeHeader(r io.Reader, maxObjectSize int64) (uint16, int64, error) {
	if maxObjectSize < int64(HeaderSize) {
		return 0, 0, fmt.Errorf("object size limit is smaller than header")
	}
	var header [HeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, 0, fmt.Errorf("read header: %w", err)
	}
	if string(header[:len(Magic)]) != Magic {
		return 0, 0, fmt.Errorf("invalid envelope magic")
	}
	version := binary.BigEndian.Uint16(header[len(Magic):])
	if version != Version {
		return 0, 0, fmt.Errorf("unsupported envelope version %d", version)
	}
	metadataSize := int64(binary.BigEndian.Uint32(header[len(Magic)+2:]))
	// Bound metadata before the caller allocates or reads it.
	if _, err := envelopeSize(metadataSize, 0, maxObjectSize); err != nil {
		return 0, 0, err
	}
	return version, metadataSize, nil
}

func decodeMetadata(b []byte, version uint16) (Metadata, error) {
	if !utf8.Valid(b) {
		return Metadata{}, fmt.Errorf("metadata is not UTF-8")
	}
	// A pointer distinguishes a missing/null payload_size from a valid zero.
	var wire struct {
		Metadata
		PayloadSize *int64 `json:"payload_size"`
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&wire); err != nil {
		return Metadata{}, fmt.Errorf("decode metadata: %w", err)
	}
	if wire.SchemaVersion != version {
		return Metadata{}, fmt.Errorf("header and schema versions differ")
	}
	if wire.PayloadSize == nil {
		return Metadata{}, fmt.Errorf("payload_size is required")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return Metadata{}, fmt.Errorf("trailing data in metadata")
	}
	wire.Metadata.PayloadSize = *wire.PayloadSize
	if err := wire.Validate(); err != nil {
		return Metadata{}, fmt.Errorf("invalid metadata: %w", err)
	}
	return wire.Metadata, nil
}

// Decode validates one envelope through EOF and streams its opaque payload to w.
// Inputs remain open. Output is valid only on success.
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
