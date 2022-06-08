// Copyright (c) 2022 The Parca Authors
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
//

package addr2line

import (
	"debug/elf"
	"errors"
	"fmt"
	"sort"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	pb "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
	"github.com/parca-dev/parca/pkg/metastore"
)

type SymtabLiner struct {
	logger log.Logger

	symbols []elf.Symbol
}

func Symbols(logger log.Logger, path string) (*SymtabLiner, error) {
	symbols, err := symtab(path)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch symbols from object file: %w", err)
	}

	return &SymtabLiner{
		logger:  log.With(logger, "liner", "symtab"),
		symbols: symbols,
	}, nil
}

func (lnr *SymtabLiner) PCToLines(addr uint64) (lines []metastore.LocationLine, err error) {
	i := sort.Search(len(lnr.symbols), func(i int) bool {
		sym := lnr.symbols[i]
		return sym.Value >= addr
	})
	if i >= len(lnr.symbols) {
		level.Debug(lnr.logger).Log("msg", "failed to find symbol for address", "addr", addr)
		return nil, errors.New("failed to find symbol for address")
	}

	var (
		file = "?"
		line int64 // 0
	)
	lines = append(lines, metastore.LocationLine{
		Line: line,
		Function: &pb.Function{
			Name:     lnr.symbols[i].Name,
			Filename: file,
		},
	})
	return lines, nil
}

func symtab(path string) ([]elf.Symbol, error) {
	objFile, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open elf: %w", err)
	}
	defer objFile.Close()

	syms, sErr := objFile.Symbols()
	dynSyms, dErr := objFile.DynamicSymbols()

	if sErr != nil && dErr != nil {
		return nil, fmt.Errorf("failed to read symbol sections: %w", sErr)
	}

	syms = append(syms, dynSyms...)
	sort.SliceStable(syms, func(i, j int) bool {
		return syms[i].Value < syms[j].Value
	})

	return syms, nil
}
