package block

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
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

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
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
	// Version1 is a enumeration of Pyroscope section of TSDB meta supported by Pyroscope.
	MetaVersion1 = MetaVersion(1)

	// MetaVersion2 indicates the block format version.
	// https://github.com/grafana/phlare/pull/767.
	//  1. In this version we introduced symdb:
	//     - stacktraces.parquet table has been deprecated.
	//     - StacktracePartition column added to profiles.parquet table.
	//     - symdb is stored in ./symbols sub-directory.
	//  2. TotalValue column added to profiles.parquet table.
	//  3. pprof labels discarded and never stored in the block.
	MetaVersion2 = MetaVersion(2)

	// MetaVersion3 indicates the block format version.
	// https://github.com/grafana/pyroscope/pull/2196.
	//  1. Introduction of symdb v2:
	//     - locations, functions, mappings, strings parquet tables
	//       moved to ./symbols sub-directory (symdb) and partitioned
	//       by StacktracePartition. References to the partitions
	//       are stored in the index.symdb file.
	//  2. In this version, parquet tables are never loaded into
	//     memory entirely. Instead, each partition (row range) is read
	//     from the block on demand at query time.
	MetaVersion3 = MetaVersion(3)
)

// IsValid returns true if the version is valid.
func (v MetaVersion) IsValid() bool {
	switch v {
	case MetaVersion1, MetaVersion2, MetaVersion3:
		return true
	default:
		return false
	}
}

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

// BlockDesc describes a block by ULID and time range.
type BlockDesc struct {
	ULID    ulid.ULID  `json:"ulid"`
	MinTime model.Time `json:"minTime"`
	MaxTime model.Time `json:"maxTime"`
}

type MetaStats struct {
	BlockStats
	FileStats      []FileStats
	TotalSizeBytes uint64
}

type FileStats struct {
	RelPath   string
	SizeBytes uint64
}

// BlockMetaCompaction holds information about compactions a block went through.
type BlockMetaCompaction struct {
	// Maximum number of compaction cycles any source block has
	// gone through.
	Level int `json:"level"`
	// ULIDs of all source head blocks that went into the block.
	Sources []ulid.ULID `json:"sources,omitempty"`
	// Indicates that during compaction it resulted in a block without any samples
	// so it should be deleted on the next reloadBlocks.
	Deletable bool `json:"deletable,omitempty"`
	// Short descriptions of the direct blocks that were used to create
	// this block.
	Parents []BlockDesc `json:"parents,omitempty"`
	Failed  bool        `json:"failed,omitempty"`
	// Additional information about the compaction, for example, block created from out-of-order chunks.
	Hints []string `json:"hints,omitempty"`
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

	// File is a sorted (by rel path) list of all files in block directory of this block known to PyroscopeDB.
	// Sorted by relative path.
	Files []File `json:"files,omitempty"`

	// Information on compactions the block was created from.
	Compaction BlockMetaCompaction `json:"compaction"`

	// Version of the index format.
	Version MetaVersion `json:"version"`

	// Labels are the external labels identifying the producer as well as tenant.
	Labels map[string]string `json:"labels"`

	// Source is a real upload source of the block.
	Source SourceType `json:"source,omitempty"`

	// Downsample is a downsampling resolution of the block. 0 means no downsampling.
	Downsample `json:"downsample"`
}

type Downsample struct {
	Resolution int64 `json:"resolution"`
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
		m.MinTime.Time().UTC().Format(time.RFC3339Nano),
		m.MaxTime.Time().UTC().Format(time.RFC3339Nano),
	)
}

func (m *Meta) Clone() *Meta {
	data, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	var clone Meta
	if err := json.Unmarshal(data, &clone); err != nil {
		panic(err)
	}
	return &clone
}
func (m *Meta) BlockInfo() *typesv1.BlockInfo {
	info := &typesv1.BlockInfo{}
	m.WriteBlockInfo(info)
	return info
}

func (m *Meta) WriteBlockInfo(info *typesv1.BlockInfo) {
	info.Ulid = m.ULID.String()
	info.MinTime = int64(m.MinTime)
	info.MaxTime = int64(m.MaxTime)
	if info.Compaction == nil {
		info.Compaction = &typesv1.BlockCompaction{}
	}
	info.Compaction.Level = int32(m.Compaction.Level)
	info.Compaction.Parents = make([]string, len(m.Compaction.Parents))
	for i, p := range m.Compaction.Parents {
		info.Compaction.Parents[i] = p.ULID.String()
	}
	info.Compaction.Sources = make([]string, len(m.Compaction.Sources))
	for i, s := range m.Compaction.Sources {
		info.Compaction.Sources[i] = s.String()
	}
	info.Labels = make([]*typesv1.LabelPair, 0, len(m.Labels))
	for k, v := range m.Labels {
		info.Labels = append(info.Labels, &typesv1.LabelPair{
			Name:  k,
			Value: v,
		})
	}
}

func generateULID() ulid.ULID {
	return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)
}

func NewMeta() *Meta {
	return &Meta{
		ULID: generateULID(),

		MinTime: math.MaxInt64,
		MaxTime: 0,
		Labels:  make(map[string]string),
		Version: MetaVersion3,
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
	switch m.Version {
	case MetaVersion1:
	case MetaVersion2:
	case MetaVersion3:
	default:
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

// WriteToFile writes the encoded meta into <dir>/meta.json.
func (meta *Meta) WriteToFile(logger log.Logger, dir string) (int64, error) {
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

func (meta *Meta) GetStats() MetaStats {
	fileStats := make([]FileStats, 0, len(meta.Files))
	totalSizeBytes := uint64(0)
	for _, file := range meta.Files {
		fileStats = append(fileStats, FileStats{
			RelPath:   file.RelPath,
			SizeBytes: file.SizeBytes,
		})
		totalSizeBytes += file.SizeBytes
	}

	return MetaStats{
		BlockStats:     meta.Stats,
		FileStats:      fileStats,
		TotalSizeBytes: totalSizeBytes,
	}
}

func (stats MetaStats) ConvertToBlockStats() *ingestv1.BlockStats {
	indexBytes := uint64(0)
	profileBytes := uint64(0)
	symbolBytes := uint64(0)
	for _, f := range stats.FileStats {
		if f.RelPath == IndexFilename {
			indexBytes = f.SizeBytes
		} else if f.RelPath == "profiles.parquet" {
			profileBytes += f.SizeBytes
		} else if strings.HasPrefix(f.RelPath, "symbols") || filepath.Ext(f.RelPath) == ".symdb" {
			symbolBytes += f.SizeBytes
		}
	}
	blockStats := &ingestv1.BlockStats{
		SeriesCount:  stats.NumSeries,
		ProfileCount: stats.NumProfiles,
		SampleCount:  stats.NumSamples,
		IndexBytes:   indexBytes,
		ProfileBytes: profileBytes,
		SymbolBytes:  symbolBytes,
	}
	return blockStats
}

// ReadMetaFromDir reads the given meta from <dir>/meta.json.
func ReadMetaFromDir(dir string) (*Meta, error) {
	f, err := os.Open(filepath.Join(dir, filepath.Clean(MetaFilename)))
	if err != nil {
		return nil, err
	}
	return Read(f)
}

func exhaustCloseWithErrCapture(err *error, r io.ReadCloser, msg string) {
	_, copyErr := io.Copy(io.Discard, r)

	runutil.CloseWithErrCapture(err, r, "%s", msg)

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

	switch m.Version {
	case MetaVersion1:
	case MetaVersion2:
	case MetaVersion3:
	default:
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
