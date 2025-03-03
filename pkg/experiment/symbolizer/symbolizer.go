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

	pprof "github.com/google/pprof/profile"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/prometheus/client_golang/prometheus"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	objstoreclient "github.com/grafana/pyroscope/pkg/objstore/client"
)

const (
	DefaultSymbolCacheSize = 100_000
)

type Config struct {
	DebuginfodURL   string                `yaml:"debuginfod_url"`
	Cache           CacheConfig           `yaml:"cache"`
	Storage         objstoreclient.Config `yaml:"storage"`
	SymbolCacheSize int                   `yaml:"symbol_cache_size"`
}

type Symbolizer interface {
	SymbolizePprof(ctx context.Context, profile *googlev1.Profile) error
}

// ProfileSymbolizer implements Symbolizer
type ProfileSymbolizer struct {
	client  DebuginfodClient
	cache   DebugInfoCache
	metrics *Metrics

	symbolCache *lru.Cache[string, []SymbolLocation]
}

func NewFromConfig(ctx context.Context, cfg Config, reg prometheus.Registerer) (*ProfileSymbolizer, error) {
	var cache DebugInfoCache = NewNullCache()

	metrics := NewMetrics(reg)

	if cfg.Cache.Enabled {
		if cfg.Storage.Backend == "" {
			return nil, fmt.Errorf("storage configuration required when cache is enabled")
		}
		bucket, err := objstoreclient.NewBucket(ctx, cfg.Storage, "debuginfo")
		if err != nil {
			return nil, fmt.Errorf("create debug info storage: %w", err)
		}
		cache = NewObjstoreCache(bucket, cfg.Cache.MaxAge, metrics)
	}

	client := NewDebuginfodClient(cfg.DebuginfodURL, metrics)

	// Use configured cache size or default
	symbolCacheSize := DefaultSymbolCacheSize
	if cfg.SymbolCacheSize > 0 {
		symbolCacheSize = cfg.SymbolCacheSize
	}

	return NewProfileSymbolizer(client, cache, metrics, symbolCacheSize), nil
}

func NewProfileSymbolizer(client DebuginfodClient, cache DebugInfoCache, metrics *Metrics, symbolCacheSize int) *ProfileSymbolizer {
	if cache == nil {
		cache = NewNullCache()
	}

	// Default to DefaultSymbolCacheSize if not specified
	if symbolCacheSize <= 0 {
		symbolCacheSize = DefaultSymbolCacheSize
	}

	symbolCache, err := lru.New[string, []SymbolLocation](symbolCacheSize)
	if err != nil {
		symbolCache = nil
	}

	return &ProfileSymbolizer{
		client:      client,
		cache:       cache,
		metrics:     metrics,
		symbolCache: symbolCache,
	}
}

func (s *ProfileSymbolizer) SymbolizePprof(ctx context.Context, profile *googlev1.Profile) error {
	start := time.Now()
	defer func() {
		s.metrics.profileSymbolizationDuration.Observe(time.Since(start).Seconds())
	}()
	s.metrics.profileSymbolizationTotal.Inc()

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
		s.metrics.profileSymbolizationErrors.WithLabelValues("symbolization_error").Inc()
		return fmt.Errorf("symbolization errors: %v", errs)
	}

	return nil
}

func (s *ProfileSymbolizer) Symbolize(ctx context.Context, req *Request) error {
	start := time.Now()
	defer func() {
		s.metrics.debugSymbolResolutionDuration.Observe(time.Since(start).Seconds())
	}()

	// Check if we need to fetch debug info at all
	if s.symbolCache != nil {
		allCached := true
		// Check each location in the Ristretto cache
		for _, loc := range req.Locations {
			// Create a cache key that combines buildID and address
			cacheKey := fmt.Sprintf("%s:%x", req.BuildID, loc.Address)

			if symbols, found := s.symbolCache.Get(cacheKey); found {
				// We've already symbolized this address
				loc.Lines = symbols
				s.metrics.debugSymbolResolutionsTotal.WithLabelValues("cache_hit").Inc()
			} else {
				// We need to fetch at least one address
				allCached = false
				break
			}
		}

		// If all addresses were in cache, we're done
		if allCached {
			return nil
		}
	}

	// Try to get from ObjstoreCache first
	objstoreReader, err := s.cache.Get(ctx, req.BuildID)
	if err == nil {
		// ObjstoreCache hit
		defer objstoreReader.Close()
		err := s.symbolizeFromReader(ctx, objstoreReader, req)
		if err != nil {
			return err
		}

		s.updateSymbolCache(req)
		return nil
	}

	// ObjstoreCache miss - try to get from debuginfod client
	debugReader, err := s.client.FetchDebuginfo(ctx, req.BuildID)
	if err != nil {
		s.metrics.debugSymbolResolutionErrors.WithLabelValues("debuginfod_error").Inc()
		return fmt.Errorf("fetch debuginfo: %w", err)
	}
	defer debugReader.Close()

	// Store in ObjstoreCache for future use
	if s.cache != nil {
		var buf bytes.Buffer
		teeReader := io.TeeReader(debugReader, &buf)

		// Symbolize from the tee reader
		err := s.symbolizeFromReader(ctx, io.NopCloser(teeReader), req)
		if err != nil {
			return err
		}

		// Put the buffered data into the ObjstoreCache
		if cacheErr := s.cache.Put(ctx, req.BuildID, bytes.NewReader(buf.Bytes())); cacheErr != nil {
			// TODO: Add logging
		}

		s.updateSymbolCache(req)
		return nil
	}

	// If we don't have an ObjstoreCache, just symbolize directly
	err = s.symbolizeFromReader(ctx, debugReader, req)
	if err != nil {
		return err
	}

	s.updateSymbolCache(req)
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
		s.metrics.debugSymbolResolutionErrors.WithLabelValues("elf_error").Inc()
		return fmt.Errorf("create ELF file from reader: %w", err)
	}
	defer elfFile.Close()

	// Get executable info for address normalization
	ei, err := ExecutableInfoFromELF(elfFile)
	if err != nil {
		s.metrics.debugSymbolResolutionErrors.WithLabelValues("elf_info_error").Inc()
		return fmt.Errorf("executable info from ELF: %w", err)
	}

	// Create liner
	liner, err := NewDwarfResolver(elfFile)
	if err != nil {
		s.metrics.debugSymbolResolutionErrors.WithLabelValues("dwarf_error").Inc()
		return fmt.Errorf("create DWARF resolver: %w", err)
	}

	for _, loc := range req.Locations {
		addr, err := MapRuntimeAddress(loc.Address, ei, Mapping{
			Start:  loc.Mapping.Start,
			Limit:  loc.Mapping.Limit,
			Offset: loc.Mapping.Offset,
		})
		if err != nil {
			s.metrics.debugSymbolResolutionErrors.WithLabelValues("error").Inc()
			return fmt.Errorf("normalize address: %w", err)
		}

		// Get source lines for the address
		lines, err := liner.ResolveAddress(ctx, addr)
		if err != nil {
			s.metrics.debugSymbolResolutionErrors.WithLabelValues("error").Inc()
			return fmt.Errorf("resolve address: %w", err)
		}

		loc.Lines = lines
		s.metrics.debugSymbolResolutionsTotal.WithLabelValues("success").Inc()
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
			// Create a cache key that combines buildID and address
			cacheKey := fmt.Sprintf("%s:%x", req.BuildID, loc.Address)
			s.symbolCache.Add(cacheKey, loc.Lines)
		}
	}
}

// lookupSymbolCache checks if an address is already in the symbol cache
func (s *ProfileSymbolizer) lookupSymbolCache(buildID string, addr uint64) ([]SymbolLocation, bool) {
	if s.symbolCache == nil {
		return nil, false
	}

	cacheKey := fmt.Sprintf("%s:%x", buildID, addr)
	lines, found := s.symbolCache.Get(cacheKey)
	if !found {
		// s.metrics.symbolCacheMissesTotal.Inc()
		return nil, false
	}

	// s.metrics.symbolCacheHitsTotal.Inc()
	return lines, true
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

func (cfg *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.StringVar(&cfg.DebuginfodURL, prefix+".debuginfod-url", "https://debuginfod.elfutils.org", "URL of the debuginfod server")
	f.IntVar(&cfg.SymbolCacheSize, prefix+".symbol-cache-size", DefaultSymbolCacheSize, "Maximum number of entries in the symbol cache")

	cachePrefix := prefix + ".cache"
	f.BoolVar(&cfg.Cache.Enabled, cachePrefix+".enabled", false, "Enable debug info caching")
	f.DurationVar(&cfg.Cache.MaxAge, cachePrefix+".max-age", 7*24*time.Hour, "Maximum age of cached debug info")
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
