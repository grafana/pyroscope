package block

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"sync"

	"github.com/cespare/xxhash/v2"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/prometheus/model/labels"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/v2/pkg/util/build"
)

var ErrSymbolBloomTooManyCandidates = errors.New("symbol bloom candidate limit exceeded")

const (
	symbolBloomIndexFormatVersion = 1
	symbolBloomDefaultFP          = 0.01
	symbolBloomMinBits            = 64
	symbolBloomMaxHashCount       = 64
	symbolBloomHashSeed           = "\x00pyroscope-symbol-bloom-v1\x00"
	symbolBloomScanWorkers        = 4
)

type SymbolBloomIndexEntry struct {
	ServiceName  string
	DatasetIndex uint32
	MinTime      int64
	MaxTime      int64
	Symbols      []string
}

type SymbolBloomIndexRow struct {
	ServiceName         string `parquet:"service_name,dict"`
	DatasetIndex        uint32 `parquet:"dataset_index,delta"`
	MinTime             int64  `parquet:"min_time,delta"`
	MaxTime             int64  `parquet:"max_time,delta"`
	BloomBits           []byte `parquet:"bloom_bits"`
	BloomHashCount      uint32 `parquet:"bloom_hash_count,delta"`
	BloomBitCount       uint32 `parquet:"bloom_bit_count,delta"`
	SymbolCountEstimate uint32 `parquet:"symbol_count_estimate,delta"`
	FormatVersion       uint32 `parquet:"format_version,delta"`
}

type SymbolBloomIndexWriter struct {
	falsePositiveRate float64
	rows              []SymbolBloomIndexRow
}

func NewSymbolBloomIndexWriter(falsePositiveRate float64) *SymbolBloomIndexWriter {
	if falsePositiveRate <= 0 || falsePositiveRate >= 1 || math.IsNaN(falsePositiveRate) {
		falsePositiveRate = symbolBloomDefaultFP
	}
	return &SymbolBloomIndexWriter{falsePositiveRate: falsePositiveRate}
}

func (w *SymbolBloomIndexWriter) Add(entry SymbolBloomIndexEntry) {
	symbols := uniqueStrings(entry.Symbols)
	row := SymbolBloomIndexRow{
		ServiceName:         entry.ServiceName,
		DatasetIndex:        entry.DatasetIndex,
		MinTime:             entry.MinTime,
		MaxTime:             entry.MaxTime,
		SymbolCountEstimate: uint32(len(symbols)),
		FormatVersion:       symbolBloomIndexFormatVersion,
	}
	if len(symbols) > 0 {
		row.BloomBitCount, row.BloomHashCount = bloomSizing(len(symbols), w.falsePositiveRate)
		row.BloomBits = make([]byte, (row.BloomBitCount+7)/8)
		for _, symbol := range symbols {
			row.add(symbol)
		}
	}
	w.rows = append(w.rows, row)
}

func (w *SymbolBloomIndexWriter) Empty() bool { return len(w.rows) == 0 }

func (w *SymbolBloomIndexWriter) BloomBitsBytes() uint64 {
	var n uint64
	for _, row := range w.rows {
		n += uint64(len(row.BloomBits))
	}
	return n
}

func (w *SymbolBloomIndexWriter) WriteTo(dst io.Writer) (int64, error) {
	if len(w.rows) == 0 {
		return 0, nil
	}
	cw := &countingWriter{w: dst}
	pw := parquet.NewGenericWriter[SymbolBloomIndexRow](cw,
		parquet.CreatedBy("github.com/grafana/pyroscope/", build.Version, build.Revision),
		parquet.MaxRowsPerRowGroup(maxRowsPerRowGroup))
	if _, err := pw.Write(w.rows); err != nil {
		_ = pw.Close()
		return cw.n, err
	}
	if err := pw.Close(); err != nil {
		return cw.n, err
	}
	w.rows = w.rows[:0]
	return cw.n, nil
}

type SymbolBloomIndex struct {
	file *ParquetFile
}

type SymbolBloomLookupRequest struct {
	SymbolNames   []string
	SymbolName    string
	MinTime       int64
	MaxTime       int64
	Matchers      []*labels.Matcher
	MaxCandidates int
}

type SymbolBloomLookupResult struct {
	Candidates []SymbolBloomIndexRow
	Complete   bool
}

type SymbolBloomVerifiedService struct {
	ServiceName  string
	ProfileTypes []string
}

type SymbolBloomSymbolResult struct {
	SymbolName string
	Services   []SymbolBloomVerifiedService
}

type SymbolBloomServiceLookupResult struct {
	Results  []SymbolBloomSymbolResult
	Complete bool
}

func LookupSymbolBloomCandidates(ctx context.Context, bucket objstore.Bucket, md *metastorev1.BlockMeta, req SymbolBloomLookupRequest, options ...ObjectOption) (SymbolBloomLookupResult, error) {
	span := oteltrace.SpanFromContext(ctx)
	span.AddEvent("symbol_bloom_candidates.start", oteltrace.WithAttributes(
		attribute.Int("symbol_count", len(req.symbolNames())),
		attribute.Int("max_candidates", req.MaxCandidates),
	))

	obj := NewObject(bucket, md, options...)
	fullMD, err := obj.ReadMetadata(ctx)
	if err != nil {
		return SymbolBloomLookupResult{}, err
	}
	obj.SetMetadata(fullMD)

	matcher := labels.MustNewMatcher(labels.MatchEqual, metadata.LabelNameTenantDataset, metadata.LabelValueSymbolBloomIndex)
	var symbolBloomDatasets []*metastorev1.Dataset
	for ds := range metadata.FindDatasets(fullMD, matcher) {
		symbolBloomDatasets = append(symbolBloomDatasets, ds)
	}
	if len(symbolBloomDatasets) == 0 {
		span.AddEvent("symbol_bloom_candidates.no_bloom_index")
		return SymbolBloomLookupResult{Complete: false}, nil
	}

	result := SymbolBloomLookupResult{Complete: true}
	for _, symbolBloomDataset := range symbolBloomDatasets {
		ds := NewDataset(symbolBloomDataset, obj)
		if err := ds.Open(ctx, SectionSymbolBloomIndex); err != nil {
			return result, err
		}
		err := ds.SymbolBloomIndex().Scan(ctx, req, symbolBloomScanWorkers, func(row SymbolBloomIndexRow) error {
			candidate, err := row.Matches(req)
			if err != nil {
				return err
			}
			if candidate {
				if int(row.DatasetIndex) >= len(fullMD.Datasets) {
					return fmt.Errorf("symbol bloom candidate dataset index %d out of range", row.DatasetIndex)
				}
				if !row.matchesLabelMatchers(req.Matchers) {
					return nil
				}
				result.Candidates = append(result.Candidates, row)
				if req.MaxCandidates > 0 && len(result.Candidates) > req.MaxCandidates {
					return fmt.Errorf("%w: limit=%d", ErrSymbolBloomTooManyCandidates, req.MaxCandidates)
				}
			}
			return nil
		})
		closeErr := ds.Close()
		if err != nil {
			return result, err
		}
		if closeErr != nil {
			return result, closeErr
		}
	}
	span.AddEvent("symbol_bloom_candidates.done", oteltrace.WithAttributes(
		attribute.Int("candidate_count", len(result.Candidates)),
		attribute.Bool("complete", result.Complete),
	))
	return result, nil
}

func LookupSymbolBloomServices(ctx context.Context, bucket objstore.Bucket, md *metastorev1.BlockMeta, req SymbolBloomLookupRequest, options ...ObjectOption) (SymbolBloomServiceLookupResult, error) {
	span := oteltrace.SpanFromContext(ctx)
	span.AddEvent("symbol_bloom_services.start", oteltrace.WithAttributes(
		attribute.Int("symbol_count", len(req.symbolNames())),
	))

	candidates, err := LookupSymbolBloomCandidates(ctx, bucket, md, req, options...)
	if err != nil {
		return SymbolBloomServiceLookupResult{}, err
	}
	result := SymbolBloomServiceLookupResult{Complete: candidates.Complete}
	seen := make(map[string]map[string]map[string]struct{})
	for _, symbolName := range req.symbolNames() {
		seen[symbolName] = make(map[string]map[string]struct{})
	}
	candidatesByDataset := make(map[uint32][]SymbolBloomIndexRow)
	for _, candidate := range candidates.Candidates {
		candidatesByDataset[candidate.DatasetIndex] = append(candidatesByDataset[candidate.DatasetIndex], candidate)
	}
	span.AddEvent("symbol_bloom_services.verify_start", oteltrace.WithAttributes(
		attribute.Int("candidate_count", len(candidates.Candidates)),
		attribute.Int("dataset_count", len(candidatesByDataset)),
	))
	for datasetIndex, datasetCandidates := range candidatesByDataset {
		found, err := VerifySymbolsInDataset(ctx, bucket, md, datasetIndex, req.symbolNames(), req.Matchers, options...)
		if err != nil {
			return result, err
		}
		for _, candidate := range datasetCandidates {
			for _, symbolName := range req.symbolNames() {
				possible, err := candidate.MightContain(symbolName)
				if err != nil {
					return result, err
				}
				profileTypesForSymbol := found[symbolName]
				if !possible || len(profileTypesForSymbol) == 0 {
					continue
				}
				services := seen[symbolName]
				profileTypes := services[candidate.ServiceName]
				if profileTypes == nil {
					profileTypes = make(map[string]struct{})
					services[candidate.ServiceName] = profileTypes
				}
				for profileType := range profileTypesForSymbol {
					profileTypes[profileType] = struct{}{}
				}
			}
		}
	}
	for _, symbolName := range req.symbolNames() {
		services := make([]string, 0, len(seen[symbolName]))
		for service := range seen[symbolName] {
			services = append(services, service)
		}
		sort.Strings(services)
		symbolResult := SymbolBloomSymbolResult{SymbolName: symbolName}
		for _, service := range services {
			profileTypes := make([]string, 0, len(seen[symbolName][service]))
			for profileType := range seen[symbolName][service] {
				profileTypes = append(profileTypes, profileType)
			}
			sort.Strings(profileTypes)
			symbolResult.Services = append(symbolResult.Services, SymbolBloomVerifiedService{
				ServiceName:  service,
				ProfileTypes: profileTypes,
			})
		}
		result.Results = append(result.Results, symbolResult)
	}
	span.AddEvent("symbol_bloom_services.done")
	return result, nil
}

func VerifySymbolBloomCandidate(ctx context.Context, bucket objstore.Bucket, md *metastorev1.BlockMeta, candidate SymbolBloomIndexRow, symbolName string, options ...ObjectOption) (bool, error) {
	found, err := VerifySymbolsInDataset(ctx, bucket, md, candidate.DatasetIndex, []string{symbolName}, nil, options...)
	if err != nil {
		return false, err
	}
	return len(found[symbolName]) > 0, nil
}

func VerifySymbolsInDataset(ctx context.Context, bucket objstore.Bucket, md *metastorev1.BlockMeta, datasetIndex uint32, symbolNames []string, matchers []*labels.Matcher, options ...ObjectOption) (map[string]map[string]struct{}, error) {
	span := oteltrace.SpanFromContext(ctx)
	span.AddEvent("verify_symbols_in_dataset.start", oteltrace.WithAttributes(
		attribute.Int("dataset_index", int(datasetIndex)),
		attribute.Int("symbol_count", len(symbolNames)),
	))

	found := make(map[string]map[string]struct{}, len(symbolNames))
	want := make(map[string]struct{}, len(symbolNames))
	for _, symbolName := range symbolNames {
		if symbolName == "" {
			continue
		}
		want[symbolName] = struct{}{}
		found[symbolName] = make(map[string]struct{})
	}
	if len(want) == 0 {
		return found, nil
	}

	obj := NewObject(bucket, md, options...)
	fullMD, err := obj.ReadMetadata(ctx)
	if err != nil {
		return nil, err
	}
	obj.SetMetadata(fullMD)

	if int(datasetIndex) >= len(fullMD.Datasets) {
		return nil, fmt.Errorf("symbol bloom candidate dataset index %d out of range", datasetIndex)
	}
	target := fullMD.Datasets[datasetIndex]
	if DatasetFormat(target.Format) != DatasetFormat0 {
		return nil, fmt.Errorf("symbol bloom candidate dataset index %d references dataset format %d", datasetIndex, target.Format)
	}
	ds := NewDataset(target, obj)
	if err := ds.Open(ctx, SectionProfiles, SectionSymbols, SectionTSDB); err != nil {
		return nil, err
	}
	defer func() { _ = ds.Close() }()

	profiles, err := NewProfileRowIterator(ds)
	if err != nil {
		return nil, err
	}
	defer func() { _ = profiles.Close() }()
	// Keep symbol partitions open while scanning the dataset. Many profiles often
	// share a partition, and releasing per profile refetches the same symbol data.
	partitions := make(map[uint64]symdb.PartitionReader)
	defer func() {
		for _, partition := range partitions {
			partition.Release()
		}
	}()
	var locations []uint64
	var profilesScanned int
	for profiles.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		profilesScanned++
		entry := profiles.At()
		if !profileLabelsMatch(matchers, entry.Labels) {
			continue
		}
		profileType := profileTypeLabel(entry.Labels)
		partitionID := entry.Row.StacktracePartitionID()
		partition := partitions[partitionID]
		if partition == nil {
			partition, err = ds.Symbols().Partition(ctx, partitionID)
			if err != nil {
				return nil, err
			}
			partitions[partitionID] = partition
		}
		symbols := partition.Symbols()
		entry.Row.ForStacktraceIDsValues(func(values []parquet.Value) {
			for _, value := range values {
				stacktraceSymbolNames(symbols, value.Uint32(), &locations, func(symbolName string) bool {
					if _, ok := want[symbolName]; !ok {
						return false
					}
					found[symbolName][profileType] = struct{}{}
					return false
				})
			}
		})
	}
	if err := profiles.Err(); err != nil {
		return nil, err
	}
	span.AddEvent("verify_symbols_in_dataset.done", oteltrace.WithAttributes(
		attribute.Int("dataset_index", int(datasetIndex)),
		attribute.Int("profiles_scanned", profilesScanned),
	))
	return found, nil
}

func profileLabelsMatch(matchers []*labels.Matcher, ls phlaremodel.Labels) bool {
	for _, matcher := range matchers {
		matched := false
		for _, label := range ls {
			if label.Name != matcher.Name {
				continue
			}
			matched = matcher.Matches(label.Value)
			break
		}
		if !matched {
			return false
		}
	}
	return true
}

func openSymbolBloomIndex(_ context.Context, s *Dataset) (err error) {
	offset := s.sectionOffset(SectionSymbolBloomIndex)
	size := s.sectionSize(SectionSymbolBloomIndex)
	var file *ParquetFile
	if buf := s.inMemoryBuffer(); buf != nil {
		offset -= int64(s.offset())
		file, err = openParquetFile(
			s.inMemoryBucket(buf), s.obj.path, offset, size,
			0,
			parquet.SkipBloomFilters(true),
			parquet.FileReadMode(parquet.ReadModeSync),
			parquet.ReadBufferSize(4<<10))
	} else {
		file, err = openParquetFile(
			s.obj.storage, s.obj.path, offset, size,
			estimateFooterSize(size),
			parquet.SkipBloomFilters(true),
			parquet.FileReadMode(parquet.ReadModeAsync),
			parquet.ReadBufferSize(estimateReadBufferSize(size)))
	}
	if err != nil {
		return fmt.Errorf("opening symbol bloom parquet table: %w", err)
	}
	s.symbolBloom = &SymbolBloomIndex{file: file}
	return nil
}

func (idx *SymbolBloomIndex) ReadAll() ([]SymbolBloomIndexRow, error) {
	if idx == nil || idx.file == nil || idx.file.reader == nil {
		return nil, fmt.Errorf("symbol bloom index is not open")
	}
	section := io.NewSectionReader(idx.file.reader, idx.file.off, idx.file.size)
	return ReadSymbolBloomIndex(section)
}

func (idx *SymbolBloomIndex) Scan(ctx context.Context, req SymbolBloomLookupRequest, workers int, emit func(SymbolBloomIndexRow) error) error {
	if idx == nil || idx.file == nil {
		return fmt.Errorf("symbol bloom index is not open")
	}
	rowGroups := idx.file.RowGroups()
	if workers <= 0 || workers > symbolBloomScanWorkers {
		workers = symbolBloomScanWorkers
	}
	if workers > len(rowGroups) {
		workers = len(rowGroups)
	}
	if workers == 0 {
		return nil
	}

	g, ctx := errgroup.WithContext(ctx)
	work := make(chan parquet.RowGroup)
	var mu sync.Mutex
	for range workers {
		g.Go(func() error {
			buf := make([]SymbolBloomIndexRow, 128)
			for rg := range work {
				if err := ctx.Err(); err != nil {
					return err
				}
				r := parquet.NewGenericRowGroupReader[SymbolBloomIndexRow](rg)
				for {
					n, err := r.Read(buf)
					for i := range n {
						row := cloneSymbolBloomIndexRow(buf[i])
						if err := row.Validate(); err != nil {
							_ = r.Close()
							return err
						}
						matches, err := row.Matches(req)
						if err != nil {
							_ = r.Close()
							return err
						}
						if !matches {
							continue
						}
						mu.Lock()
						emitErr := emit(row)
						mu.Unlock()
						if emitErr != nil {
							_ = r.Close()
							return emitErr
						}
					}
					if err == io.EOF {
						break
					}
					if err != nil {
						_ = r.Close()
						return err
					}
				}
				if err := r.Close(); err != nil {
					return err
				}
			}
			return nil
		})
	}
	g.Go(func() error {
		defer close(work)
		for _, rg := range rowGroups {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case work <- rg:
			}
		}
		return nil
	})
	return g.Wait()
}

func (idx *SymbolBloomIndex) Close() error {
	if idx.file != nil {
		return idx.file.Close()
	}
	return nil
}

func ReadSymbolBloomIndex(r io.ReaderAt) ([]SymbolBloomIndexRow, error) {
	pr := parquet.NewGenericReader[SymbolBloomIndexRow](r)
	defer func() { _ = pr.Close() }()
	buf := make([]SymbolBloomIndexRow, 128)
	var rows []SymbolBloomIndexRow
	for {
		n, err := pr.Read(buf)
		for i := range n {
			if err := buf[i].Validate(); err != nil {
				return nil, err
			}
			rows = append(rows, cloneSymbolBloomIndexRow(buf[i]))
		}
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func (r SymbolBloomIndexRow) Validate() error {
	if r.FormatVersion != symbolBloomIndexFormatVersion {
		return fmt.Errorf("unsupported symbol bloom index format version %d", r.FormatVersion)
	}
	if r.BloomHashCount > symbolBloomMaxHashCount {
		return fmt.Errorf("symbol bloom hash count %d exceeds maximum %d", r.BloomHashCount, symbolBloomMaxHashCount)
	}
	if r.BloomBitCount == 0 || r.BloomHashCount == 0 {
		if len(r.BloomBits) != 0 || r.BloomBitCount != 0 || r.BloomHashCount != 0 {
			return fmt.Errorf("invalid empty symbol bloom filter")
		}
		return nil
	}
	if uint32(len(r.BloomBits))*8 < r.BloomBitCount {
		return fmt.Errorf("symbol bloom bitset too small: %d bytes for %d bits", len(r.BloomBits), r.BloomBitCount)
	}
	return nil
}

func (r SymbolBloomIndexRow) MightContain(symbol string) (bool, error) {
	if err := r.Validate(); err != nil {
		return false, err
	}
	if r.BloomBitCount == 0 || r.BloomHashCount == 0 {
		return false, nil
	}
	h1, h2 := symbolBloomHashes(symbol)
	for i := uint32(0); i < r.BloomHashCount; i++ {
		bit := uint32((h1 + uint64(i)*h2) % uint64(r.BloomBitCount))
		if r.BloomBits[bit/8]&(1<<(bit%8)) == 0 {
			return false, nil
		}
	}
	return true, nil
}

func (r SymbolBloomIndexRow) Matches(req SymbolBloomLookupRequest) (bool, error) {
	if req.MinTime != 0 && r.MaxTime < req.MinTime {
		return false, nil
	}
	if req.MaxTime != 0 && r.MinTime > req.MaxTime {
		return false, nil
	}
	for _, symbolName := range req.symbolNames() {
		contains, err := r.MightContain(symbolName)
		if err != nil {
			return false, err
		}
		if contains {
			return true, nil
		}
	}
	return false, nil
}

func (r SymbolBloomIndexRow) matchesLabelMatchers(matchers []*labels.Matcher) bool {
	for _, matcher := range matchers {
		var value string
		switch matcher.Name {
		case "service_name":
			value = r.ServiceName
		default:
			continue
		}
		if !matcher.Matches(value) {
			return false
		}
	}
	return true
}

func (req SymbolBloomLookupRequest) symbolNames() []string {
	if len(req.SymbolNames) > 0 {
		return req.SymbolNames
	}
	if req.SymbolName != "" {
		return []string{req.SymbolName}
	}
	return nil
}

func stacktraceContainsFunctionName(symbols *symdb.Symbols, stacktraceID uint32, symbolName string, locations *[]uint64) bool {
	var found bool
	stacktraceSymbolNames(symbols, stacktraceID, locations, func(name string) bool {
		found = name == symbolName
		return found
	})
	return found
}

func stacktraceSymbolNames(symbols *symdb.Symbols, stacktraceID uint32, locations *[]uint64, fn func(string) bool) {
	*locations = symbols.Stacktraces.LookupLocations((*locations)[:0], stacktraceID)
	for _, locationID := range *locations {
		if locationID == 0 || int(locationID) > len(symbols.Locations) {
			continue
		}
		location := symbols.Locations[locationID-1]
		for _, line := range location.Line {
			functionID := line.FunctionId
			if functionID == 0 || int(functionID) > len(symbols.Functions) {
				continue
			}
			function := symbols.Functions[functionID-1]
			if function.Name == 0 || int(function.Name) >= len(symbols.Strings) {
				continue
			}
			if fn(symbols.Strings[function.Name]) {
				return
			}
		}
	}
}

func (r SymbolBloomIndexRow) add(symbol string) {
	h1, h2 := symbolBloomHashes(symbol)
	for i := uint32(0); i < r.BloomHashCount; i++ {
		bit := uint32((h1 + uint64(i)*h2) % uint64(r.BloomBitCount))
		r.BloomBits[bit/8] |= 1 << (bit % 8)
	}
}

func bloomSizing(n int, falsePositiveRate float64) (bits, hashes uint32) {
	if n <= 0 {
		return 0, 0
	}
	m := -float64(n) * math.Log(falsePositiveRate) / (math.Ln2 * math.Ln2)
	if m < symbolBloomMinBits {
		m = symbolBloomMinBits
	}
	bits = uint32(math.Ceil(m))
	bits = (bits + 7) &^ 7
	k := uint32(math.Round(float64(bits) / float64(n) * math.Ln2))
	if k < 1 {
		k = 1
	}
	if k > symbolBloomMaxHashCount {
		k = symbolBloomMaxHashCount
	}
	return bits, k
}

func symbolBloomHashes(symbol string) (uint64, uint64) {
	h1 := xxhash.Sum64String(symbol)
	h2 := xxhash.Sum64String(symbolBloomHashSeed + symbol)
	if h2 == 0 {
		h2 = 1
	}
	return h1, h2
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return append([]string(nil), values...)
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func cloneSymbolBloomIndexRow(r SymbolBloomIndexRow) SymbolBloomIndexRow {
	r.BloomBits = append([]byte(nil), r.BloomBits...)
	return r
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.n += int64(n)
	return n, err
}
