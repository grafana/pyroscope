package compactionworker

import (
	"os"
	"path/filepath"

	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
)

type SymbolsRewriter interface {
	ReWriteRow(profile profileRow) error
	Close() error
}

type SymbolsRewriterFn func(blockPath string) SymbolsRewriter

type profileRow struct {
	timeNanos int64

	labels phlaremodel.Labels
	fp     model.Fingerprint
	row    schemav1.ProfileRow

	serviceReader *serviceReader
}

type symbolsCompactor struct {
	version     symdb.FormatVersion
	rewriters   map[*serviceReader]*symdb.Rewriter
	w           *symdb.SymDB
	stacktraces []uint32

	dst     string
	flushed bool
}

func newSymbolsCompactor(path string, version symdb.FormatVersion) *symbolsCompactor {
	return &symbolsCompactor{
		version: version,
		w: symdb.NewSymDB(symdb.DefaultConfig().
			WithVersion(symdb.FormatV3).
			WithDirectory(path)),
		dst:       path,
		rewriters: make(map[*serviceReader]*symdb.Rewriter),
	}
}

func (s *symbolsCompactor) Rewriter(dst string) SymbolsRewriter {
	return &symbolsRewriter{
		symbolsCompactor: s,
		dst:              dst,
	}
}

type symbolsRewriter struct {
	*symbolsCompactor

	numSamples uint64
	dst        string
}

func (s *symbolsRewriter) NumSamples() uint64 { return s.numSamples }

func (s *symbolsRewriter) ReWriteRow(profile profileRow) error {
	total, err := s.symbolsCompactor.ReWriteRow(profile)
	s.numSamples += total
	return err
}

func (s *symbolsRewriter) Close() error {
	return s.symbolsCompactor.Flush()
}

func (s *symbolsCompactor) ReWriteRow(profile profileRow) (uint64, error) {
	var (
		err              error
		rewrittenSamples uint64
	)
	profile.row.ForStacktraceIDsValues(func(values []parquet.Value) {
		s.loadStacktraceIDs(values)
		r, ok := s.rewriters[profile.serviceReader]
		if !ok {
			r = symdb.NewRewriter(s.w, profile.serviceReader.symbolsReader)
			s.rewriters[profile.serviceReader] = r
		}
		if err = r.Rewrite(profile.row.StacktracePartitionID(), s.stacktraces); err != nil {
			return
		}
		rewrittenSamples += uint64(len(values))
		for i, v := range values {
			// FIXME: the original order is not preserved, which will affect encoding.
			values[i] = parquet.Int64Value(int64(s.stacktraces[i])).Level(v.RepetitionLevel(), v.DefinitionLevel(), v.Column())
		}
	})
	if err != nil {
		return rewrittenSamples, err
	}
	return rewrittenSamples, nil
}

func (s *symbolsCompactor) Flush() error {
	if s.flushed {
		return nil
	}
	if err := s.w.Flush(); err != nil {
		return err
	}
	s.flushed = true
	return nil
}

func (s *symbolsCompactor) Close() error {
	if s.version == symdb.FormatV3 {
		return os.RemoveAll(filepath.Join(s.dst, symdb.DefaultFileName))
	}
	return os.RemoveAll(s.dst)
}

func (s *symbolsCompactor) loadStacktraceIDs(values []parquet.Value) {
	s.stacktraces = grow(s.stacktraces, len(values))
	for i := range values {
		s.stacktraces[i] = values[i].Uint32()
	}
}

func grow[T any](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n, 2*n)
	}
	return s[:n]
}
