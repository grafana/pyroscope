package symbolizer

import (
	"bytes"
	"context"
	"debug/elf"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	pprof "github.com/google/pprof/profile"
	"github.com/prometheus/client_golang/prometheus"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/lidia"
	"github.com/grafana/pyroscope/pkg/objstore"
)

type DebuginfodClient interface {
	FetchDebuginfo(ctx context.Context, buildID string) (io.ReadCloser, error)
}

type Config struct {
	Enabled                  bool                 `yaml:"enabled"`
	DebuginfodURL            string               `yaml:"debuginfod_url"`
	PersistentDebugInfoStore DebugInfoStoreConfig `yaml:"persistent_debuginfo_store"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&cfg.Enabled, "symbolizer.enabled", false, "Enable symbolization for unsymbolized profiles")
	f.StringVar(&cfg.DebuginfodURL, "symbolizer.debuginfod-url", "https://debuginfod.elfutils.org", "URL of the debuginfod server")
	f.DurationVar(&cfg.PersistentDebugInfoStore.MaxAge, "symbolizer.persistent-debuginfo-store.max-age", 7*24*time.Hour, "Maximum age of stored debug info")
	cfg.PersistentDebugInfoStore.Storage.RegisterFlagsWithPrefix("symbolizer.persistent-debuginfo-store.storage.", f)
}

type Symbolizer struct {
	logger  log.Logger
	client  DebuginfodClient
	store   DebugInfoStore
	metrics *metrics
}

func New(logger log.Logger, cfg Config, reg prometheus.Registerer, bucket objstore.Bucket) (*Symbolizer, error) {
	metrics := newMetrics(reg)

	store := NewObjstoreDebugInfoStore(bucket, cfg.PersistentDebugInfoStore.MaxAge, metrics)

	client, err := NewDebuginfodClient(logger, cfg.DebuginfodURL, metrics)
	if err != nil {
		return nil, err
	}

	return &Symbolizer{
		logger:  logger,
		client:  client,
		store:   store,
		metrics: metrics,
	}, nil
}

func (s *Symbolizer) SymbolizePprof(ctx context.Context, profile *googlev1.Profile) error {
	start := time.Now()
	status := statusSuccess
	defer func() {
		s.metrics.profileSymbolization.WithLabelValues(status).Observe(time.Since(start).Seconds())
	}()

	mappingsToSymbolize := make(map[uint64]bool)
	for i, mapping := range profile.Mapping {
		if mapping.HasFunctions && mapping.HasFilenames && mapping.HasLineNumbers {
			continue
		}
		mappingsToSymbolize[uint64(i+1)] = true
	}
	if len(mappingsToSymbolize) == 0 {
		return nil
	}

	locationsByMapping, err := s.groupLocationsByMapping(profile, mappingsToSymbolize)
	if err != nil {
		return fmt.Errorf("grouping locations by mapping: %w", err)
	}

	for mappingID, locations := range locationsByMapping {
		if err := s.symbolizeLocationsForMapping(ctx, profile, mappingID, locations); err != nil {
			return fmt.Errorf("failed to symbolize mapping ID %d: %w", mappingID, err)
		}
	}

	return nil
}

// groupLocationsByMapping groups locations by their mapping ID
func (s *Symbolizer) groupLocationsByMapping(profile *googlev1.Profile, mappingsToSymbolize map[uint64]bool) (map[uint64][]locToSymbolize, error) {
	locsByMapping := make(map[uint64][]locToSymbolize)

	for i, loc := range profile.Location {
		if loc.MappingId == 0 {
			return nil, fmt.Errorf("invalid profile: location at index %d has MappingId 0", i)
		}

		mappingIdx := loc.MappingId - 1
		if int(mappingIdx) >= len(profile.Mapping) {
			return nil, fmt.Errorf("invalid profile: location at index %d references non-existent mapping %d", i, loc.MappingId)
		}

		if !mappingsToSymbolize[loc.MappingId] {
			continue
		}

		locsByMapping[loc.MappingId] = append(locsByMapping[loc.MappingId], locToSymbolize{
			idx: i,
			loc: loc,
		})
	}

	return locsByMapping, nil
}

// symbolizeLocationsForMapping symbolizes a single mapping group
func (s *Symbolizer) symbolizeLocationsForMapping(ctx context.Context, profile *googlev1.Profile, mappingID uint64, locs []locToSymbolize) error {
	mapping := profile.Mapping[mappingID-1]

	binaryName, err := s.extractBinaryName(profile, mapping)
	if err != nil {
		return err
	}

	buildID, err := s.extractBuildID(profile, mapping)
	if err != nil {
		return err
	}
	if buildID == "" {
		return nil
	}

	req := s.createSymbolizationRequest(binaryName, buildID, mapping, locs)

	if err := s.Symbolize(ctx, &req); err != nil {
		return err
	}

	s.updateProfileWithSymbols(profile, mapping, locs, req.Locations)

	return nil
}

// extractBinaryName extracts the binary name from the mapping
func (s *Symbolizer) extractBinaryName(profile *googlev1.Profile, mapping *googlev1.Mapping) (string, error) {
	if mapping.Filename < 0 || int(mapping.Filename) >= len(profile.StringTable) {
		return "", fmt.Errorf("invalid mapping: filename index %d out of range (string table length: %d)",
			mapping.Filename, len(profile.StringTable))
	}

	fullPath := profile.StringTable[mapping.Filename]
	return filepath.Base(fullPath), nil
}

// extractBuildID extracts and sanitizes the build ID from the mapping
func (s *Symbolizer) extractBuildID(profile *googlev1.Profile, mapping *googlev1.Mapping) (string, error) {
	buildID := profile.StringTable[mapping.BuildId]
	sanitizedBuildID, err := sanitizeBuildID(buildID)
	if err != nil {
		level.Error(s.logger).Log("msg", "Invalid buildID", "buildID", buildID)
		return "", err
	}

	return sanitizedBuildID, nil
}

// createSymbolizationRequest creates a symbolization request for a mapping group
func (s *Symbolizer) createSymbolizationRequest(binaryName, buildID string, mapping *googlev1.Mapping, locs []locToSymbolize) Request {
	req := Request{
		BuildID:    buildID,
		BinaryName: binaryName,
		Locations:  make([]*Location, len(locs)),
	}

	for i, loc := range locs {
		req.Locations[i] = &Location{
			Address: loc.loc.Address,
			Mapping: &pprof.Mapping{
				Start:   mapping.MemoryStart,
				Limit:   mapping.MemoryLimit,
				Offset:  mapping.FileOffset,
				BuildID: buildID,
			},
		}
	}

	return req
}

// updateProfileWithSymbols updates the profile with symbolization results
func (s *Symbolizer) updateProfileWithSymbols(profile *googlev1.Profile, mapping *googlev1.Mapping, locs []locToSymbolize, symLocs []*Location) {
	stringMap := make(map[string]int64, len(profile.StringTable))
	for i, str := range profile.StringTable {
		stringMap[str] = int64(i)
	}

	type funcKey struct {
		nameIdx, filenameIdx int64
	}
	funcMap := make(map[funcKey]uint64)
	maxFuncID := uint64(0)

	for _, fn := range profile.Function {
		if fn.Id > maxFuncID {
			maxFuncID = fn.Id
		}
		funcMap[funcKey{fn.Name, fn.Filename}] = fn.Id
	}

	for i, symLoc := range symLocs {
		locIdx := locs[i].idx
		profile.Location[locIdx].Line = make([]*googlev1.Line, len(symLoc.Lines))

		for j, line := range symLoc.Lines {
			nameIdx, ok := stringMap[line.FunctionName]
			if !ok {
				nameIdx = int64(len(profile.StringTable))
				profile.StringTable = append(profile.StringTable, line.FunctionName)
				stringMap[line.FunctionName] = nameIdx
			}

			filenameIdx, ok := stringMap[line.FilePath]
			if !ok {
				filenameIdx = int64(len(profile.StringTable))
				profile.StringTable = append(profile.StringTable, line.FilePath)
				stringMap[line.FilePath] = filenameIdx
			}

			key := funcKey{nameIdx, filenameIdx}
			funcID, ok := funcMap[key]
			if !ok {
				maxFuncID++
				funcID = maxFuncID
				profile.Function = append(profile.Function, &googlev1.Function{
					Id:        funcID,
					Name:      nameIdx,
					Filename:  filenameIdx,
					StartLine: int64(line.LineNumber),
				})
				funcMap[key] = funcID
			}

			profile.Location[locIdx].Line[j] = &googlev1.Line{
				FunctionId: funcID,
				Line:       int64(line.LineNumber),
			}
		}
	}

	mapping.HasFunctions = true
	mapping.HasFilenames = true
}

func (s *Symbolizer) Symbolize(ctx context.Context, req *Request) error {
	start := time.Now()
	status := statusSuccess
	defer func() {
		s.metrics.profileSymbolization.WithLabelValues(status).Observe(time.Since(start).Seconds())
	}()

	if s.checkObjectStoreCache(ctx, req) {
		return nil
	}

	return s.fetchAndCacheFromDebuginfod(ctx, req, &status)
}

func (s *Symbolizer) symbolizeWithTable(_ context.Context, table *lidia.Table, req *Request) {
	var framesBuf []lidia.SourceInfoFrame

	resolveStart := time.Now()
	defer func() {
		s.metrics.debugSymbolResolution.WithLabelValues(statusSuccess).Observe(time.Since(resolveStart).Seconds())
	}()

	for _, loc := range req.Locations {
		frames, err := table.Lookup(framesBuf, loc.Address)
		if err != nil {
			loc.Lines = s.createNotFoundSymbols(req.BinaryName, loc)
			continue
		}

		if len(frames) == 0 {
			loc.Lines = s.createNotFoundSymbols(req.BinaryName, loc)
			continue
		}

		loc.Lines = frames
	}
}

// checkObjectStoreCache checks if the debug info is in the object store cache
func (s *Symbolizer) checkObjectStoreCache(ctx context.Context, req *Request) bool {
	objstoreReader, err := s.store.Get(ctx, req.BuildID)
	if err != nil {
		level.Error(s.logger).Log(
			"msg", "failed to get from object store",
			"build_id", req.BuildID,
			"error", err,
		)
		return false
	}
	defer objstoreReader.Close()

	err = s.symbolizeFromReader(ctx, objstoreReader, req)
	if err != nil {
		level.Error(s.logger).Log(
			"msg", "failed to symbolize from object store data",
			"build_id", req.BuildID,
			"error", err,
		)
		return false
	}

	return true
}

// handleDebuginfodError handles errors from the debuginfod client
func (s *Symbolizer) handleDebuginfodError(err error, req *Request, debuginfodStart time.Time, status *string) error {
	var bnfErr buildIDNotFoundError
	statusCode, isHTTPError := isHTTPStatusError(err)

	if errors.As(err, &bnfErr) || (isHTTPError && statusCode == http.StatusNotFound) {
		*status = statusErrorNotFound
		s.metrics.debuginfodRequestDuration.WithLabelValues(statusErrorNotFound).Observe(time.Since(debuginfodStart).Seconds())

		level.Info(s.logger).Log("msg", "Build ID not found, caching placeholder symbols",
			"build_id", req.BuildID,
			"binary", req.BinaryName)

		for _, loc := range req.Locations {
			loc.Lines = s.createNotFoundSymbols(req.BinaryName, loc)
		}

		// TODO: Cache the placeholder symbols for 404s?

		return nil
	}

	*status = statusErrorDebuginfod
	s.metrics.debuginfodRequestDuration.WithLabelValues(statusErrorDebuginfod).Observe(time.Since(debuginfodStart).Seconds())
	s.metrics.debugSymbolResolution.WithLabelValues(statusErrorDebuginfod).Observe(0)
	return fmt.Errorf("fetch debuginfo: %w", err)
}

func (s *Symbolizer) fetchAndCacheFromDebuginfod(ctx context.Context, req *Request, status *string) error {
	if s.client == nil {
		*status = statusErrorDebuginfod
		return fmt.Errorf("no debuginfod client configured")
	}

	debuginfodStart := time.Now()
	debugReader, err := s.client.FetchDebuginfo(ctx, req.BuildID)
	if err != nil {
		return s.handleDebuginfodError(err, req, debuginfodStart, status)
	}

	s.metrics.debuginfodRequestDuration.WithLabelValues(statusSuccess).Observe(time.Since(debuginfodStart).Seconds())
	defer debugReader.Close()

	elfData, err := io.ReadAll(debugReader)
	if err != nil {
		return fmt.Errorf("read debuginfod data: %w", err)
	}

	lidiaData, err := s.processELFData(elfData)
	if err != nil {
		level.Error(s.logger).Log(
			"msg", "symbolizer: Failed to process ELF data",
			"build_id", req.BuildID,
			"error", err,
		)
		return err
	}

	lidiaReader := NewReaderAtCloser(lidiaData)
	table, err := lidia.OpenReader(lidiaReader, lidia.WithCRC())
	if err != nil {
		s.metrics.debugSymbolResolution.WithLabelValues("lidia_error").Observe(0)
		level.Error(s.logger).Log("msg", "opening Lidia table", "err", err)
		return fmt.Errorf("open lidia file: %w", err)
	}

	s.symbolizeWithTable(ctx, table, req)

	table.Close()

	s.metrics.debuginfodFileSize.Observe(float64(len(lidiaData)))

	if err := s.store.Put(ctx, req.BuildID, bytes.NewReader(lidiaData)); err != nil {
		level.Warn(s.logger).Log("msg", "Failed to store debug info in objstore", "buildID", req.BuildID, "err", err)
	}

	return nil
}

func (s *Symbolizer) symbolizeFromReader(ctx context.Context, r io.ReadCloser, req *Request) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read content: %w", err)
	}

	lidiaReader := NewReaderAtCloser(data)

	table, err := lidia.OpenReader(lidiaReader, lidia.WithCRC())
	if err != nil {
		s.metrics.debugSymbolResolution.WithLabelValues("lidia_error").Observe(0)
		level.Error(s.logger).Log("msg", "opening Lidia table", "err", err)
		return fmt.Errorf("open lidia file: %w", err)
	}
	defer table.Close()

	s.symbolizeWithTable(ctx, table, req)

	return nil
}

func (s *Symbolizer) processELFData(data []byte) (lidiaData []byte, err error) {
	reader, err := detectCompression(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("detect compression: %w", err)
	}

	sr := io.NewSectionReader(reader, 0, 1<<63-1)
	elfFile, err := elf.NewFile(sr)
	if err != nil {
		return nil, fmt.Errorf("parse ELF file: %w", err)
	}
	defer elfFile.Close()

	initialSize := len(data) * 2 // A simple heuristic: twice the compressed size
	memBuffer := newMemoryBuffer(initialSize)

	err = lidia.CreateLidiaFromELF(elfFile, memBuffer, lidia.WithCRC(), lidia.WithFiles(), lidia.WithLines())
	if err != nil {
		return nil, fmt.Errorf("create lidia file: %w", err)
	}

	return memBuffer.Bytes(), nil
}

func (s *Symbolizer) createNotFoundSymbols(binaryName string, loc *Location) []lidia.SourceInfoFrame {
	prefix := "unknown"
	if binaryName != "" {
		prefix = binaryName
	}

	return []lidia.SourceInfoFrame{{
		FunctionName: fmt.Sprintf("%s!0x%x", prefix, loc.Address),
		LineNumber:   0,
	}}
}
