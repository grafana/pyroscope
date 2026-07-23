package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// The replay dump file is a simple, self-contained, streamable archive of
// individually reconstructed pprof profiles, together with their original
// series labels and timestamps. It is produced by `profilecli replay dump`
// and consumed by `profilecli replay push`.
//
// File layout:
//
//	magic (8 bytes) "PYRPLAY1"
//	varint headerLen
//	header (JSON, see replayHeader)
//	record*
//
// Each record is framed as:
//
//	varint recordLen
//	record body:
//	  varint timestampNanos (zigzag)
//	  varint numLabels
//	  (varint nameLen, name bytes, varint valueLen, value bytes){numLabels}
//	  varint pprofLen
//	  pprof bytes (gzip-compressed pprof, as produced by pkg/pprof.MustMarshal)
const (
	replayMagic        = "PYRPLAY1"
	replayMaxRecordLen = 512 << 20 // 512MiB safety limit for a single profile record
)

// replayHeader carries metadata about the dump, written once at the
// beginning of the file. It intentionally does not carry the min/max
// timestamp of the dump: that is derived by the reader from the records
// themselves, so the writer can stream records without buffering.
type replayHeader struct {
	Version     int      `json:"version"`
	SourceQuery string   `json:"source_query"`
	Tenants     []string `json:"tenants"`
	From        int64    `json:"from_unix_milli"`
	To          int64    `json:"to_unix_milli"`
	CreatedAt   int64    `json:"created_at_unix_milli"`
}

const replayFormatVersion = 1

// replayRecord is a single reconstructed profile: its original series
// labels, the timestamp it was recorded at (nanoseconds since epoch), and
// the gzip-compressed pprof bytes.
type replayRecord struct {
	Labels         []*typesv1.LabelPair
	TimestampNanos int64
	Pprof          []byte
}

// replayWriter buffers and frames replay records onto an io.Writer. It does
// not own or close the underlying writer: callers remain responsible for
// closing the destination (e.g. an *os.File) themselves.
type replayWriter struct {
	w       *bufio.Writer
	scratch [binary.MaxVarintLen64]byte
}

func newReplayWriter(w io.Writer, header replayHeader) (*replayWriter, error) {
	header.Version = replayFormatVersion
	bw := bufio.NewWriter(w)
	if _, err := bw.WriteString(replayMagic); err != nil {
		return nil, err
	}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal replay header: %w", err)
	}
	rw := &replayWriter{w: bw}
	rw.writeUvarint(uint64(len(headerBytes)))
	if _, err := bw.Write(headerBytes); err != nil {
		return nil, err
	}
	return rw, nil
}

func (rw *replayWriter) writeUvarint(v uint64) {
	n := binary.PutUvarint(rw.scratch[:], v)
	_, _ = rw.w.Write(rw.scratch[:n])
}

func (rw *replayWriter) WriteRecord(rec replayRecord) error {
	var body bufferedRecord
	body.writeVarint(rec.TimestampNanos)
	body.writeUvarint(uint64(len(rec.Labels)))
	for _, l := range rec.Labels {
		body.writeString(l.Name)
		body.writeString(l.Value)
	}
	body.writeUvarint(uint64(len(rec.Pprof)))
	body.buf = append(body.buf, rec.Pprof...)

	rw.writeUvarint(uint64(len(body.buf)))
	_, err := rw.w.Write(body.buf)
	return err
}

// Flush writes any buffered data to the underlying writer. It does not close
// the underlying writer.
func (rw *replayWriter) Flush() error {
	return rw.w.Flush()
}

// bufferedRecord is a small helper to build a record body in memory before
// writing its length prefix.
type bufferedRecord struct {
	buf     []byte
	scratch [binary.MaxVarintLen64]byte
}

func (b *bufferedRecord) writeUvarint(v uint64) {
	n := binary.PutUvarint(b.scratch[:], v)
	b.buf = append(b.buf, b.scratch[:n]...)
}

func (b *bufferedRecord) writeVarint(v int64) {
	n := binary.PutVarint(b.scratch[:], v)
	b.buf = append(b.buf, b.scratch[:n]...)
}

func (b *bufferedRecord) writeString(s string) {
	b.writeUvarint(uint64(len(s)))
	b.buf = append(b.buf, s...)
}

type replayReader struct {
	r      *bufio.Reader
	Header replayHeader
}

func newReplayReader(r io.Reader) (*replayReader, error) {
	br := bufio.NewReader(r)
	magic := make([]byte, len(replayMagic))
	if _, err := io.ReadFull(br, magic); err != nil {
		return nil, fmt.Errorf("failed to read replay file magic: %w", err)
	}
	if string(magic) != replayMagic {
		return nil, fmt.Errorf("not a profilecli replay dump file (bad magic %q)", magic)
	}
	headerLen, err := binary.ReadUvarint(br)
	if err != nil {
		return nil, fmt.Errorf("failed to read replay header length: %w", err)
	}
	headerBytes := make([]byte, headerLen)
	if _, err := io.ReadFull(br, headerBytes); err != nil {
		return nil, fmt.Errorf("failed to read replay header: %w", err)
	}
	var header replayHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("failed to unmarshal replay header: %w", err)
	}
	if header.Version != replayFormatVersion {
		return nil, fmt.Errorf("unsupported replay dump file version %d (expected %d)", header.Version, replayFormatVersion)
	}
	return &replayReader{r: br, Header: header}, nil
}

// ReadRecord reads the next record, returning io.EOF when the file is
// exhausted.
func (rr *replayReader) ReadRecord() (replayRecord, error) {
	recordLen, err := binary.ReadUvarint(rr.r)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return replayRecord{}, io.EOF
		}
		return replayRecord{}, fmt.Errorf("failed to read record length: %w", err)
	}
	if recordLen > replayMaxRecordLen {
		return replayRecord{}, fmt.Errorf("record length %d exceeds maximum %d", recordLen, replayMaxRecordLen)
	}
	body := make([]byte, recordLen)
	if _, err := io.ReadFull(rr.r, body); err != nil {
		return replayRecord{}, fmt.Errorf("failed to read record body: %w", err)
	}
	return decodeRecordBody(body)
}

func decodeRecordBody(body []byte) (replayRecord, error) {
	br := &byteReader{b: body}

	ts, err := br.readVarint()
	if err != nil {
		return replayRecord{}, fmt.Errorf("failed to read record timestamp: %w", err)
	}
	numLabels, err := br.readUvarint()
	if err != nil {
		return replayRecord{}, fmt.Errorf("failed to read record label count: %w", err)
	}
	labels := make([]*typesv1.LabelPair, 0, numLabels)
	for i := uint64(0); i < numLabels; i++ {
		name, err := br.readString()
		if err != nil {
			return replayRecord{}, fmt.Errorf("failed to read label name: %w", err)
		}
		value, err := br.readString()
		if err != nil {
			return replayRecord{}, fmt.Errorf("failed to read label value: %w", err)
		}
		labels = append(labels, &typesv1.LabelPair{Name: name, Value: value})
	}
	pprofLen, err := br.readUvarint()
	if err != nil {
		return replayRecord{}, fmt.Errorf("failed to read pprof length: %w", err)
	}
	pprofBytes, err := br.readBytes(int(pprofLen))
	if err != nil {
		return replayRecord{}, fmt.Errorf("failed to read pprof bytes: %w", err)
	}
	return replayRecord{
		Labels:         labels,
		TimestampNanos: ts,
		Pprof:          pprofBytes,
	}, nil
}

// byteReader is a minimal varint/bytes reader over an in-memory buffer,
// used to decode a single record body.
type byteReader struct {
	b   []byte
	off int
}

func (r *byteReader) readUvarint() (uint64, error) {
	v, n := binary.Uvarint(r.b[r.off:])
	if n <= 0 {
		return 0, errors.New("invalid uvarint")
	}
	r.off += n
	return v, nil
}

func (r *byteReader) readVarint() (int64, error) {
	v, n := binary.Varint(r.b[r.off:])
	if n <= 0 {
		return 0, errors.New("invalid varint")
	}
	r.off += n
	return v, nil
}

func (r *byteReader) readBytes(n int) ([]byte, error) {
	if n < 0 || r.off+n > len(r.b) {
		return nil, errors.New("unexpected end of record body")
	}
	out := r.b[r.off : r.off+n]
	r.off += n
	return out, nil
}

func (r *byteReader) readString() (string, error) {
	n, err := r.readUvarint()
	if err != nil {
		return "", err
	}
	b, err := r.readBytes(int(n))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
