package symbolizer

import (
	"bytes"
	"context"
	"debug/dwarf"
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
	"github.com/grafana/pyroscope/pkg/objstore"
)

const (
	DefaultInMemorySymbolCacheSize    = 100_000
	DefaultInMemoryDebuginfoCacheSize = 2 << 30 // 2GB default
)

type DebuginfodClient interface {
	FetchDebuginfo(ctx context.Context, buildID string) (io.ReadCloser, error)
}

type Config struct {
	Enabled                    bool                 `yaml:"enabled"`
	DebuginfodURL              string               `yaml:"debuginfod_url"`
	InMemorySymbolCacheSize    int                  `yaml:"in_memory_symbol_cache_size"`
	InMemoryDebuginfoCacheSize int                  `yaml:"in_memory_debuginfo_cache_size"`
	PersistentDebugInfoStore   DebugInfoStoreConfig `yaml:"persistent_debuginfo_store"`
}

// ProfileSymbolizer implements Symbolizer
type ProfileSymbolizer struct {
	logger  log.Logger
	client  DebuginfodClient
	store   DebugInfoStore
	metrics *Metrics

	symbolCache    *lru.Cache[string, []SymbolLocation]
	debugInfoCache *ristretto.Cache
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

	debuginfoCacheSize := DefaultInMemoryDebuginfoCacheSize
	if cfg.InMemoryDebuginfoCacheSize > 0 {
		debuginfoCacheSize = cfg.InMemoryDebuginfoCacheSize
	}

	return NewProfileSymbolizer(logger, client, store, metrics, symbolCacheSize, debuginfoCacheSize)
}

func NewProfileSymbolizer(logger log.Logger, client DebuginfodClient, store DebugInfoStore, metrics *Metrics, symbolCacheSize int, debuginfoCacheSize int) (*ProfileSymbolizer, error) {
	symbolCache, err := lru.New[string, []SymbolLocation](symbolCacheSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create symbol cache: %w", err)
	}

	// Create Ristretto cache for debug info
	debugInfoCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,                       // number of keys to track frequency of (10M)
		MaxCost:     int64(debuginfoCacheSize), // maximum cost of cache
		BufferItems: 64,                        // number of keys per Get buffer
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create debug info cache: %w", err)
	}

	return &ProfileSymbolizer{
		logger:         logger,
		client:         client,
		store:          store,
		metrics:        metrics,
		symbolCache:    symbolCache,
		debugInfoCache: debugInfoCache,
	}, nil
}

func (s *ProfileSymbolizer) SymbolizePprof(ctx context.Context, profile *googlev1.Profile) error {
	level.Info(s.logger).Log("msg", ">> starting SymbolizePprof")
	start := time.Now()
	status := StatusSuccess
	defer func() {
		s.metrics.profileSymbolization.WithLabelValues(status).Observe(time.Since(start).Seconds())
	}()

	if profile == nil || len(profile.Mapping) == 0 {
		level.Info(s.logger).Log("msg", "profile is either nil or has no mappings, skipping symbolization")
		status = "already_symbolized"
		return nil
	}

	// if !s.NeedsSymbolization(profile) {
	// 	level.Info(s.logger).Log("msg", ">> don't need symbolization")
	// 	status = "already_symbolized"
	// 	level.Error(s.logger).Log("msg", "Symbolizer exited since profile don't need symbolization")
	// 	return nil
	// }

	// Group locations by mapping ID
	type locToSymbolize struct {
		idx int
		loc *googlev1.Location
	}
	locsByMapping := make(map[uint64][]locToSymbolize)

	for i, loc := range profile.Location {
		if loc.MappingId > 0 {
			mapping := profile.Mapping[loc.MappingId-1]
			needsSymbolization := true

			// If the mapping claims to have symbols, validate this specific location
			if mapping.HasFunctions {
				if mapping.HasFunctions && len(loc.Line) > 0 {
					needsSymbolization = false
				}
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

	var errs []error
	// Process each mapping group
	for mappingID, locs := range locsByMapping {
		mapping := profile.Mapping[mappingID-1]

		// Skip if mapping already has symbols
		if mapping.HasFunctions && mapping.HasFilenames && mapping.HasLineNumbers {
			continue
		}

		binaryName := "unknown"
		if mapping.Filename >= 0 && int(mapping.Filename) < len(profile.StringTable) {
			fullPath := profile.StringTable[mapping.Filename]
			if lastSlash := strings.LastIndex(fullPath, "/"); lastSlash >= 0 {
				binaryName = fullPath[lastSlash+1:]
			} else {
				binaryName = fullPath
			}
		}

		buildID := profile.StringTable[mapping.BuildId]
		level.Info(s.logger).Log("msg", ">> mapping build id --> ", "mapping.BuildId", mapping.BuildId)
		level.Info(s.logger).Log("msg", ">> build id --> ", "buildID", buildID)
		buildID, err := sanitizeBuildID(buildID)
		if err != nil {
			status = StatusErrorInvalidID
			level.Error(s.logger).Log("msg", "Invalid buildID", "buildID", buildID)
			continue
		}

		// Create symbolization request for this mapping group
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

		if err := s.Symbolize(ctx, &req); err != nil {
			errs = append(errs, fmt.Errorf("symbolizer symbolize mapping ID %d: %w", mappingID, err))
			continue
		}

		// Store symbolization results back into profile
		for i, symLoc := range req.Locations {
			if len(symLoc.Lines) > 0 {
				locIdx := locs[i].idx
				profile.Location[locIdx].Line = make([]*googlev1.Line, len(symLoc.Lines))

				for j, line := range symLoc.Lines {
					// Create or find function name in string table
					nameIdx := int64(-1)
					filenameIdx := int64(-1)
					maxFuncId := uint64(0)
					for i, s := range profile.StringTable {
						if s == line.Function.Name {
							nameIdx = int64(i)
						}
						if s == line.Function.Filename {
							filenameIdx = int64(i)
						}
					}
					if nameIdx == -1 {
						nameIdx = int64(len(profile.StringTable))
						profile.StringTable = append(profile.StringTable, line.Function.Name)
					}
					if filenameIdx == -1 {
						filenameIdx = int64(len(profile.StringTable))
						profile.StringTable = append(profile.StringTable, line.Function.Filename)
					}

					// Create or find function
					funcId := uint64(0)
					for _, f := range profile.Function {
						maxFuncId = max(maxFuncId, f.Id)
						if f.Name == nameIdx && f.Filename == filenameIdx {
							funcId = f.Id
							break
						}
					}
					if funcId == 0 {
						funcId = maxFuncId + 1
						profile.Function = append(profile.Function, &googlev1.Function{
							Id:        funcId,
							Name:      nameIdx,
							Filename:  filenameIdx,
							StartLine: line.Function.StartLine,
						})
					}

					profile.Location[locIdx].Line[j] = &googlev1.Line{
						FunctionId: funcId,
						Line:       line.Line,
					}
				}
			}
		}

		mapping.HasFunctions = true
		mapping.HasFilenames = true
		mapping.HasLineNumbers = true
	}

	if len(errs) > 0 {
		status = StatusErrorOther
		s.metrics.profileSymbolization.WithLabelValues(status).Observe(time.Since(start).Seconds())
		return fmt.Errorf("symbolization errors: %v", errs)
	}

	return nil
}

func (s *ProfileSymbolizer) Symbolize(ctx context.Context, req *Request) error {
	start := time.Now()
	status := StatusSuccess
	defer func() {
		s.metrics.profileSymbolization.WithLabelValues(status).Observe(time.Since(start).Seconds())
	}()

	// Check if we need to fetch debug info at all
	if s.symbolCache != nil {
		allCached := true
		for _, loc := range req.Locations {
			cacheKey := s.createSymbolCacheKey(req.BuildID, loc.Address)
			symbolStart := time.Now()
			symbolStatus := StatusCacheMiss
			if symbols, found := s.symbolCache.Get(cacheKey); found {
				loc.Lines = symbols
				symbolStatus = StatusCacheHit
				s.metrics.debugSymbolResolution.WithLabelValues(StatusCacheHit).Observe(time.Since(symbolStart).Seconds())
			} else {
				allCached = false
				break
			}
			s.metrics.cacheOperations.WithLabelValues("symbol_cache", "get", symbolStatus).Observe(time.Since(symbolStart).Seconds())
		}

		// If all addresses were in cache, we're done
		if allCached {
			return nil
		}
	}

	// Check in-memory Ristretto cache for debug info
	debugInfoStart := time.Now()
	debugInfoStatus := StatusCacheMiss
	if s.debugInfoCache != nil {
		if data, found := s.debugInfoCache.Get(req.BuildID); found {
			debugInfoStatus = StatusCacheHit
			debugReader := io.NopCloser(bytes.NewReader(data.([]byte)))
			err := s.symbolizeFromReader(ctx, debugReader, req)
			if err != nil {
				return err
			}

			s.metrics.cacheOperations.WithLabelValues("debuginfo_cache", "get", debugInfoStatus).Observe(time.Since(debugInfoStart).Seconds())

			s.updateSymbolCache(req)
			return nil
		}
		s.metrics.cacheOperations.WithLabelValues("debuginfo_cache", "get", debugInfoStatus).Observe(time.Since(debugInfoStart).Seconds())
	}

	// Try to get from ObjstoreCache
	if s.store != nil {
		objstoreStart := time.Now()
		objstoreStatus := StatusCacheMiss
		objstoreReader, err := s.store.Get(ctx, req.BuildID)
		if err == nil {
			objstoreStatus = StatusCacheHit
			s.metrics.cacheOperations.WithLabelValues("objstore_cache", "get", objstoreStatus).Observe(time.Since(objstoreStart).Seconds())
			defer objstoreReader.Close()

			// Read the entire content to store in Ristretto cache
			var buf bytes.Buffer
			teeReader := io.TeeReader(objstoreReader, &buf)
			err := s.symbolizeFromReader(ctx, io.NopCloser(teeReader), req)
			if err != nil {
				return err
			}

			// Update in ristretto cache
			if s.debugInfoCache != nil {
				data := buf.Bytes()
				cacheStart := time.Now()
				success := s.debugInfoCache.Set(req.BuildID, data, int64(len(data)))
				s.metrics.cacheOperations.WithLabelValues("debuginfo_cache", "set", StatusSuccess).Observe(time.Since(cacheStart).Seconds())
				if !success {
					level.Warn(s.logger).Log("msg", "Failed to store debug info in cache", "buildID", req.BuildID, "size", len(data))
				}
				s.metrics.cacheSizeBytes.WithLabelValues("debuginfo_cache").Set(float64(len(data)))
			}

			s.updateSymbolCache(req)
			return nil
		} else {
			s.metrics.cacheOperations.WithLabelValues("objstore_cache", "get", objstoreStatus).Observe(time.Since(objstoreStart).Seconds())
		}
	}

	// Try to get from debuginfod client
	if s.client == nil {
		status = StatusErrorDebuginfod
		return fmt.Errorf("no debuginfod client configured")
	}

	debuginfodStart := time.Now()
	debugReader, err := s.client.FetchDebuginfo(ctx, req.BuildID)
	if err != nil {
		var bnfErr buildIDNotFoundError
		statusCode, isHTTPError := isHTTPStatusError(err)
		if errors.As(err, &bnfErr) || (isHTTPError && statusCode == http.StatusNotFound) {
			status = StatusErrorNotFound
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

		status = StatusErrorDebuginfod
		s.metrics.debuginfodRequestDuration.WithLabelValues(StatusErrorDebuginfod).Observe(time.Since(debuginfodStart).Seconds())
		s.metrics.debugSymbolResolution.WithLabelValues(StatusErrorDebuginfod).Observe(0)
		return fmt.Errorf("fetch debuginfo: %w", err)
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

	// Store in Ristretto cache
	if s.debugInfoCache != nil {
		cacheStart := time.Now()
		success := s.debugInfoCache.Set(req.BuildID, data, int64(len(data)))
		s.metrics.cacheOperations.WithLabelValues("debuginfo_cache", "set", StatusSuccess).Observe(time.Since(cacheStart).Seconds())
		if !success {
			level.Warn(s.logger).Log("msg", "Failed to store debug info in cache", "buildID", req.BuildID, "size", len(data))
		}
		s.metrics.cacheSizeBytes.WithLabelValues("debuginfo_cache").Set(float64(len(data)))
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
	var buf [512]byte
	n, err := io.ReadFull(r, buf[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return fmt.Errorf("read header: %w", err)
	}

	// Auto-detect compression
	combined := io.MultiReader(bytes.NewReader(buf[:n]), r)
	reader, err := detectCompression(combined)
	if err != nil {
		return fmt.Errorf("detect compression: %w", err)
	}

	sr := io.NewSectionReader(reader, 0, 1<<63-1)
	elfFile, err := elf.NewFile(sr)
	if err != nil {
		s.metrics.debugSymbolResolution.WithLabelValues("elf_error").Observe(0)
		return fmt.Errorf("create ELF file from reader: %w", err)
	}
	defer elfFile.Close()

	// Get executable info for address normalization
	ei, err := ExecutableInfoFromELF(elfFile)
	if err != nil {
		s.metrics.debugSymbolResolution.WithLabelValues("elf_info_error").Observe(0)
		return fmt.Errorf("executable info from ELF: %w", err)
	}

	// Create liner
	liner, err := NewDwarfResolver(elfFile)
	if err != nil {
		s.metrics.debugSymbolResolution.WithLabelValues("dwarf_error").Observe(0)
		return fmt.Errorf("create DWARF resolver: %w", err)
	}

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

		// Get source lines for the address
		lines, err := liner.ResolveAddress(ctx, addr)
		if err != nil {
			level.Error(s.logger).Log(
				"msg", "Failed to resolve address",
				"addr", fmt.Sprintf("0x%x", addr),
				"binary", req.BinaryName,
				"build_id", req.BuildID,
				"mapping_start", fmt.Sprintf("0x%x", loc.Mapping.Start),
				"mapping_limit", fmt.Sprintf("0x%x", loc.Mapping.Limit),
				"mapping_offset", fmt.Sprintf("0x%x", loc.Mapping.Offset),
				"normalized_addr", fmt.Sprintf("0x%x", addr),
				"error", err,
			)
			loc.Lines = s.createNotFoundSymbols(req.BinaryName, loc, addr)
			s.metrics.debugSymbolResolution.WithLabelValues(StatusErrorServerError).Observe(time.Since(resolveStart).Seconds())
			continue
		}

		loc.Lines = lines
		s.metrics.debugSymbolResolution.WithLabelValues(StatusSuccess).Observe(time.Since(resolveStart).Seconds())
	}

	return nil
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

// NeedsSymbolization checks if a profile needs symbolization.
// It returns true if any mapping in the profile needs symbolization.
// This function also marks mappings with JSON build IDs as already symbolized.
func (s *ProfileSymbolizer) NeedsSymbolization(profile *googlev1.Profile) bool {
	if profile == nil || len(profile.Mapping) == 0 {
		level.Info(s.logger).Log("msg", "Symbolize check: Profile has no mappings, skipping symbolization")
		return false
	}

	for i, mapping := range profile.Mapping {
		if mapping == nil {
			level.Info(s.logger).Log("msg", "Symbolize check: Skipping nil mapping")
			continue
		}

		// Skip if mapping already has symbols
		if mapping.HasFunctions && mapping.HasFilenames && mapping.HasLineNumbers {
			level.Info(s.logger).Log("msg", "Symbolize check: Mapping already has symbols",
				"mapping_index", i,
				"has_functions", mapping.HasFunctions,
				"has_filenames", mapping.HasFilenames,
				"has_line_numbers", mapping.HasLineNumbers)
			continue
		}

		// Check if build ID is valid
		if mapping.BuildId < 0 || mapping.BuildId >= int64(len(profile.StringTable)) {
			level.Info(s.logger).Log("msg", "Symbolize check: Invalid build ID index",
				"mapping_index", i,
				"build_id", mapping.BuildId)
			continue
		}

		buildID := profile.StringTable[mapping.BuildId]

		sanitizedBuildID, err := sanitizeBuildID(buildID)
		if err != nil {
			level.Warn(s.logger).Log("msg", "Symbolize check: BuildID sanitization failed, marking as already symbolized",
				"mapping_index", i,
				"build_id", buildID,
				"error", err)
			continue
		}

		// If sanitization resulted in an empty build ID, skip this mapping
		if sanitizedBuildID == "" {
			level.Warn(s.logger).Log("msg", "Symbolize check: Empty sanitized build ID, skipping mapping",
				"mapping_index", i,
				"original_build_id", buildID)
			continue
		}

		return true
	}

	return false
}

func (s *ProfileSymbolizer) createNotFoundSymbols(binaryName string, loc *Location, addr uint64) []SymbolLocation {
	prefix := "unknown"
	if binaryName != "" {
		prefix = binaryName
	}

	return []SymbolLocation{{
		Function: &pprof.Function{
			Name:      fmt.Sprintf("%s!0x%x", prefix, loc.Address),
			Filename:  fmt.Sprintf("mapped_0x%x", addr),
			StartLine: 0,
		},
		Line: 0,
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
	f.IntVar(&cfg.InMemoryDebuginfoCacheSize, "symbolizer.in-memory-debuginfo-cache-size", DefaultInMemoryDebuginfoCacheSize, "Maximum size in bytes for the in-memory debug info cache")
	f.DurationVar(&cfg.PersistentDebugInfoStore.MaxAge, "symbolizer.persistent-debuginfo-store.max-age", 7*24*time.Hour, "Maximum age of stored debug info")
	cfg.PersistentDebugInfoStore.Storage.RegisterFlagsWithPrefix("symbolizer.persistent-debuginfo-store.storage.", f)
}

type DwarfResolver struct {
	debugData *dwarf.Data
	dbgFile   *DWARFInfo
	file      *elf.File
}

func NewDwarfResolver(f *elf.File) (SymbolResolver, error) {
	debugData, err := f.DWARF()
	if err != nil {
		return nil, fmt.Errorf("read DWARF data: %w", err)
	}

	debugInfo := NewDWARFInfo(debugData)

	return &DwarfResolver{
		debugData: debugData,
		dbgFile:   debugInfo,
		file:      f,
	}, nil
}

func (d *DwarfResolver) ResolveAddress(ctx context.Context, pc uint64) ([]SymbolLocation, error) {
	return d.dbgFile.ResolveAddress(ctx, pc)
}

func (d *DwarfResolver) Close() error {
	return d.file.Close()
}
