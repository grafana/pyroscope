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
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/lidia"
	"github.com/grafana/pyroscope/pkg/objstore"
)

type DebuginfodClient interface {
	FetchDebuginfo(ctx context.Context, buildID string) (io.ReadCloser, error)
}

type Config struct {
	DebuginfodURL            string `yaml:"debuginfod_url"`
	MaxDebuginfodConcurrency int    `yaml:"max_debuginfod_concurrency" category:"advanced"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&cfg.DebuginfodURL, "symbolizer.debuginfod-url", "https://debuginfod.elfutils.org", "URL of the debuginfod server")
	f.IntVar(&cfg.MaxDebuginfodConcurrency, "symbolizer.max-debuginfod-concurrency", 10, "Maximum number of concurrent symbolization requests to debuginfod server.")
}

func (cfg *Config) Validate() error {
	if cfg.MaxDebuginfodConcurrency < 1 {
		return fmt.Errorf("invalid max-debuginfod-concurrency value, must be positive")
	}
	return nil
}

type Symbolizer struct {
	logger  log.Logger
	client  DebuginfodClient
	bucket  objstore.Bucket
	metrics *metrics
	cfg     Config
}

func New(logger log.Logger, cfg Config, reg prometheus.Registerer, bucket objstore.Bucket) (*Symbolizer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	metrics := newMetrics(reg)

	client, err := NewDebuginfodClient(logger, cfg.DebuginfodURL, metrics)
	if err != nil {
		return nil, err
	}

	return &Symbolizer{
		logger:  logger,
		client:  client,
		bucket:  bucket,
		metrics: metrics,
		cfg:     cfg,
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
		if mapping.HasFunctions {
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

	stringMap := make(map[string]int64, len(profile.StringTable))
	for i, str := range profile.StringTable {
		stringMap[str] = int64(i)
	}

	allSymbolizedLocs, err := s.symbolizeMappingsConcurrently(ctx, profile, locationsByMapping)
	if err != nil {
		return fmt.Errorf("symbolizing mappings: %w", err)
	}

	s.updateAllSymbolsInProfile(profile, allSymbolizedLocs, stringMap)

	return nil
}

// symbolizeMappingsConcurrently symbolizes multiple mappings concurrently with a concurrency limit.
func (s *Symbolizer) symbolizeMappingsConcurrently(
	ctx context.Context,
	profile *googlev1.Profile,
	locationsByMapping map[uint64][]*googlev1.Location,
) ([]symbolizedLocation, error) {
	maxConcurrency := s.cfg.MaxDebuginfodConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 10
	}

	type mappingJob struct {
		mappingID uint64
		locations []*googlev1.Location
	}

	type mappingResult struct {
		mappingID uint64
		locations []symbolizedLocation
	}

	totalLocs := 0
	jobs := make(chan mappingJob, len(locationsByMapping))
	for mappingID, locations := range locationsByMapping {
		totalLocs += len(locations)
		jobs <- mappingJob{mappingID: mappingID, locations: locations}
	}
	close(jobs)

	// Process jobs concurrently with errgroup for proper error handling
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrency)

	// Results channel with buffer to avoid blocking jobs
	results := make(chan mappingResult, len(locationsByMapping))

	for job := range jobs {
		job := job
		g.Go(func() error {
			mapping := profile.Mapping[job.mappingID-1]

			binaryName, err := s.extractBinaryName(profile, mapping)
			if err != nil {
				return fmt.Errorf("extract binary name for mapping %d: %w", job.mappingID, err)
			}

			buildID, err := s.extractBuildID(profile, mapping)
			if err != nil {
				return fmt.Errorf("extract build ID for mapping %d: %w", job.mappingID, err)
			}

			req := s.createSymbolizationRequest(binaryName, buildID, job.locations)
			s.symbolize(ctx, &req)

			// Collect symbolized locations for this mapping
			symbolizedLocs := make([]symbolizedLocation, len(job.locations))
			for i, loc := range job.locations {
				symbolizedLocs[i] = symbolizedLocation{
					loc:     loc,
					symLoc:  req.locations[i],
					mapping: mapping,
				}
			}

			select {
			case results <- mappingResult{mappingID: job.mappingID, locations: symbolizedLocs}:
			case <-ctx.Done():
				return ctx.Err()
			}

			return nil
		})
	}

	err := g.Wait()
	close(results)

	if err != nil {
		return nil, err
	}

	allSymbolizedLocs := make([]symbolizedLocation, 0, totalLocs)
	for result := range results {
		allSymbolizedLocs = append(allSymbolizedLocs, result.locations...)
	}

	return allSymbolizedLocs, nil
}

// groupLocationsByMapping groups locations by their mapping ID
func (s *Symbolizer) groupLocationsByMapping(profile *googlev1.Profile, mappingsToSymbolize map[uint64]bool) (map[uint64][]*googlev1.Location, error) {
	locsByMapping := make(map[uint64][]*googlev1.Location)

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

		// Skip locations that already have symbols
		if len(loc.Line) > 0 {
			continue
		}

		locsByMapping[loc.MappingId] = append(locsByMapping[loc.MappingId], loc)
	}

	return locsByMapping, nil
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
func (s *Symbolizer) createSymbolizationRequest(binaryName, buildID string, locs []*googlev1.Location) request {
	req := request{
		buildID:    buildID,
		binaryName: binaryName,
		locations:  make([]*location, len(locs)),
	}

	for i, loc := range locs {
		req.locations[i] = &location{
			address: loc.Address,
		}
	}

	return req
}

func (s *Symbolizer) updateAllSymbolsInProfile(
	profile *googlev1.Profile,
	symbolizedLocs []symbolizedLocation,
	stringMap map[string]int64,
) {
	funcMap := make(map[funcKey]uint64)
	maxFuncID := uint64(len(profile.Function))
	funcPtrMap := make(map[uint64]*googlev1.Function)

	for _, item := range symbolizedLocs {
		loc := item.loc
		symLoc := item.symLoc
		mapping := item.mapping

		locIdx := loc.Id - 1
		if loc.Id <= 0 || locIdx >= uint64(len(profile.Location)) {
			continue
		}

		profile.Location[locIdx].Line = make([]*googlev1.Line, len(symLoc.lines))

		for j, line := range symLoc.lines {
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
				fn := &googlev1.Function{
					Id:        funcID,
					Name:      nameIdx,
					Filename:  filenameIdx,
					StartLine: int64(line.LineNumber),
				}
				profile.Function = append(profile.Function, fn)
				funcMap[key] = funcID
				funcPtrMap[funcID] = fn
			} else {
				// Update StartLine to be the minimum line number seen for this function
				if line.LineNumber > 0 {
					if fn, ok := funcPtrMap[funcID]; ok {
						currentStartLine := fn.StartLine
						// 0 means "not set" in proto
						if currentStartLine == 0 || int64(line.LineNumber) < currentStartLine {
							fn.StartLine = int64(line.LineNumber)
						}
					}
				}
			}

			profile.Location[locIdx].Line[j] = &googlev1.Line{
				FunctionId: funcID,
				Line:       int64(line.LineNumber),
			}
		}

		mapping.HasFunctions = true
	}
}

func (s *Symbolizer) symbolize(ctx context.Context, req *request) {
	if req.buildID == "" {
		s.metrics.debugSymbolResolutionErrors.WithLabelValues("empty_build_id").Inc()
		s.setFallbackSymbols(req)
		return
	}

	lidiaBytes, err := s.getLidiaBytes(ctx, req.buildID)
	if err != nil {
		level.Warn(s.logger).Log("msg", "Failed to get debug info", "buildID", req.buildID, "err", err)
		s.setFallbackSymbols(req)
		return
	}

	lidiaReader := NewReaderAtCloser(lidiaBytes)
	table, err := lidia.OpenReader(lidiaReader, lidia.WithCRC())
	if err != nil {
		s.metrics.debugSymbolResolutionErrors.WithLabelValues("lidia_error").Inc()
		level.Warn(s.logger).Log("msg", "Failed to open Lidia file", "err", err)
		s.setFallbackSymbols(req)
		return
	}
	defer table.Close()

	s.symbolizeWithTable(table, req)
}

// setFallbackSymbols sets fallback symbols for all locations in the request
func (s *Symbolizer) setFallbackSymbols(req *request) {
	for _, loc := range req.locations {
		loc.lines = s.createFallbackSymbol(req.binaryName, loc)
	}
}

func (s *Symbolizer) symbolizeWithTable(table *lidia.Table, req *request) {
	var framesBuf []lidia.SourceInfoFrame

	resolveStart := time.Now()
	defer func() {
		s.metrics.debugSymbolResolution.WithLabelValues(statusSuccess).Observe(time.Since(resolveStart).Seconds())
	}()

	for _, loc := range req.locations {
		frames, err := table.Lookup(framesBuf, loc.address)
		if err != nil {
			loc.lines = s.createFallbackSymbol(req.binaryName, loc)
			continue
		}

		if len(frames) == 0 {
			loc.lines = s.createFallbackSymbol(req.binaryName, loc)
			continue
		}

		loc.lines = frames
	}
}

func (s *Symbolizer) getLidiaBytes(ctx context.Context, buildID string) ([]byte, error) {
	if client, ok := s.client.(*DebuginfodHTTPClient); ok {
		if sanitizedBuildID, err := sanitizeBuildID(buildID); err == nil {
			if found, _ := client.notFoundCache.Get(sanitizedBuildID); found {
				s.metrics.cacheOperations.WithLabelValues("not_found", "get", statusSuccess).Inc()
				return nil, buildIDNotFoundError{buildID: buildID}
			}
		}
	}

	lidiaBytes, err := s.fetchLidiaFromObjectStore(ctx, buildID)
	if err == nil {
		s.metrics.cacheOperations.WithLabelValues("object_storage", "get", statusSuccess).Inc()
		return lidiaBytes, nil
	}
	s.metrics.cacheOperations.WithLabelValues("object_storage", "get", "miss").Inc()

	lidiaBytes, err = s.fetchLidiaFromDebuginfod(ctx, buildID)
	if err != nil {
		return nil, err
	}

	if err := s.bucket.Upload(ctx, buildID, bytes.NewReader(lidiaBytes)); err != nil {
		level.Warn(s.logger).Log("msg", "Failed to store debug info in objstore", "buildID", buildID, "err", err)
		s.metrics.cacheOperations.WithLabelValues("object_storage", "set", "error").Inc()
	} else {
		s.metrics.cacheOperations.WithLabelValues("object_storage", "set", statusSuccess).Inc()
	}

	return lidiaBytes, nil
}

// fetchLidiaFromObjectStore retrieves Lidia data from the object store
func (s *Symbolizer) fetchLidiaFromObjectStore(ctx context.Context, buildID string) ([]byte, error) {
	objstoreReader, err := s.bucket.Get(ctx, buildID)
	if err != nil {
		return nil, err
	}
	defer objstoreReader.Close()

	data, err := io.ReadAll(objstoreReader)
	if err != nil {
		return nil, fmt.Errorf("read content: %w", err)
	}

	return data, nil
}

// fetchLidiaFromDebuginfod fetches debug info from debuginfod and converts to Lidia format
func (s *Symbolizer) fetchLidiaFromDebuginfod(ctx context.Context, buildID string) ([]byte, error) {
	debugReader, err := s.fetchFromDebuginfod(ctx, buildID)
	if err != nil {
		var bnfErr buildIDNotFoundError
		if errors.As(err, &bnfErr) {
			return nil, err
		}
		return nil, err
	}
	defer debugReader.Close()

	elfData, err := io.ReadAll(debugReader)
	if err != nil {
		return nil, fmt.Errorf("read debuginfod data: %w", err)
	}

	lidiaData, err := s.processELFData(elfData)
	if err != nil {
		return nil, err
	}

	return lidiaData, nil
}

func (s *Symbolizer) fetchFromDebuginfod(ctx context.Context, buildID string) (io.ReadCloser, error) {
	debugReader, err := s.client.FetchDebuginfo(ctx, buildID)
	if err != nil {
		var bnfErr buildIDNotFoundError
		statusCode, isHTTPError := isHTTPStatusError(err)

		if errors.As(err, &bnfErr) || (isHTTPError && statusCode == http.StatusNotFound) {
			return nil, buildIDNotFoundError{buildID: buildID}
		}

		return nil, fmt.Errorf("fetch debuginfo: %w", err)
	}

	return debugReader, nil
}

func (s *Symbolizer) processELFData(data []byte) (lidiaData []byte, err error) {
	decompressedData, err := detectCompression(data)
	if err != nil {
		s.metrics.debugSymbolResolutionErrors.WithLabelValues("compression_error").Inc()
		return nil, fmt.Errorf("detect compression: %w", err)
	}

	reader := bytes.NewReader(decompressedData)

	elfFile, err := elf.NewFile(reader)
	if err != nil {
		s.metrics.debugSymbolResolutionErrors.WithLabelValues("elf_parsing_error").Inc()
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

func (s *Symbolizer) createFallbackSymbol(binaryName string, loc *location) []lidia.SourceInfoFrame {
	prefix := "unknown"
	if binaryName != "" {
		prefix = binaryName
	}

	return []lidia.SourceInfoFrame{{
		FunctionName: fmt.Sprintf("%s!0x%x", prefix, loc.address),
		LineNumber:   0,
	}}
}
