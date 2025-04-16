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
	"strings"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	pprof "github.com/google/pprof/profile"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/prometheus/client_golang/prometheus"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/lidia"
	"github.com/grafana/pyroscope/pkg/objstore"
)

const (
	DefaultInMemorySymbolCacheSize     = 100_000
	DefaultInMemoryLidiaTableCacheSize = 2 << 30 // 2GB default
)

type DebuginfodClient interface {
	FetchDebuginfo(ctx context.Context, buildID string) (io.ReadCloser, error)
}

type Config struct {
	Enabled                     bool                 `yaml:"enabled"`
	DebuginfodURL               string               `yaml:"debuginfod_url"`
	InMemorySymbolCacheSize     int                  `yaml:"in_memory_symbol_cache_size"`
	InMemoryLidiaTableCacheSize int                  `yaml:"in_memory_lidia_table_cache_size"`
	PersistentDebugInfoStore    DebugInfoStoreConfig `yaml:"persistent_debuginfo_store"`
}

// ProfileSymbolizer implements Symbolizer
type ProfileSymbolizer struct {
	logger  log.Logger
	client  DebuginfodClient
	store   DebugInfoStore
	metrics *Metrics

	symbolCache     *lru.Cache[string, []lidia.SourceInfoFrame]
	lidiaTableCache *ristretto.Cache
}

func NewFromConfig(_ context.Context, logger log.Logger, cfg Config, reg prometheus.Registerer, bucket objstore.Bucket) (*ProfileSymbolizer, error) {
	metrics := NewMetrics(reg)

	if bucket == nil {
		return nil, fmt.Errorf("storage bucket is required for symbolizer")
	}

	prefixedBucket := objstore.NewPrefixedBucket(bucket, "symbolizer")

	store := NewObjstoreDebugInfoStore(prefixedBucket, cfg.PersistentDebugInfoStore.MaxAge, metrics)

	client, err := NewDebuginfodClient(logger, cfg.DebuginfodURL, metrics)
	if err != nil {
		return nil, err
	}

	// Use configured cache sizes or defaults
	symbolCacheSize := DefaultInMemorySymbolCacheSize
	if cfg.InMemorySymbolCacheSize > 0 {
		symbolCacheSize = cfg.InMemorySymbolCacheSize
	}

	lidiaTableCacheSize := DefaultInMemoryLidiaTableCacheSize
	if cfg.InMemoryLidiaTableCacheSize > 0 {
		lidiaTableCacheSize = cfg.InMemoryLidiaTableCacheSize
	}

	return NewProfileSymbolizer(logger, client, store, metrics, symbolCacheSize, lidiaTableCacheSize)
}

func NewProfileSymbolizer(logger log.Logger, client DebuginfodClient, store DebugInfoStore, metrics *Metrics, symbolCacheSize int, lidiaTableCacheSize int) (*ProfileSymbolizer, error) {
	symbolCache, err := lru.New[string, []lidia.SourceInfoFrame](symbolCacheSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create symbol cache: %w", err)
	}

	// Create Ristretto cache for lidia tables
	lidiaTableCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,                        // number of keys to track frequency of (10M)
		MaxCost:     int64(lidiaTableCacheSize), // maximum cost of cache
		BufferItems: 64,                         // number of keys per Get buffer
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create debug info cache: %w", err)
	}

	return &ProfileSymbolizer{
		logger:          logger,
		client:          client,
		store:           store,
		metrics:         metrics,
		symbolCache:     symbolCache,
		lidiaTableCache: lidiaTableCache,
	}, nil
}

func (s *ProfileSymbolizer) SymbolizePprof(ctx context.Context, profile *googlev1.Profile) error {
	level.Info(s.logger).Log("msg", ">> starting SymbolizePprof")
	start := time.Now()
	status := StatusSuccess
	defer func() {
		s.metrics.profileSymbolization.WithLabelValues(status).Observe(time.Since(start).Seconds())
	}()

	if !s.needsSymbolization(profile) {
		status = "already_symbolized"
		return nil
	}

	// Group locations by mapping ID
	locsByMapping := s.groupLocationsByMapping(profile)

	// Process each mapping group
	errs := s.symbolizeMappingGroups(ctx, profile, locsByMapping)

	// Handle errors
	if len(errs) > 0 {
		status = StatusErrorOther
		return fmt.Errorf("symbolization errors: %v", errs)
	}

	return nil
}

// needsSymbolization checks if the profile needs symbolization at all
func (s *ProfileSymbolizer) needsSymbolization(profile *googlev1.Profile) bool {
	if profile == nil || len(profile.Mapping) == 0 {
		level.Info(s.logger).Log("msg", "profile is either nil or has no mappings, skipping symbolization")
		return false
	}
	return true
}

// groupLocationsByMapping groups locations by their mapping ID
func (s *ProfileSymbolizer) groupLocationsByMapping(profile *googlev1.Profile) map[uint64][]locToSymbolize {
	locsByMapping := make(map[uint64][]locToSymbolize)

	for i, loc := range profile.Location {
		if loc.MappingId > 0 {
			mapping := profile.Mapping[loc.MappingId-1]
			needsSymbolization := true

			// If the mapping claims to have symbols, validate this specific location
			if mapping.HasFunctions && len(loc.Line) > 0 {
				needsSymbolization = false
			}

			// Only add locations that need symbolization
			if needsSymbolization {
				locsByMapping[loc.MappingId] = append(locsByMapping[loc.MappingId], locToSymbolize{
					idx: i,
					loc: loc,
				})
			}
		}
	}

	return locsByMapping
}

// symbolizeMappingGroups processes each mapping group and symbolizes its locations
func (s *ProfileSymbolizer) symbolizeMappingGroups(ctx context.Context, profile *googlev1.Profile, locsByMapping map[uint64][]locToSymbolize) []error {
	var errs []error

	// Process each mapping group
	for mappingID, locs := range locsByMapping {
		if err := s.symbolizeMappingGroup(ctx, profile, mappingID, locs); err != nil {
			errs = append(errs, fmt.Errorf("symbolizer symbolize mapping ID %d: %w", mappingID, err))
		}
	}

	return errs
}

// symbolizeMappingGroup symbolizes a single mapping group
func (s *ProfileSymbolizer) symbolizeMappingGroup(ctx context.Context, profile *googlev1.Profile, mappingID uint64, locs []locToSymbolize) error {
	mapping := profile.Mapping[mappingID-1]

	// Skip if mapping already has symbols
	if mapping.HasFunctions && mapping.HasFilenames && mapping.HasLineNumbers {
		return nil
	}

	binaryName := s.extractBinaryName(profile, mapping)
	buildID, err := s.extractBuildID(profile, mapping)
	if err != nil {
		return err
	}

	// Create symbolization request for this mapping group
	req := s.createSymbolizationRequest(binaryName, buildID, mapping, locs)

	if err := s.Symbolize(ctx, &req); err != nil {
		return err
	}

	// Store symbolization results back into profile
	s.updateProfileWithSymbols(profile, mapping, locs, req.Locations)

	return nil
}

// extractBinaryName extracts the binary name from the mapping
func (s *ProfileSymbolizer) extractBinaryName(profile *googlev1.Profile, mapping *googlev1.Mapping) string {
	binaryName := "unknown"
	if mapping.Filename >= 0 && int(mapping.Filename) < len(profile.StringTable) {
		fullPath := profile.StringTable[mapping.Filename]
		if lastSlash := strings.LastIndex(fullPath, "/"); lastSlash >= 0 {
			binaryName = fullPath[lastSlash+1:]
		} else {
			binaryName = fullPath
		}
	}
	return binaryName
}

// extractBuildID extracts and sanitizes the build ID from the mapping
func (s *ProfileSymbolizer) extractBuildID(profile *googlev1.Profile, mapping *googlev1.Mapping) (string, error) {
	buildID := profile.StringTable[mapping.BuildId]
	sanitizedBuildID, err := sanitizeBuildID(buildID)
	if err != nil {
		level.Error(s.logger).Log("msg", "Invalid buildID", "buildID", buildID)
		return "", err
	}

	level.Info(s.logger).Log("msg", ">> mapping build id --> ", "mapping.BuildId", mapping.BuildId)
	level.Info(s.logger).Log("msg", ">> build id --> ", "buildID", sanitizedBuildID)

	return sanitizedBuildID, nil
}

// createSymbolizationRequest creates a symbolization request for a mapping group
func (s *ProfileSymbolizer) createSymbolizationRequest(binaryName, buildID string, mapping *googlev1.Mapping, locs []locToSymbolize) Request {
	req := Request{
		BuildID:    buildID,
		BinaryName: binaryName,
		Locations:  make([]*Location, len(locs)),
	}

	// Prepare locations for symbolization
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
func (s *ProfileSymbolizer) updateProfileWithSymbols(profile *googlev1.Profile, mapping *googlev1.Mapping, locs []locToSymbolize, symLocs []*Location) {
	for i, symLoc := range symLocs {
		if len(symLoc.Lines) > 0 {
			locIdx := locs[i].idx
			profile.Location[locIdx].Line = make([]*googlev1.Line, len(symLoc.Lines))

			for j, line := range symLoc.Lines {
				// Create or find function name in string table
				nameIdx := s.findOrAddString(profile, line.FunctionName)
				filenameIdx := s.findOrAddString(profile, line.FilePath)

				// Create or find function
				funcId := s.findOrAddFunction(profile, nameIdx, filenameIdx, line.LineNumber)

				profile.Location[locIdx].Line[j] = &googlev1.Line{
					FunctionId: funcId,
					Line:       int64(line.LineNumber),
				}
			}
		}
	}

	mapping.HasFunctions = true
	mapping.HasFilenames = true
	mapping.HasLineNumbers = true
}

// findOrAddString finds a string in the string table or adds it if not found
func (s *ProfileSymbolizer) findOrAddString(profile *googlev1.Profile, str string) int64 {
	for i, s := range profile.StringTable {
		if s == str {
			return int64(i)
		}
	}

	idx := int64(len(profile.StringTable))
	profile.StringTable = append(profile.StringTable, str)
	return idx
}

// findOrAddFunction finds a function in the function table or adds it if not found
func (s *ProfileSymbolizer) findOrAddFunction(profile *googlev1.Profile, nameIdx, filenameIdx int64, lineNumber uint64) uint64 {
	maxFuncId := uint64(0)
	for _, f := range profile.Function {
		maxFuncId = max(maxFuncId, f.Id)
		if f.Name == nameIdx && f.Filename == filenameIdx {
			return f.Id
		}
	}

	funcId := maxFuncId + 1
	profile.Function = append(profile.Function, &googlev1.Function{
		Id:        funcId,
		Name:      nameIdx,
		Filename:  filenameIdx,
		StartLine: int64(lineNumber),
	})

	return funcId
}

func (s *ProfileSymbolizer) Symbolize(ctx context.Context, req *Request) error {
	start := time.Now()
	status := StatusSuccess
	defer func() {
		s.metrics.profileSymbolization.WithLabelValues(status).Observe(time.Since(start).Seconds())
	}()

	if s.checkSymbolCache(req) {
		return nil
	}

	if s.checkLidiaTableCache(ctx, req) {
		return nil
	}

	if s.checkObjectStoreCache(ctx, req) {
		return nil
	}

	// Fetch from debuginfod as last resort
	return s.fetchAndCacheFromDebuginfod(ctx, req, &status)
}

// symbolizeWithTable processes all locations in a request using a lidia table
func (s *ProfileSymbolizer) symbolizeWithTable(_ context.Context, table *lidia.Table, ei *BinaryLayout, req *Request) error {
	// Buffer for reusing in symbol lookups
	var framesBuf []lidia.SourceInfoFrame

	for _, loc := range req.Locations {
		resolveStart := time.Now()
		addr, err := MapRuntimeAddress(loc.Address, ei, Mapping{
			Start:  loc.Mapping.Start,
			Limit:  loc.Mapping.Limit,
			Offset: loc.Mapping.Offset,
		})
		if err != nil {
			return fmt.Errorf("normalize address: %w", err)
		}

		// Look up the address directly in the lidia table
		frames, err := table.Lookup(framesBuf, addr)
		if err != nil {
			level.Error(s.logger).Log(
				"msg", "Failed to resolve address on Lidia table lookup",
				"addr", fmt.Sprintf("0x%x", addr),
				"binary", req.BinaryName,
				"build_id", req.BuildID,
				"error", err,
			)
			loc.Lines = s.createNotFoundSymbols(req.BinaryName, loc, addr)
			s.metrics.debugSymbolResolution.WithLabelValues(StatusErrorServerError).Observe(time.Since(resolveStart).Seconds())
			continue
		}

		if len(frames) == 0 {
			level.Debug(s.logger).Log(
				"msg", "No symbols found for address",
				"addr", fmt.Sprintf("0x%x", addr),
				"binary", req.BinaryName,
				"build_id", req.BuildID,
			)
			loc.Lines = s.createNotFoundSymbols(req.BinaryName, loc, addr)
			s.metrics.debugSymbolResolution.WithLabelValues(StatusErrorOther).Observe(time.Since(resolveStart).Seconds())
			continue
		}

		loc.Lines = frames
		s.metrics.debugSymbolResolution.WithLabelValues(StatusSuccess).Observe(time.Since(resolveStart).Seconds())
	}

	return nil
}

// checkSymbolCache checks if all addresses are in the symbol cache
func (s *ProfileSymbolizer) checkSymbolCache(req *Request) bool {
	if s.symbolCache == nil {
		return false
	}

	allCached := true
	for _, loc := range req.Locations {
		cacheKey := s.createSymbolCacheKey(req.BuildID, loc.Address)
		symbolStart := time.Now()
		symbolStatus := StatusCacheMiss

		if frames, found := s.symbolCache.Get(cacheKey); found {
			loc.Lines = frames
			symbolStatus = StatusCacheHit
			s.metrics.debugSymbolResolution.WithLabelValues(StatusCacheHit).Observe(time.Since(symbolStart).Seconds())
		} else {
			allCached = false
			break
		}

		s.metrics.cacheOperations.WithLabelValues("symbol_cache", "get", symbolStatus).Observe(time.Since(symbolStart).Seconds())
	}

	return allCached
}

// checkLidiaTableCache checks if the debug info is in the Lidia table cache
func (s *ProfileSymbolizer) checkLidiaTableCache(ctx context.Context, req *Request) bool {
	lidiaTableStart := time.Now()
	lidiaTableStatus := StatusCacheMiss

	if s.lidiaTableCache == nil {
		return false
	}

	if data, found := s.lidiaTableCache.Get(req.BuildID); found {
		lidiaTableStatus = StatusCacheHit

		// Since this is not in production yet, we can assume the cached data is always a LidiaTableCacheEntry
		entry := data.(LidiaTableCacheEntry)
		dataBytes := entry.Data
		ei := entry.EI
		level.Debug(s.logger).Log("msg", "Using cached BinaryLayout", "buildID", req.BuildID)

		lidiaReader := &memoryReader{
			bs:  dataBytes,
			off: 0,
		}

		// Open the Lidia table
		table, err := lidia.OpenReader(lidiaReader, lidia.WithCRC())
		if err != nil {
			s.metrics.debugSymbolResolution.WithLabelValues("lidia_error").Observe(0)
			return false
		}
		defer table.Close()

		// Use the BinaryLayout from the cache or the one we just extracted
		err = s.symbolizeWithTable(ctx, table, ei, req)
		if err != nil {
			return false
		}

		s.metrics.cacheOperations.WithLabelValues("lidia_table_cache", "get", lidiaTableStatus).Observe(time.Since(lidiaTableStart).Seconds())

		s.updateSymbolCache(req)
		return true
	}

	s.metrics.cacheOperations.WithLabelValues("lidia_table_cache", "get", lidiaTableStatus).Observe(time.Since(lidiaTableStart).Seconds())
	return false
}

// checkObjectStoreCache checks if the debug info is in the object store cache
func (s *ProfileSymbolizer) checkObjectStoreCache(ctx context.Context, req *Request) bool {
	if s.store == nil {
		return false
	}

	objstoreStart := time.Now()
	objstoreStatus := StatusCacheMiss
	objstoreReader, err := s.store.Get(ctx, req.BuildID)

	if err != nil {
		s.metrics.cacheOperations.WithLabelValues("objstore_cache", "get", objstoreStatus).Observe(time.Since(objstoreStart).Seconds())
		return false
	}

	objstoreStatus = StatusCacheHit
	s.metrics.cacheOperations.WithLabelValues("objstore_cache", "get", objstoreStatus).Observe(time.Since(objstoreStart).Seconds())
	defer objstoreReader.Close()

	// Read the entire content to store in Ristretto cache
	var buf bytes.Buffer
	teeReader := io.TeeReader(objstoreReader, &buf)
	err = s.symbolizeFromReader(ctx, io.NopCloser(teeReader), req)
	if err != nil {
		return false
	}

	// Update in ristretto cache
	if s.lidiaTableCache != nil {
		data := buf.Bytes()
		cacheStart := time.Now()
		if err := s.storeLidiaTableInCache(data, req.BuildID, cacheStart); err != nil {
			return false
		}
	}

	s.updateSymbolCache(req)
	return true
}

// fetchAndCacheFromDebuginfod fetches debug info from debuginfod and caches it
func (s *ProfileSymbolizer) fetchAndCacheFromDebuginfod(ctx context.Context, req *Request, status *string) error {
	// Try to get from debuginfod client
	if s.client == nil {
		*status = StatusErrorDebuginfod
		return fmt.Errorf("no debuginfod client configured")
	}

	debuginfodStart := time.Now()
	debugReader, err := s.client.FetchDebuginfo(ctx, req.BuildID)
	if err != nil {
		return s.handleDebuginfodError(err, req, debuginfodStart, status)
	}

	s.metrics.debuginfodRequestDuration.WithLabelValues(StatusSuccess).Observe(time.Since(debuginfodStart).Seconds())
	defer debugReader.Close()

	// Read the entire content to store in caches
	var buf bytes.Buffer
	teeReader := io.TeeReader(debugReader, &buf)

	err = s.symbolizeFromReader(ctx, io.NopCloser(teeReader), req)
	if err != nil {
		return err
	}

	// Update caches
	data := buf.Bytes()
	s.metrics.debuginfodFileSize.Observe(float64(len(data)))
	s.updateSymbolCache(req)

	// Update all caches with the fetched data
	return s.updateAllCaches(ctx, req, data)
}

// handleDebuginfodError handles errors from the debuginfod client
func (s *ProfileSymbolizer) handleDebuginfodError(err error, req *Request, debuginfodStart time.Time, status *string) error {
	var bnfErr buildIDNotFoundError
	statusCode, isHTTPError := isHTTPStatusError(err)

	if errors.As(err, &bnfErr) || (isHTTPError && statusCode == http.StatusNotFound) {
		*status = StatusErrorNotFound
		s.metrics.debuginfodRequestDuration.WithLabelValues(StatusErrorNotFound).Observe(time.Since(debuginfodStart).Seconds())

		level.Info(s.logger).Log("msg", "Build ID not found, caching placeholder symbols",
			"build_id", req.BuildID,
			"binary", req.BinaryName)

		cacheStart := time.Now()
		for _, loc := range req.Locations {
			symbols := s.createNotFoundSymbols(req.BinaryName, loc, loc.Address)
			if s.symbolCache != nil {
				cacheKey := s.createSymbolCacheKey(req.BuildID, loc.Address)
				s.symbolCache.Add(cacheKey, symbols)
			}
			loc.Lines = symbols
		}

		if s.symbolCache != nil {
			s.metrics.cacheOperations.WithLabelValues("symbol_cache", "set", StatusErrorNotFound).Observe(time.Since(cacheStart).Seconds())
		}

		return nil
	}

	*status = StatusErrorDebuginfod
	s.metrics.debuginfodRequestDuration.WithLabelValues(StatusErrorDebuginfod).Observe(time.Since(debuginfodStart).Seconds())
	s.metrics.debugSymbolResolution.WithLabelValues(StatusErrorDebuginfod).Observe(0)
	return fmt.Errorf("fetch debuginfo: %w", err)
}

// updateAllCaches updates all caches with the fetched data
func (s *ProfileSymbolizer) updateAllCaches(ctx context.Context, req *Request, data []byte) error {
	// Store in Ristretto cache
	if s.lidiaTableCache != nil {
		cacheStart := time.Now()
		if err := s.storeLidiaTableInCache(data, req.BuildID, cacheStart); err != nil {
			return err
		}
	}

	// Store in ObjstoreCache
	if s.store != nil {
		cacheStart := time.Now()
		if cacheErr := s.store.Put(ctx, req.BuildID, bytes.NewReader(data)); cacheErr != nil {
			s.metrics.cacheOperations.WithLabelValues("objstore_cache", "put", StatusErrorUpload).Observe(time.Since(cacheStart).Seconds())
			level.Warn(s.logger).Log("msg", "Failed to store debug info in objstore", "buildID", req.BuildID, "err", cacheErr)
		} else {
			s.metrics.cacheOperations.WithLabelValues("objstore_cache", "put", StatusSuccess).Observe(time.Since(cacheStart).Seconds())
			s.metrics.cacheSizeBytes.WithLabelValues("objstore_cache").Add(float64(len(data)))
		}
	}

	return nil
}

func (s *ProfileSymbolizer) symbolizeFromReader(ctx context.Context, r io.ReadCloser, req *Request) error {
	// Read the entire content into memory
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read content: %w", err)
	}

	// Process the ELF data
	lidiaData, ei, err := s.processELFData(data)
	if err != nil {
		// Handle error or create mock symbols if needed
		level.Debug(s.logger).Log("msg", "Failed to process ELF data, creating mock symbols", "err", err)
		for _, loc := range req.Locations {
			loc.Lines = []lidia.SourceInfoFrame{{
				FunctionName: fmt.Sprintf("%s!0x%x", req.BinaryName, loc.Address),
				FilePath:     fmt.Sprintf("unknown_file_0x%x", loc.Address),
				LineNumber:   0,
			}}
		}
		return nil
	}

	// Create a reader for the Lidia table
	lidiaReader := &memoryReader{
		bs:  lidiaData,
		off: 0,
	}

	// Open the lidia table from memory
	table, err := lidia.OpenReader(lidiaReader, lidia.WithCRC())
	if err != nil {
		s.metrics.debugSymbolResolution.WithLabelValues("lidia_error").Observe(0)
		level.Error(s.logger).Log("msg", "Error opening Lidia table", "err", err)
		return fmt.Errorf("open lidia file: %w", err)
	}
	defer table.Close()

	return s.symbolizeWithTable(ctx, table, ei, req)
}

func (s *ProfileSymbolizer) storeLidiaTableInCache(data []byte, buildID string, cacheStart time.Time) error {
	// Process the ELF data
	lidiaData, ei, err := s.processELFData(data)
	if err != nil {
		return err
	}

	// Store both the processed Lidia table data and the BinaryLayout in the cache
	entry := LidiaTableCacheEntry{
		Data: lidiaData,
		EI:   ei,
	}

	success := s.lidiaTableCache.Set(buildID, entry, int64(len(lidiaData)))
	s.metrics.cacheOperations.WithLabelValues("debuginfo_cache", "set", StatusSuccess).Observe(time.Since(cacheStart).Seconds())
	if !success {
		level.Warn(s.logger).Log("msg", "Failed to store debug info in cache", "buildID", buildID, "size", len(lidiaData))
	}
	s.metrics.cacheSizeBytes.WithLabelValues("debuginfo_cache").Set(float64(len(lidiaData)))

	return nil
}

func (s *ProfileSymbolizer) processELFData(data []byte) (lidiaData []byte, ei *BinaryLayout, err error) {
	// Create a reader from the data
	reader, err := detectCompression(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("detect compression: %w", err)
	}

	// Parse the ELF file
	sr := io.NewSectionReader(reader, 0, 1<<63-1)
	elfFile, err := elf.NewFile(sr)
	if err != nil {
		return nil, nil, fmt.Errorf("parse ELF file: %w", err)
	}
	defer elfFile.Close()

	// Get executable info for address normalization
	ei, err = ExecutableInfoFromELF(elfFile)
	if err != nil {
		return nil, nil, fmt.Errorf("executable info from ELF: %w", err)
	}

	// Create an in-memory buffer for the lidia format
	initialSize := len(data) * 2 // A simple heuristic: twice the compressed size
	memBuffer := newMemoryBuffer(initialSize)

	// Create lidia file from ELF directly in memory
	err = lidia.CreateLidiaFromELF(elfFile, memBuffer, lidia.WithCRC(), lidia.WithFiles(), lidia.WithLines())
	if err != nil {
		return nil, nil, fmt.Errorf("create lidia file: %w", err)
	}

	return memBuffer.Bytes(), ei, nil
}

// updateSymbolCache updates the symbol cache with newly symbolized addresses
func (s *ProfileSymbolizer) updateSymbolCache(req *Request) {
	if s.symbolCache == nil {
		return
	}

	for _, loc := range req.Locations {
		if len(loc.Lines) > 0 {
			cacheKey := s.createSymbolCacheKey(req.BuildID, loc.Address)
			cacheStart := time.Now()
			s.symbolCache.Add(cacheKey, loc.Lines)
			s.metrics.cacheOperations.WithLabelValues("symbol_cache", "set", StatusSuccess).Observe(time.Since(cacheStart).Seconds())
		}
	}
}

func (s *ProfileSymbolizer) createNotFoundSymbols(binaryName string, loc *Location, addr uint64) []lidia.SourceInfoFrame {
	prefix := "unknown"
	if binaryName != "" {
		prefix = binaryName
	}

	return []lidia.SourceInfoFrame{{
		FunctionName: fmt.Sprintf("%s!0x%x", prefix, loc.Address),
		FilePath:     fmt.Sprintf("mapped_0x%x", addr),
		LineNumber:   0,
	}}
}

func (s *ProfileSymbolizer) createSymbolCacheKey(buildID string, address uint64) string {
	return fmt.Sprintf("%s:%x", buildID, address)
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	// TODO: change me back to false!
	f.BoolVar(&cfg.Enabled, "symbolizer.enabled", true, "Enable symbolization for unsymbolized profiles")
	f.StringVar(&cfg.DebuginfodURL, "symbolizer.debuginfod-url", "https://debuginfod.elfutils.org", "URL of the debuginfod server")
	f.IntVar(&cfg.InMemorySymbolCacheSize, "symbolizer.in-memory-symbol-cache-size", DefaultInMemorySymbolCacheSize, "Maximum number of entries in the in-memory symbol cache")
	f.IntVar(&cfg.InMemoryLidiaTableCacheSize, "symbolizer.in-memory-debuginfo-cache-size", DefaultInMemoryLidiaTableCacheSize, "Maximum size in bytes for the in-memory debug info cache")
	f.DurationVar(&cfg.PersistentDebugInfoStore.MaxAge, "symbolizer.persistent-debuginfo-store.max-age", 7*24*time.Hour, "Maximum age of stored debug info")
	cfg.PersistentDebugInfoStore.Storage.RegisterFlagsWithPrefix("symbolizer.persistent-debuginfo-store.storage.", f)
}
