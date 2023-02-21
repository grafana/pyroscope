package block

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/runutil"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

const (
	UnknownSource   SourceType = ""
	IngesterSource  SourceType = "ingester"
	CompactorSource SourceType = "compactor"
)

const (
	MetaFilename = "meta.json"
)

type SourceType string

type MetaVersion int

const (
	// Version1 is a enumeration of Phlare section of TSDB meta supported by Phlare.
	MetaVersion1 = MetaVersion(1)
)

type BlockStats struct {
	NumSamples  uint64 `json:"numSamples,omitempty"`
	NumSeries   uint64 `json:"numSeries,omitempty"`
	NumProfiles uint64 `json:"numProfiles,omitempty"`
}

type File struct {
	RelPath string `json:"relPath"`
	// SizeBytes is optional (e.g meta.json does not show size).
	SizeBytes uint64 `json:"sizeBytes,omitempty"`

	// Parquet can contain some optional Parquet file info
	Parquet *ParquetFile `json:"parquet,omitempty"`
	// TSDB can contain some optional TSDB file info
	TSDB *TSDBFile `json:"tsdb,omitempty"`
}

type ParquetFile struct {
	NumRowGroups uint64 `json:"numRowGroups,omitempty"`
	NumRows      uint64 `json:"numRows,omitempty"`
}

type TSDBFile struct {
	NumSeries uint64 `json:"numSeries,omitempty"`
}

type Meta struct {
	// Unique identifier for the block and its contents. Changes on compaction.
	ULID ulid.ULID `json:"ulid"`

	// MinTime and MaxTime specify the time range all samples
	// in the block are in.
	MinTime model.Time `json:"minTime"`
	MaxTime model.Time `json:"maxTime"`

	// Stats about the contents of the block.
	Stats BlockStats `json:"stats,omitempty"`

	// File is a sorted (by rel path) list of all files in block directory of this block known to PhlareDB.
	// Sorted by relative path.
	Files []File `json:"files,omitempty"`

	// Information on compactions the block was created from.
	Compaction tsdb.BlockMetaCompaction `json:"compaction"`

	// Version of the index format.
	Version MetaVersion `json:"version"`

	// Labels are the external labels identifying the producer as well as tenant.
	Labels map[string]string `json:"labels,omitempty"`

	// Source is a real upload source of the block.
	Source SourceType `json:"source,omitempty"`
}

func (m *Meta) FileByRelPath(name string) *File {
	for _, f := range m.Files {
		if f.RelPath == name {
			return &f
		}
	}
	return nil
}

func (m *Meta) InRange(start, end model.Time) bool {
	return InRange(m.MinTime, m.MaxTime, start, end)
}

func (m *Meta) String() string {
	return fmt.Sprintf(
		"%s (min time: %s, max time: %s)",
		m.ULID,
		m.MinTime.Time().Format(time.RFC3339Nano),
		m.MaxTime.Time().Format(time.RFC3339Nano),
	)
}

var ulidEntropy = rand.New(rand.NewSource(time.Now().UnixNano()))

func generateULID() ulid.ULID {
	return ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy)
}

func NewMeta() *Meta {
	return &Meta{
		ULID: generateULID(),

		MinTime: math.MaxInt64,
		MaxTime: 0,
		Labels:  make(map[string]string),
	}
}

func MetaFromDir(dir string) (*Meta, int64, error) {
	b, err := os.ReadFile(filepath.Join(dir, MetaFilename))
	if err != nil {
		return nil, 0, err
	}
	var m Meta

	if err := json.Unmarshal(b, &m); err != nil {
		return nil, 0, err
	}
	if m.Version != MetaVersion1 {
		return nil, 0, errors.Errorf("unexpected meta file version %d", m.Version)
	}

	return &m, int64(len(b)), nil
}

type wrappedWriter struct {
	w io.Writer
	n int
}

func (w *wrappedWriter) Write(p []byte) (n int, err error) {
	n, err = w.w.Write(p)
	if err != nil {
		return 0, err
	}
	w.n += n
	return n, nil
}

func (meta *Meta) WriteTo(w io.Writer) (int64, error) {
	wrapped := &wrappedWriter{
		w: w,
	}
	enc := json.NewEncoder(wrapped)
	enc.SetIndent("", "\t")
	return int64(wrapped.n), enc.Encode(meta)
}

func (meta *Meta) WriteToFile(logger log.Logger, dir string) (int64, error) {
	meta.Version = MetaVersion1

	// Make any changes to the file appear atomic.
	path := filepath.Join(dir, MetaFilename)
	tmp := path + ".tmp"
	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			level.Error(logger).Log("msg", "remove tmp file", "err", err.Error())
		}
	}()

	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}

	jsonMeta, err := json.MarshalIndent(meta, "", "\t")
	if err != nil {
		return 0, err
	}

	n, err := f.Write(jsonMeta)
	if err != nil {
		return 0, multierror.New(err, f.Close()).Err()
	}

	// Force the kernel to persist the file on disk to avoid data loss if the host crashes.
	if err := f.Sync(); err != nil {
		return 0, multierror.New(err, f.Close()).Err()
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	return int64(n), fileutil.Replace(tmp, path)
}

func (meta *Meta) TSDBBlockMeta() tsdb.BlockMeta {
	return tsdb.BlockMeta{
		ULID:    meta.ULID,
		MinTime: int64(meta.MinTime),
		MaxTime: int64(meta.MaxTime),
	}
}

// ReadFromDir reads the given meta from <dir>/meta.json.
func ReadFromDir(dir string) (*Meta, error) {
	f, err := os.Open(filepath.Join(dir, filepath.Clean(MetaFilename)))
	if err != nil {
		return nil, err
	}
	return Read(f)
}

func exhaustCloseWithErrCapture(err *error, r io.ReadCloser, format string) {
	_, copyErr := io.Copy(io.Discard, r)

	runutil.CloseWithErrCapture(err, r, format)

	// Prepend the io.Copy error.
	merr := multierror.MultiError{}
	merr.Add(copyErr)
	merr.Add(*err)

	*err = merr.Err()
}

// Read the block meta from the given reader.
func Read(rc io.ReadCloser) (_ *Meta, err error) {
	defer exhaustCloseWithErrCapture(&err, rc, "close meta JSON")

	var m Meta
	if err = json.NewDecoder(rc).Decode(&m); err != nil {
		return nil, err
	}

	if m.Version != MetaVersion1 {
		return nil, errors.Errorf("unexpected meta file version %d", m.Version)
	}

	return &m, nil
}

func InRange(min, max, start, end model.Time) bool {
	if start > max {
		return false
	}
	if end < min {
		return false
	}
	return true
}
