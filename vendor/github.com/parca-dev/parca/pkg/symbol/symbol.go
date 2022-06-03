// Copyright 2021 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package symbol

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/goburrow/cache"

	pb "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
	"github.com/parca-dev/parca/pkg/hash"
	"github.com/parca-dev/parca/pkg/metastore"
	"github.com/parca-dev/parca/pkg/symbol/addr2line"
	"github.com/parca-dev/parca/pkg/symbol/demangle"
	"github.com/parca-dev/parca/pkg/symbol/elfutils"
)

var ErrLinerCreationFailedBefore = errors.New("failed to initialize liner")

type Symbolizer struct {
	logger    log.Logger
	demangler *demangle.Demangler

	cacheOpts  []cache.Option
	linerCache cache.Cache

	attemptThreshold    int
	linerCreationFailed map[string]struct{}

	symbolizationAttempts map[string]map[uint64]int
	symbolizationFailed   map[string]map[uint64]struct{}
}

type liner interface {
	PCToLines(pc uint64) ([]metastore.LocationLine, error)
}

func NewSymbolizer(logger log.Logger, opts ...Option) (*Symbolizer, error) {
	log.With(logger, "component", "symbolizer")

	const (
		defaultDemangleMode     = "simple"
		defaultCacheSize        = 1000
		defaultCacheItemTTL     = time.Minute
		defaultAttemptThreshold = 3
	)

	sym := &Symbolizer{
		logger:    logger,
		demangler: demangle.NewDemangler(defaultDemangleMode, false),

		// e.g: Parca binary compressed DWARF data size ~8mb as of 10.2021
		cacheOpts: []cache.Option{
			cache.WithMaximumSize(defaultCacheSize),
			cache.WithExpireAfterAccess(defaultCacheItemTTL),
		},

		attemptThreshold: defaultAttemptThreshold,

		linerCreationFailed: map[string]struct{}{},

		symbolizationAttempts: map[string]map[uint64]int{},
		symbolizationFailed:   map[string]map[uint64]struct{}{},
	}
	for _, opt := range opts {
		opt(sym)
	}
	sym.linerCache = cache.New(sym.cacheOpts...)

	return sym, nil
}

// Symbolize attempts to symbolize the given locations using the given debug information file.
func (s *Symbolizer) Symbolize(ctx context.Context, m *pb.Mapping, locations []*metastore.Location, debugInfoFile string) (map[*metastore.Location][]metastore.LocationLine, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	logger := log.With(s.logger, "buildid", m.BuildId)

	liner, err := s.liner(m, debugInfoFile)
	if err != nil {
		const msg = "failed to create liner"
		level.Debug(logger).Log("msg", msg, "err", err)
		return nil, fmt.Errorf(msg+": %w", err)
	}

	// Generate a hash key to use for error tracking.
	key, err := hash.File(debugInfoFile)
	if err != nil {
		level.Warn(s.logger).Log("msg", "failed to generate cache key", "err", err)
		key = m.BuildId
	}

	locationLines := map[*metastore.Location][]metastore.LocationLine{}
	for _, loc := range locations {
		locationLines[loc] = append(locationLines[loc], s.pcToLines(liner, m.BuildId, key, loc.Address)...)
	}
	return locationLines, nil
}

// pcToLines returns the line number of the given PC while keeping the track of symbolization attempts and failures.
func (s *Symbolizer) pcToLines(liner liner, buildID, key string, addr uint64) []metastore.LocationLine {
	logger := log.With(s.logger, "addr", addr, "buildid", buildID)
	// Check if we already attempt to symbolize this location and failed.
	if _, failedBefore := s.symbolizationFailed[key][addr]; failedBefore {
		level.Debug(logger).Log("msg", "location already had been attempted to be symbolized and failed, skipping")
		return nil
	}
	// Where the magic happens.
	lines, err := liner.PCToLines(addr)
	if err != nil {
		// Error bookkeeping.
		if prev, ok := s.symbolizationAttempts[key][addr]; ok {
			prev++
			if prev >= s.attemptThreshold {
				if _, ok := s.symbolizationFailed[key]; ok {
					s.symbolizationFailed[key][addr] = struct{}{}
				} else {
					s.symbolizationFailed[key] = map[uint64]struct{}{addr: {}}
				}
				delete(s.symbolizationAttempts[key], addr)
			} else {
				s.symbolizationAttempts[key][addr] = prev
			}
			return nil
		}
		// First failed attempt.
		s.symbolizationAttempts[key] = map[uint64]int{addr: 1}
		level.Debug(logger).Log("msg", "failed to extract source lines", "err", err)
		return nil
	}
	if len(lines) == 0 {
		if _, ok := s.symbolizationFailed[key]; ok {
			s.symbolizationFailed[key][addr] = struct{}{}
		} else {
			s.symbolizationFailed[key] = map[uint64]struct{}{addr: {}}
		}
		delete(s.symbolizationAttempts[key], addr)
		level.Debug(logger).Log("msg", "could not find any lines for given address")
	}
	return lines
}

func (s *Symbolizer) Close() error {
	return s.linerCache.Close()
}

// liner creates a new liner for the given mapping and object file path and caches it.
func (s *Symbolizer) liner(m *pb.Mapping, path string) (liner, error) {
	h, err := hash.File(path)
	if err != nil {
		level.Warn(s.logger).Log("msg", "failed to generate cache key", "err", err)
		h = path
	}

	// Check if we already attempt to build a liner for this path.
	if _, failedBefore := s.linerCreationFailed[h]; failedBefore {
		level.Debug(s.logger).Log("msg", "already failed to create liner for this debug info file, skipping")
		return nil, ErrLinerCreationFailedBefore
	}

	if val, ok := s.linerCache.GetIfPresent(h); ok {
		level.Debug(s.logger).Log("msg", "using cached liner to resolve symbols", "file", path)
		return val.(liner), nil
	}

	lnr, err := s.newLiner(m.BuildId, path)
	if err != nil {
		level.Error(s.logger).Log(
			"msg", "failed to open object file",
			"file", path,
			"buildid", m.BuildId,
			"err", err,
		)
		s.linerCreationFailed[h] = struct{}{}
		s.linerCache.Invalidate(h)
		return nil, err
	}

	level.Debug(s.logger).Log("msg", "liner cached", "file", path)
	s.linerCache.Put(h, lnr)
	return lnr, nil
}

// newLiner creates a new liner for the given mapping and object file path.
func (s *Symbolizer) newLiner(buildID, path string) (liner, error) {
	logger := log.With(s.logger, "file", path, "buildid", buildID)
	hasDWARF, err := elfutils.HasDWARF(path)
	if err != nil {
		level.Debug(logger).Log("msg", "failed to determine if binary has DWARF info", "err", err)
	}
	if hasDWARF {
		level.Debug(logger).Log("msg", "using DWARF liner to resolve symbols")
		lnr, err := addr2line.DWARF(logger, path, s.demangler)
		if err != nil {
			return nil, err
		}
		return lnr, nil
	}

	// Go binaries has a special case. They use ".gopclntab" section to symbolize addresses.
	// Keep that section and other identifying sections in the debug information file.
	isGo, err := elfutils.IsSymbolizableGoObjFile(path)
	if err != nil {
		level.Debug(logger).Log("msg", "failed to determine if binary is a Go binary", "err", err)
	}
	if isGo {
		// Right now, this uses "debug/gosym" package, and it won't work for inlined functions,
		// so this is just a best-effort implementation, in case we don't have DWARF.
		lnr, err := addr2line.Go(logger, path)
		if err == nil {
			level.Debug(logger).Log("msg", "using go liner to resolve symbols")
			return lnr, nil
		}
		level.Error(logger).Log("msg", "failed to create go liner, falling back to symtab liner", "err", err)
	}

	// As a last resort, use the symtab liner which utilizes .symtab section and .dynsym section.
	hasSymbols, err := elfutils.HasSymbols(path)
	if err != nil {
		level.Debug(logger).Log("msg", "failed to determine if binary has symbols", "err", err)
	}
	if hasSymbols {
		lnr, err := addr2line.Symbols(logger, path)
		if err == nil {
			level.Debug(logger).Log("msg", "using symtab liner to resolve symbols")
			return lnr, nil
		}
		level.Error(logger).Log("msg", "failed to create symtab liner", "err", err)
	}

	return nil, errors.New("cannot create a liner from given object file")
}
