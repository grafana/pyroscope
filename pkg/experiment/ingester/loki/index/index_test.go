// Copyright 2017 The Prometheus Authors
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

package index

import (
	"context"
	"fmt"
	"hash/crc32"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/util/testutil"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("github.com/golang/glog.(*fileSink).flushDaemon"),
		goleak.IgnoreTopFunction("github.com/dgraph-io/ristretto.(*defaultPolicy).processItems"),
		goleak.IgnoreTopFunction("github.com/dgraph-io/ristretto.(*Cache).processItems"),
	)
}

type series struct {
	l      phlaremodel.Labels
	chunks []index.ChunkMeta
}

type mockIndex struct {
	series map[storage.SeriesRef]series
	// we're forced to use a anonymous struct here because we can't use typesv1.LabelPair as it's not comparable.
	postings map[struct{ Name, Value string }][]storage.SeriesRef
	symbols  map[string]struct{}
}

func newMockIndex() mockIndex {
	allPostingsKeyName, allPostingsKeyValue := index.AllPostingsKey()
	ix := mockIndex{
		series:   make(map[storage.SeriesRef]series),
		postings: make(map[struct{ Name, Value string }][]storage.SeriesRef),
		symbols:  make(map[string]struct{}),
	}
	ix.postings[struct {
		Name  string
		Value string
	}{allPostingsKeyName, allPostingsKeyValue}] = []storage.SeriesRef{}
	return ix
}

func (m mockIndex) Symbols() (map[string]struct{}, error) {
	return m.symbols, nil
}

func (m mockIndex) AddSeries(ref storage.SeriesRef, l phlaremodel.Labels, chunks ...index.ChunkMeta) error {
	allPostingsKeyName, allPostingsKeyValue := index.AllPostingsKey()

	if _, ok := m.series[ref]; ok {
		return errors.Errorf("series with reference %d already added", ref)
	}
	for _, lbl := range l {
		m.symbols[lbl.Name] = struct{}{}
		m.symbols[lbl.Value] = struct{}{}
		if _, ok := m.postings[struct {
			Name  string
			Value string
		}{lbl.Name, lbl.Value}]; !ok {
			m.postings[struct {
				Name  string
				Value string
			}{lbl.Name, lbl.Value}] = []storage.SeriesRef{}
		}
		m.postings[struct {
			Name  string
			Value string
		}{lbl.Name, lbl.Value}] = append(m.postings[struct {
			Name  string
			Value string
		}{lbl.Name, lbl.Value}], ref)
	}
	m.postings[struct {
		Name  string
		Value string
	}{allPostingsKeyName, allPostingsKeyValue}] = append(m.postings[struct {
		Name  string
		Value string
	}{allPostingsKeyName, allPostingsKeyValue}], ref)

	s := series{l: l}
	// Actual chunk data is not stored in the index.
	s.chunks = append(s.chunks, chunks...)
	m.series[ref] = s

	return nil
}

func (m mockIndex) Close() error {
	return nil
}

func (m mockIndex) LabelValues(name string) ([]string, error) {
	values := []string{}
	for l := range m.postings {
		if l.Name == name {
			values = append(values, l.Value)
		}
	}
	return values, nil
}

func (m mockIndex) Postings(name string, values ...string) (index.Postings, error) {
	p := []index.Postings{}
	for _, value := range values {
		p = append(p, iter.NewSliceSeekIterator(m.postings[struct {
			Name  string
			Value string
		}{Name: name, Value: value}]))
	}
	return index.Merge(p...), nil
}

func (m mockIndex) Series(ref storage.SeriesRef, lset *phlaremodel.Labels, chks *[]index.ChunkMeta) error {
	s, ok := m.series[ref]
	if !ok {
		return errors.New("not found")
	}
	*lset = append((*lset)[:0], s.l...)
	*chks = append((*chks)[:0], s.chunks...)

	return nil
}

func TestIndexRW_Create_Open(t *testing.T) {

	// An empty index must still result in a readable file.
	iw, err := NewWriter(context.Background(), BlocksIndexWriterBufSize)
	require.NoError(t, err)
	require.NoError(t, iw.Close())

	bytes := iw.ReleaseIndexBuffer().buf.Bytes()
	ir, err := NewReader(RealByteSlice(bytes))
	require.NoError(t, err)
	require.NoError(t, ir.Close())

	// Modify magic header must cause open to fail.
	//f, err := os.OpenFile(fn, os.O_WRONLY, 0o666)
	//require.NoError(t, err)
	//err = iw.f.WriteAt([]byte{0, 0}, 0)
	bytes[0] = 0
	require.NoError(t, err)
	//f.Close()

	//_, err = NewFileReader(dir)
	//require.Error(t, err)
}

func TestIndexRW_Postings(t *testing.T) {

	iw, err := NewWriter(context.Background(), BlocksIndexWriterBufSize)
	require.NoError(t, err)

	series := []phlaremodel.Labels{
		phlaremodel.LabelsFromStrings("a", "1", "b", "1"),
		phlaremodel.LabelsFromStrings("a", "1", "b", "2"),
		phlaremodel.LabelsFromStrings("a", "1", "b", "3"),
		phlaremodel.LabelsFromStrings("a", "1", "b", "4"),
	}

	require.NoError(t, iw.AddSymbol("1"))
	require.NoError(t, iw.AddSymbol("2"))
	require.NoError(t, iw.AddSymbol("3"))
	require.NoError(t, iw.AddSymbol("4"))
	require.NoError(t, iw.AddSymbol("a"))
	require.NoError(t, iw.AddSymbol("b"))

	// Postings lists are only written if a series with the respective
	// reference was added before.
	require.NoError(t, iw.AddSeries(1, series[0], model.Fingerprint(series[0].Hash())))
	require.NoError(t, iw.AddSeries(2, series[1], model.Fingerprint(series[1].Hash())))
	require.NoError(t, iw.AddSeries(3, series[2], model.Fingerprint(series[2].Hash())))
	require.NoError(t, iw.AddSeries(4, series[3], model.Fingerprint(series[3].Hash())))

	require.NoError(t, iw.Close())

	ir, err := NewReader(RealByteSlice(iw.ReleaseIndexBuffer().buf.Bytes()))
	require.NoError(t, err)

	p, err := ir.Postings("a", nil, "1")
	require.NoError(t, err)

	var l phlaremodel.Labels
	var c []index.ChunkMeta

	for i := 0; p.Next(); i++ {
		_, err := ir.Series(p.At(), &l, &c)

		require.NoError(t, err)
		require.Equal(t, 0, len(c))
		require.Equal(t, series[i], l)
	}
	require.NoError(t, p.Err())

	// The label indices are no longer used, so test them by hand here.
	labelIndices := map[string][]string{}
	require.NoError(t, ReadOffsetTable(ir.b, ir.toc.LabelIndicesTable, func(key []string, off uint64, _ int) error {
		if len(key) != 1 {
			return errors.Errorf("unexpected key length for label indices table %d", len(key))
		}

		d := encoding.NewDecbufAt(ir.b, int(off), castagnoliTable)
		vals := []string{}
		nc := d.Be32int()
		if nc != 1 {
			return errors.Errorf("unexpected number of label indices table names %d", nc)
		}
		for i := d.Be32(); i > 0; i-- {
			v, err := ir.lookupSymbol(d.Be32())
			if err != nil {
				return err
			}
			vals = append(vals, v)
		}
		labelIndices[key[0]] = vals
		return d.Err()
	}))
	require.Equal(t, map[string][]string{
		"a": {"1"},
		"b": {"1", "2", "3", "4"},
	}, labelIndices)

	require.NoError(t, ir.Close())
}

func TestPostingsMany(t *testing.T) {

	iw, err := NewWriter(context.Background(), BlocksIndexWriterBufSize)
	require.NoError(t, err)

	// Create a label in the index which has 999 values.
	symbols := map[string]struct{}{}
	series := []phlaremodel.Labels{}
	for i := 1; i < 1000; i++ {
		v := fmt.Sprintf("%03d", i)
		series = append(series, phlaremodel.LabelsFromStrings("i", v, "foo", "bar"))
		symbols[v] = struct{}{}
	}
	symbols["i"] = struct{}{}
	symbols["foo"] = struct{}{}
	symbols["bar"] = struct{}{}
	syms := []string{}
	for s := range symbols {
		syms = append(syms, s)
	}
	sort.Strings(syms)
	for _, s := range syms {
		require.NoError(t, iw.AddSymbol(s))
	}

	sort.Slice(series, func(i, j int) bool {
		return series[i].Hash() < series[j].Hash()
	})

	for i, s := range series {
		require.NoError(t, iw.AddSeries(storage.SeriesRef(i), s, model.Fingerprint(s.Hash())))
	}
	require.NoError(t, iw.Close())

	ir, err := NewReader(RealByteSlice(iw.ReleaseIndexBuffer().buf.Bytes()))
	require.NoError(t, err)
	defer func() { require.NoError(t, ir.Close()) }()

	cases := []struct {
		in []string
	}{
		// Simple cases, everything is present.
		{in: []string{"002"}},
		{in: []string{"031", "032", "033"}},
		{in: []string{"032", "033"}},
		{in: []string{"127", "128"}},
		{in: []string{"127", "128", "129"}},
		{in: []string{"127", "129"}},
		{in: []string{"128", "129"}},
		{in: []string{"998", "999"}},
		{in: []string{"999"}},
		// Before actual values.
		{in: []string{"000"}},
		{in: []string{"000", "001"}},
		{in: []string{"000", "002"}},
		// After actual values.
		{in: []string{"999a"}},
		{in: []string{"999", "999a"}},
		{in: []string{"998", "999", "999a"}},
		// In the middle of actual values.
		{in: []string{"126a", "127", "128"}},
		{in: []string{"127", "127a", "128"}},
		{in: []string{"127", "127a", "128", "128a", "129"}},
		{in: []string{"127", "128a", "129"}},
		{in: []string{"128", "128a", "129"}},
		{in: []string{"128", "129", "129a"}},
		{in: []string{"126a", "126b", "127", "127a", "127b", "128", "128a", "128b", "129", "129a", "129b"}},
	}

	for _, c := range cases {
		it, err := ir.Postings("i", nil, c.in...)
		require.NoError(t, err)

		got := []string{}
		var lbls phlaremodel.Labels
		var metas []index.ChunkMeta
		for it.Next() {
			_, err := ir.Series(it.At(), &lbls, &metas)
			require.NoError(t, err)
			got = append(got, lbls.Get("i"))
		}
		require.NoError(t, it.Err())
		exp := []string{}
		for _, e := range c.in {
			if _, ok := symbols[e]; ok && e != "l" {
				exp = append(exp, e)
			}
		}

		// sort expected values by label hash instead of lexicographically by labelset
		sort.Slice(exp, func(i, j int) bool {
			return labels.FromStrings("i", exp[i], "foo", "bar").Hash() < labels.FromStrings("i", exp[j], "foo", "bar").Hash()
		})

		require.Equal(t, exp, got, fmt.Sprintf("input: %v", c.in))
	}
}

func TestPersistence_index_e2e(t *testing.T) {
	lbls, err := labels.ReadLabels("../../../../phlaredb/tsdb/testdata/20kseries.json", 20000)
	require.NoError(t, err)

	flbls := make([]phlaremodel.Labels, len(lbls))
	for i, ls := range lbls {
		flbls[i] = make(phlaremodel.Labels, 0, len(ls))
		for _, l := range ls {
			flbls[i] = append(flbls[i], &typesv1.LabelPair{Name: l.Name, Value: l.Value})
		}
	}

	// Sort labels as the index writer expects series in sorted order by fingerprint.
	sort.Slice(flbls, func(i, j int) bool {
		return flbls[i].Hash() < flbls[j].Hash()
	})

	symbols := map[string]struct{}{}
	for _, lset := range lbls {
		for _, l := range lset {
			symbols[l.Name] = struct{}{}
			symbols[l.Value] = struct{}{}
		}
	}

	var input index.IndexWriterSeriesSlice

	// Generate ChunkMetas for every label set.
	for i, lset := range flbls {
		var metas []index.ChunkMeta

		for j := 0; j <= (i % 20); j++ {
			metas = append(metas, index.ChunkMeta{
				MinTime:  int64(j * 10000),
				MaxTime:  int64((j + 1) * 10000),
				Checksum: rand.Uint32(),
			})
		}
		input = append(input, &index.IndexWriterSeries{
			Labels: lset,
			Chunks: metas,
		})
	}

	iw, err := NewWriter(context.Background(), BlocksIndexWriterBufSize)
	require.NoError(t, err)

	syms := []string{}
	for s := range symbols {
		syms = append(syms, s)
	}
	sort.Strings(syms)
	for _, s := range syms {
		require.NoError(t, iw.AddSymbol(s))
	}

	// Population procedure as done by compaction.
	var (
		postings = index.NewMemPostings()
		values   = map[string]map[string]struct{}{}
	)

	mi := newMockIndex()

	for i, s := range input {
		err = iw.AddSeries(storage.SeriesRef(i), s.Labels, model.Fingerprint(s.Labels.Hash()), s.Chunks...)
		require.NoError(t, err)
		require.NoError(t, mi.AddSeries(storage.SeriesRef(i), s.Labels, s.Chunks...))

		for _, l := range s.Labels {
			valset, ok := values[l.Name]
			if !ok {
				valset = map[string]struct{}{}
				values[l.Name] = valset
			}
			valset[l.Value] = struct{}{}
		}
		postings.Add(storage.SeriesRef(i), s.Labels)
	}

	err = iw.Close()
	require.NoError(t, err)

	ir, err := NewReader(RealByteSlice(iw.ReleaseIndexBuffer().buf.Bytes()))
	require.NoError(t, err)

	for p := range mi.postings {
		gotp, err := ir.Postings(p.Name, nil, p.Value)
		require.NoError(t, err)

		expp, err := mi.Postings(p.Name, p.Value)
		require.NoError(t, err)

		var lset, explset phlaremodel.Labels
		var chks, expchks []index.ChunkMeta

		for gotp.Next() {
			require.True(t, expp.Next())

			ref := gotp.At()

			_, err := ir.Series(ref, &lset, &chks)
			require.NoError(t, err)

			err = mi.Series(expp.At(), &explset, &expchks)
			require.NoError(t, err)
			require.Equal(t, explset, lset)
			require.Equal(t, expchks, chks)
		}
		require.False(t, expp.Next(), "Expected no more postings for %q=%q", p.Name, p.Value)
		require.NoError(t, gotp.Err())
	}

	labelPairs := map[string][]string{}
	for l := range mi.postings {
		labelPairs[l.Name] = append(labelPairs[l.Name], l.Value)
	}
	for k, v := range labelPairs {
		sort.Strings(v)

		res, err := ir.SortedLabelValues(k)
		require.NoError(t, err)

		require.Equal(t, len(v), len(res))
		for i := 0; i < len(v); i++ {
			require.Equal(t, v[i], res[i])
		}
	}

	gotSymbols := []string{}
	it := ir.Symbols()
	for it.Next() {
		gotSymbols = append(gotSymbols, it.At())
	}
	require.NoError(t, it.Err())
	expSymbols := []string{}
	for s := range mi.symbols {
		expSymbols = append(expSymbols, s)
	}
	sort.Strings(expSymbols)
	require.Equal(t, expSymbols, gotSymbols)

	require.NoError(t, ir.Close())
}

func TestDecbufUvarintWithInvalidBuffer(t *testing.T) {
	b := RealByteSlice([]byte{0x81, 0x81, 0x81, 0x81, 0x81, 0x81})

	db := encoding.NewDecbufUvarintAt(b, 0, castagnoliTable)
	require.Error(t, db.Err())
}

func TestReaderWithInvalidBuffer(t *testing.T) {
	b := RealByteSlice([]byte{0x81, 0x81, 0x81, 0x81, 0x81, 0x81})

	_, err := NewReader(b)
	require.Error(t, err)
}

// TestNewFileReaderErrorNoOpenFiles ensures that in case of an error no file remains open.
func TestNewFileReaderErrorNoOpenFiles(t *testing.T) {
	dir := testutil.NewTemporaryDirectory("block", t)

	idxName := filepath.Join(dir.Path(), "index")
	err := os.WriteFile(idxName, []byte("corrupted contents"), 0o666)
	require.NoError(t, err)

	_, err = NewFileReader(idxName)
	require.Error(t, err)

	// dir.Close will fail on Win if idxName fd is not closed on error path.
	dir.Close()
}

func TestSymbols(t *testing.T) {
	buf := encoding.Encbuf{}

	// Add prefix to the buffer to simulate symbols as part of larger buffer.
	buf.PutUvarintStr("something")

	symbolsStart := buf.Len()
	buf.PutBE32int(204) // Length of symbols table.
	buf.PutBE32int(100) // Number of symbols.
	for i := 0; i < 100; i++ {
		// i represents index in unicode characters table.
		buf.PutUvarintStr(string(rune(i))) // Symbol.
	}
	checksum := crc32.Checksum(buf.Get()[symbolsStart+4:], castagnoliTable)
	buf.PutBE32(checksum) // Check sum at the end.

	s, err := NewSymbols(RealByteSlice(buf.Get()), FormatV2, symbolsStart)
	require.NoError(t, err)

	// We store only 4 offsets to symbols.
	require.Equal(t, 32, s.Size())

	for i := 99; i >= 0; i-- {
		s, err := s.Lookup(uint32(i))
		require.NoError(t, err)
		require.Equal(t, string(rune(i)), s)
	}
	_, err = s.Lookup(100)
	require.Error(t, err)

	for i := 99; i >= 0; i-- {
		r, err := s.ReverseLookup(string(rune(i)))
		require.NoError(t, err)
		require.Equal(t, uint32(i), r)
	}
	_, err = s.ReverseLookup(string(rune(100)))
	require.Error(t, err)

	iter := s.Iter()
	i := 0
	for iter.Next() {
		require.Equal(t, string(rune(i)), iter.At())
		i++
	}
	require.NoError(t, iter.Err())
}

func TestDecoder_Postings_WrongInput(t *testing.T) {
	_, _, err := (&Decoder{}).Postings([]byte("the cake is a lie"))
	require.Error(t, err)
}

func TestWriter_ShouldReturnErrorOnSeriesWithDuplicatedLabelNames(t *testing.T) {
	w, err := NewWriter(context.Background(), BlocksIndexWriterBufSize)
	require.NoError(t, err)

	require.NoError(t, w.AddSymbol("__name__"))
	require.NoError(t, w.AddSymbol("metric_1"))
	require.NoError(t, w.AddSymbol("metric_2"))

	require.NoError(t, w.AddSeries(0, phlaremodel.LabelsFromStrings("__name__", "metric_1", "__name__", "metric_2"), 0))

	err = w.Close()
	require.Error(t, err)
	require.ErrorContains(t, err, "corruption detected when writing postings to index")
}
