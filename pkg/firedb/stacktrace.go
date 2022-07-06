package firedb

import (
	"encoding/binary"

	"github.com/cespare/xxhash/v2"
)

type Stacktrace struct {
	LocationIDs []uint64 `parquet:","`
}

type stacktracesKey uint64

type stacktracesHelper struct {
}

func (_ *stacktracesHelper) key(s *Stacktrace) stacktracesKey {
	var (
		h = xxhash.New()
		b = make([]byte, 8)
	)

	for pos := range s.LocationIDs {
		binary.LittleEndian.PutUint64(b, s.LocationIDs[pos])
		if _, err := h.Write(b); err != nil {
			panic("unable to write hash")
		}
	}

	// TODO: Those hashes might as well collide, let's defer handling collisions till we need to do it
	return stacktracesKey(h.Sum64())
}

func (_ *stacktracesHelper) addToRewriter(r *rewriter, m idConversionTable) {
	r.stacktraces = m
}

func (_ *stacktracesHelper) rewrite(r *rewriter, s *Stacktrace) error {
	for pos := range s.LocationIDs {
		r.locations.rewriteUint64(&s.LocationIDs[pos])
	}
	return nil
}
