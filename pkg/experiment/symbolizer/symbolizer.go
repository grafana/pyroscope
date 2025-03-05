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
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/dgraph-io/ristretto"
	pprof "github.com/google/pprof/profile"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/prometheus/client_golang/prometheus"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	objstoreclient "github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/util"
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

func NewFromConfig(ctx context.Context, logger log.Logger, cfg Config, reg prometheus.Registerer) (*ProfileSymbolizer, error) {
	var store DebugInfoStore = NewNullDebugInfoStore()

	metrics := NewMetrics(reg)

	if cfg.PersistentDebugInfoStore.Enabled {
		if cfg.PersistentDebugInfoStore.Storage.Backend == "" {
			return nil, fmt.Errorf("storage configuration required when persistent debug info store is enabled")
		}
		bucket, err := objstoreclient.NewBucket(ctx, cfg.PersistentDebugInfoStore.Storage, "debuginfo")
		if err != nil {
			return nil, fmt.Errorf("create debug info storage: %w", err)
		}
		store = NewObjstoreDebugInfoStore(bucket, cfg.PersistentDebugInfoStore.MaxAge, metrics)
	}

	client, err := NewDebuginfodClient(cfg.DebuginfodURL, metrics)
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
	if store == nil {
		store = NewNullDebugInfoStore()
	}

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
	start := time.Now()
	status := StatusSuccess
	defer func() {
		s.metrics.profileSymbolization.WithLabelValues(status).Observe(time.Since(start).Seconds())
	}()

	// Group locations by mapping ID
	type locToSymbolize struct {
		idx int
		loc *googlev1.Location
	}
	locsByMapping := make(map[uint64][]locToSymbolize)

	for i, loc := range profile.Location {
		if loc.MappingId > 0 {
			// Validate symbols if mapping claims to have them
			if mapping := profile.Mapping[loc.MappingId-1]; mapping.HasFunctions {
				if !hasValidSymbols(loc, profile) {
					// Reset flags if validation fails
					mapping.HasFunctions = false
					mapping.HasFilenames = false
					mapping.HasLineNumbers = false
					continue
				}
			}

			locsByMapping[loc.MappingId] = append(locsByMapping[loc.MappingId], locToSymbolize{
				idx: i,
				loc: loc,
			})
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

		buildID := profile.StringTable[mapping.BuildId]
		if buildID == "" {
			continue
		}

		// Create symbolization request for this mapping group
		req := Request{
			BuildID:   buildID,
			Locations: make([]*Location, len(locs)),
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
			errs = append(errs, fmt.Errorf("symbolize mapping ID %d: %w", mappingID, err))
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
					funcIdx := -1
					for k, f := range profile.Function {
						if f.Name == nameIdx && f.Filename == filenameIdx {
							funcIdx = k
							break
						}
					}
					if funcIdx == -1 {
						funcIdx = len(profile.Function)
						profile.Function = append(profile.Function, &googlev1.Function{
							Name:      nameIdx,
							Filename:  filenameIdx,
							StartLine: line.Function.StartLine,
						})
					}

					profile.Location[locIdx].Line[j] = &googlev1.Line{
						FunctionId: uint64(funcIdx),
						Line:       line.Line,
					}
				}
			}
		}

		// Update mapping flags
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
			cacheKey := fmt.Sprintf("%s:%x", req.BuildID, loc.Address)
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
	if s.store != nil && s.store.IsEnabled() {
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
			return fmt.Errorf("resolve address: %w", err)
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
			cacheKey := fmt.Sprintf("%s:%x", req.BuildID, loc.Address)
			cacheStart := time.Now()
			s.symbolCache.Add(cacheKey, loc.Lines)
			s.metrics.cacheOperations.WithLabelValues("symbol_cache", "set", StatusSuccess).Observe(time.Since(cacheStart).Seconds())
		}
	}
}

func hasValidSymbols(loc *googlev1.Location, profile *googlev1.Profile) bool {
	if len(loc.Line) == 0 {
		return false
	}

	// Quick bounds check for better performance
	funcLen := uint64(len(profile.Function))
	strLen := int64(len(profile.StringTable))

	for _, line := range loc.Line {
		if line.FunctionId >= funcLen {
			return false
		}
		fn := profile.Function[line.FunctionId]
		// Avoid multiple bounds checks
		if fn.Name <= 0 || fn.Filename <= 0 || fn.Name >= strLen || fn.Filename >= strLen {
			return false
		}
	}
	return true
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&cfg.Enabled, "symbolizer.enabled", false, "Enable symbolization for unsymbolized profiles")
	f.StringVar(&cfg.DebuginfodURL, "symbolizer.debuginfod-url", "https://debuginfod.elfutils.org", "URL of the debuginfod server")
	f.IntVar(&cfg.InMemorySymbolCacheSize, "symbolizer.in-memory-symbol-cache-size", DefaultInMemorySymbolCacheSize, "Maximum number of entries in the in-memory symbol cache")
	f.IntVar(&cfg.InMemoryDebuginfoCacheSize, "symbolizer.in-memory-debuginfo-cache-size", DefaultInMemoryDebuginfoCacheSize, "Maximum size in bytes for the in-memory debug info cache")
	f.BoolVar(&cfg.PersistentDebugInfoStore.Enabled, "symbolizer.persistent-debuginfo-store.enabled", false, "Enable persistent debug info storage")
	f.DurationVar(&cfg.PersistentDebugInfoStore.MaxAge, "symbolizer.persistent-debuginfo-store.max-age", 7*24*time.Hour, "Maximum age of stored debug info")
}

func (cfg *Config) RegisterFlagsWithContext(ctx context.Context, f *flag.FlagSet) {
	cfg.RegisterFlags(f)
	cfg.PersistentDebugInfoStore.Storage.RegisterFlagsWithPrefix("symbolizer.persistent-debuginfo-store.storage.", f, util.Logger)
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
