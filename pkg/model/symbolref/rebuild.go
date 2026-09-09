package symbolref

import (
	"fmt"
	"slices"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
)

// Frame is one resolved stack frame; deliberately minimal, with no lidia
// dependency.
type Frame struct {
	Name string
}

// resolveKey memoizes Rebuild's resolve calls by (build ID index, address)
// within a single call, so a repeated address is resolved once.
type resolveKey struct {
	buildID uint32
	addr    uint64
}

// Rebuild expands every model.LocationRefName ref in treeBytes (a
// model.LocationRefNameTree marshal paired with pb) into resolved
// model.FunctionNames, rebuilds a plain tree from the expanded stacks, and
// truncates exactly once, via a single tree.Bytes(maxNodes, nil) call after
// every stack has been expanded and reinserted.
//
// resolve returning nil or an empty slice for a given (buildID, addr) means
// "could not resolve"; Rebuild then synthesizes exactly one fallback frame
// named by FallbackSymbolName. A resolved chain must be root-first — outermost
// caller at index 0, innermost frame last, the reverse of lidia's and pprof
// Line order — and is spliced into the rebuilt stack unchanged, expanding
// one address into that many tree levels.
// Because every stack is reinserted from scratch, two refs that expand to
// the same name at the same position merge structurally, with no separate
// dedup pass.
//
// Returns an error only for malformed input: an unmarshalable treeBytes, a
// structurally inconsistent pb, or a tree ref outside pb's valid range.
func Rebuild(
	treeBytes []byte,
	pb *queryv1.SymbolRefTable,
	resolve func(buildID string, addr uint64) []Frame,
	maxNodes int64,
) ([]byte, error) {
	if err := validateSymbolRefTable(pb); err != nil {
		return nil, err
	}

	src, err := model.UnmarshalTree[model.LocationRefName, model.LocationRefNameI](treeBytes)
	if err != nil {
		return nil, err
	}

	names := pb.GetNames()
	buildIDs := pb.GetBuildIds()
	binaryNames := pb.GetBinaryNames()
	unresolvedBuildID := pb.GetUnresolvedBuildId()
	unresolvedAddress := pb.GetUnresolvedAddress()
	numUnresolved := len(unresolvedAddress)

	cache := make(map[resolveKey][]model.FunctionName, numUnresolved)

	expand := func(ref model.LocationRefName) ([]model.FunctionName, error) {
		switch {
		case ref == model.OtherLocationRef:
			return []model.FunctionName{model.OtherFunctionName}, nil
		case ref < 0:
			return nil, fmt.Errorf("symbolref: ref %d out of range", ref)
		case int(ref) < len(names):
			return []model.FunctionName{model.FunctionName(names[ref])}, nil
		}

		i := int(ref) - len(names)
		if i >= numUnresolved {
			return nil, fmt.Errorf("symbolref: ref %d out of range (len(names)=%d, len(unresolved)=%d)", ref, len(names), numUnresolved)
		}

		bi := unresolvedBuildID[i]
		addr := unresolvedAddress[i]
		key := resolveKey{buildID: bi, addr: addr}
		if frames, ok := cache[key]; ok {
			return frames, nil
		}

		frames := expandOne(resolve(buildIDs[bi], addr), buildIDBinaryName(binaryNames, bi), addr)
		cache[key] = frames
		return frames, nil
	}

	dst := new(model.FunctionNameTree)
	expanded := make([]model.FunctionName, 0, 64)
	var rebuildErr error
	src.IterateStacks(func(_ model.LocationRefName, self int64, stack []model.LocationRefName) {
		if rebuildErr != nil {
			return
		}
		slices.Reverse(stack)
		expanded = expanded[:0]
		for _, ref := range stack {
			frames, err := expand(ref)
			if err != nil {
				rebuildErr = err
				return
			}
			expanded = append(expanded, frames...)
		}
		dst.InsertStack(self, expanded...)
	})
	if rebuildErr != nil {
		return nil, rebuildErr
	}

	return dst.Bytes(maxNodes, nil), nil
}

// expandOne converts a resolve() result into the frame chain for one
// unresolved location, synthesizing the fallback frame when resolution
// failed.
func expandOne(resolved []Frame, binaryName string, addr uint64) []model.FunctionName {
	if len(resolved) == 0 {
		return []model.FunctionName{model.FunctionName(FallbackSymbolName(binaryName, addr))}
	}
	frames := make([]model.FunctionName, len(resolved))
	for i, f := range resolved {
		frames[i] = model.FunctionName(f.Name)
	}
	return frames
}

func buildIDBinaryName(binaryNames []string, bi uint32) string {
	if int(bi) < len(binaryNames) {
		return binaryNames[bi]
	}
	return ""
}
