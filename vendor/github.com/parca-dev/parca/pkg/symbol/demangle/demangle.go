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

package demangle

import (
	"strings"

	"github.com/ianlancetaylor/demangle"

	pb "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
)

type Demangler struct {
	options []demangle.Option
	mode    string
	force   bool
}

func NewDemangler(mode string, force bool) *Demangler {
	var options []demangle.Option
	switch mode {
	case "", "simple": // demangled, simplified: no parameters, no templates, no return type
		options = []demangle.Option{demangle.NoParams, demangle.NoTemplateParams}
	case "templates": // demangled, simplified: no parameters, no return type
		options = []demangle.Option{demangle.NoParams}
	case "full":
		options = []demangle.Option{demangle.NoClones}
	case "none": // no demangling
		return nil
	}

	return &Demangler{
		options: options,
		mode:    mode,
		force:   force,
	}
}

// Demangle updates the function names in a profile demangling C++ and
// Rust names, simplified according to demanglerMode. If force is set,
// overwrite any names that appear already demangled.
// A modified version of pprof demangler.
func (d *Demangler) Demangle(fn *pb.Function) *pb.Function {
	if d == nil {
		return fn
	}

	if d.force {
		// Remove the current demangled names to force demangling.
		if fn.Name != "" && fn.SystemName != "" {
			fn.Name = fn.SystemName
		}
	}

	if fn.Name != "" && fn.SystemName != fn.Name {
		return fn // Already demangled.
	}

	if demangled := demangle.Filter(fn.SystemName, d.options...); demangled != fn.SystemName {
		fn.Name = demangled
		return fn
	}
	// Could not demangle. Apply heuristics in case the name is
	// already demangled.
	name := fn.SystemName
	if looksLikeDemangledCPlusPlus(name) {
		if d.mode == "" || d.mode == "templates" {
			name = removeMatching(name, '(', ')')
		}
		if d.mode == "" {
			name = removeMatching(name, '<', '>')
		}
	}
	fn.Name = name
	return fn
}

// looksLikeDemangledCPlusPlus is a heuristic to decide if a name is
// the result of demangling C++. If so, further heuristics will be
// applied to simplify the name.
func looksLikeDemangledCPlusPlus(demangled string) bool {
	if strings.Contains(demangled, ".<") { // Skip java names of the form "class.<init>"
		return false
	}
	return strings.ContainsAny(demangled, "<>[]") || strings.Contains(demangled, "::")
}

// removeMatching removes nested instances of start..end from name.
func removeMatching(name string, start, end byte) string {
	s := string(start) + string(end)
	var nesting, first, current int
	for index := strings.IndexAny(name[current:], s); index != -1; index = strings.IndexAny(name[current:], s) {
		switch current += index; name[current] {
		case start:
			nesting++
			if nesting == 1 {
				first = current
			}
		case end:
			nesting--
			switch {
			case nesting < 0:
				return name // Mismatch, abort
			case nesting == 0:
				name = name[:first] + name[current+1:]
				current = first - 1
			}
		}
		current++
	}
	return name
}
