package symbolizer

import (
	"context"
	"debug/dwarf"
	"debug/elf"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	objstoreclient "github.com/grafana/pyroscope/pkg/objstore/client"
)

// DwarfResolver implements the liner interface
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

type Config struct {
	DebuginfodURL string                `yaml:"debuginfod_url"`
	Cache         CacheConfig           `yaml:"cache"`
	Storage       objstoreclient.Config `yaml:"storage"`
}

type Symbolizer struct {
	client DebuginfodClient
	cache  DebugInfoCache
}

func NewSymbolizer(client DebuginfodClient, cache DebugInfoCache) *Symbolizer {
	if cache == nil {
		cache = NewNullCache()
	}
	return &Symbolizer{
		client: client,
		cache:  cache,
	}
}

func NewFromConfig(ctx context.Context, cfg Config) (*Symbolizer, error) {
	client := NewDebuginfodClient(cfg.DebuginfodURL)

	// Default to no caching
	var cache = NewNullCache()

	if cfg.Cache.Enabled {
		if cfg.Storage.Backend == "" {
			return nil, fmt.Errorf("storage configuration required when cache is enabled")
		}
		bucket, err := objstoreclient.NewBucket(ctx, cfg.Storage, "debuginfo")
		if err != nil {
			return nil, fmt.Errorf("create debug info storage: %w", err)
		}
		cache = NewObjstoreCache(bucket, cfg.Cache.MaxAge)
	}

	return &Symbolizer{
		client: client,
		cache:  cache,
	}, nil
}

func (s *Symbolizer) Symbolize(ctx context.Context, req Request) error {
	debugReader, err := s.cache.Get(ctx, req.BuildID)
	if err == nil {
		defer debugReader.Close()
		return s.symbolizeFromReader(ctx, debugReader, req)
	}

	// Cache miss - fetch from debuginfod
	filepath, err := s.client.FetchDebuginfo(req.BuildID)
	if err != nil {
		return fmt.Errorf("fetch debuginfo: %w", err)
	}

	// Open for symbolization
	f, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("open debug file: %w", err)
	}
	defer f.Close()

	// Cache it for future use
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("seek file: %w", err)
	}
	if err := s.cache.Put(ctx, req.BuildID, f); err != nil {
		// TODO: Log it but don't fail?
	}

	// Seek back to start for symbolization
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("seek file: %w", err)
	}

	return s.symbolizeFromReader(ctx, f, req)
}

func (s *Symbolizer) symbolizeFromReader(ctx context.Context, r io.ReadCloser, req Request) error {
	elfFile, err := elf.NewFile(io.NewSectionReader(r.(io.ReaderAt), 0, 1<<63-1))
	if err != nil {
		return fmt.Errorf("create ELF file from reader: %w", err)
	}
	defer elfFile.Close()

	// Get executable info for address normalization
	ei, err := ExecutableInfoFromELF(elfFile)
	if err != nil {
		return fmt.Errorf("executable info from ELF: %w", err)
	}

	// Create liner
	liner, err := NewDwarfResolver(elfFile)
	if err != nil {
		return fmt.Errorf("create liner: %w", err)
	}
	//defer liner.Close()

	for _, mapping := range req.Mappings {
		for _, loc := range mapping.Locations {
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
				continue // Skip errors for individual addresses
			}

			loc.Lines = lines
		}
	}

	return nil
}

func (cfg *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.StringVar(&cfg.DebuginfodURL, prefix+".debuginfod-url", "https://debuginfod.elfutils.org", "URL of the debuginfod server")

	cachePrefix := prefix + ".cache"
	f.BoolVar(&cfg.Cache.Enabled, cachePrefix+".enabled", false, "Enable debug info caching")
	f.DurationVar(&cfg.Cache.MaxAge, cachePrefix+".max-age", 7*24*time.Hour, "Maximum age of cached debug info")
}
