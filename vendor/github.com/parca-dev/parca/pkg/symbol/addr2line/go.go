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

package addr2line

import (
	"debug/elf"
	"debug/gosym"
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/go-kit/log"

	pb "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
	"github.com/parca-dev/parca/pkg/metastore"
)

type GoLiner struct {
	logger log.Logger

	symtab *gosym.Table
}

func Go(logger log.Logger, path string) (*GoLiner, error) {
	tab, err := gosymtab(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create go symbtab: %w", err)
	}

	return &GoLiner{
		logger: log.With(logger, "liner", "go"),
		symtab: tab,
	}, nil
}

func (gl *GoLiner) PCToLines(addr uint64) (lines []metastore.LocationLine, err error) {
	defer func() {
		// PCToLine panics with "invalid memory address or nil pointer dereference",
		//	- when it refers to an address that doesn't actually exist.
		if r := recover(); r != nil {
			fmt.Println("recovered stack stares:\n", string(debug.Stack()))
			err = fmt.Errorf("recovering from panic in Go add2line: %v", r)
		}
	}()

	name := "?"
	// TODO(kakkoyun): Do we need to consider the base address for any part of Go binaries?
	file, line, fn := gl.symtab.PCToLine(addr)
	if fn != nil {
		name = fn.Name
	}

	// TODO(kakkoyun): These lines miss the inline functions.
	// - Find a way to symbolize inline functions.
	lines = append(lines, metastore.LocationLine{
		Line: int64(line),
		Function: &pb.Function{
			Name:     name,
			Filename: file,
		},
	})
	return lines, nil
}

func gosymtab(path string) (*gosym.Table, error) {
	objFile, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open elf: %w", err)
	}
	defer objFile.Close()

	var pclntab []byte
	if sec := objFile.Section(".gopclntab"); sec != nil {
		if sec.Type == elf.SHT_NOBITS {
			return nil, errors.New(".gopclntab section has no bits")
		}

		pclntab, err = sec.Data()
		if err != nil {
			return nil, fmt.Errorf("could not find .gopclntab section: %w", err)
		}
	}

	if len(pclntab) <= 0 {
		return nil, errors.New(".gopclntab section has no bits")
	}

	var symtab []byte
	if sec := objFile.Section(".gosymtab"); sec != nil {
		symtab, _ = sec.Data()
	}

	var text uint64
	if sec := objFile.Section(".text"); sec != nil {
		text = sec.Addr
	}

	table, err := gosym.NewTable(symtab, gosym.NewLineTable(pclntab, text))
	if err != nil {
		return nil, fmt.Errorf("failed to build symtab or pclinetab: %w", err)
	}
	return table, nil
}
