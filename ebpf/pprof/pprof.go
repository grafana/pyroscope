package pprof

import (
	"fmt"
	"io"
	"reflect"
	"sync"
	"time"
	"unsafe"

	"github.com/cespare/xxhash/v2"
	"github.com/google/pprof/profile"
	"github.com/klauspost/compress/gzip"
	"github.com/prometheus/prometheus/model/labels"
)

var (
	gzipWriterPool = sync.Pool{
		New: func() any {
			res, err := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
			if err != nil {
				panic(err)
			}
			return res
		},
	}
)

type ProfileBuilders struct {
	Builders   map[uint64]*ProfileBuilder
	SampleRate int64
}

func NewProfileBuilders(sampleRate int64) *ProfileBuilders {
	return &ProfileBuilders{Builders: make(map[uint64]*ProfileBuilder), SampleRate: sampleRate}
}

func (b ProfileBuilders) BuilderForTarget(hash uint64, labels labels.Labels) *ProfileBuilder {
	res := b.Builders[hash]
	if res != nil {
		return res
	}

	builder := &ProfileBuilder{
		locations:          make(map[string]*profile.Location),
		functions:          make(map[string]*profile.Function),
		sampleHashToSample: make(map[uint64]*profile.Sample),
		Labels:             labels,
		Profile: &profile.Profile{
			Mapping: []*profile.Mapping{
				{
					ID: 1,
				},
			},
			SampleType: []*profile.ValueType{{Type: "cpu", Unit: "nanoseconds"}},
			Period:     time.Second.Nanoseconds() / b.SampleRate,
			PeriodType: &profile.ValueType{Type: "cpu", Unit: "nanoseconds"},
			TimeNanos:  time.Now().UnixNano(),
		},
		hash:           xxhash.New(),
		tmpLocationIDs: make([]uint64, 0, 128),
	}
	res = builder
	b.Builders[hash] = res
	return res
}

type ProfileBuilder struct {
	locations          map[string]*profile.Location
	functions          map[string]*profile.Function
	sampleHashToSample map[uint64]*profile.Sample
	Profile            *profile.Profile
	Labels             labels.Labels

	hash           *xxhash.Digest
	b              [8]byte
	tmpLocations   []*profile.Location
	tmpLocationIDs []uint64
}

func (p *ProfileBuilder) CreateSample(stacktrace []string, value uint64) {
	sample := &profile.Sample{
		Value: []int64{int64(value) * p.Profile.Period},
	}
	for _, s := range stacktrace {
		loc := p.addLocation(s)
		sample.Location = append(sample.Location, loc)
	}
	p.Profile.Sample = append(p.Profile.Sample, sample)
}

func (p *ProfileBuilder) CreateSampleOrAddValue(stacktrace []string, value uint64) {
	scaledValue := int64(value) * p.Profile.Period
	if cap(p.tmpLocations) < len(stacktrace) {
		p.tmpLocations = make([]*profile.Location, 0, len(stacktrace))
	} else {
		p.tmpLocations = p.tmpLocations[:0]
	}
	if cap(p.tmpLocationIDs) < len(stacktrace) {
		p.tmpLocationIDs = make([]uint64, 0, len(stacktrace))
	} else {
		p.tmpLocationIDs = p.tmpLocationIDs[:0]
	}
	for _, s := range stacktrace {
		loc := p.addLocation(s)
		p.tmpLocations = append(p.tmpLocations, loc)
		p.tmpLocationIDs = append(p.tmpLocationIDs, loc.ID)
	}
	p.hash.Reset()
	if _, err := p.hash.Write(uint64Bytes(p.tmpLocationIDs)); err != nil {
		panic(err)
	}
	h := p.hash.Sum64()
	sample := p.sampleHashToSample[h]
	if sample != nil {
		sample.Value[0] += scaledValue
		return
	}
	sample = &profile.Sample{
		Location: p.tmpLocations,
		Value:    []int64{scaledValue},
	}
	p.sampleHashToSample[h] = sample
	p.tmpLocations = nil
	p.Profile.Sample = append(p.Profile.Sample, sample)
}

func (p *ProfileBuilder) addLocation(function string) *profile.Location {
	loc, ok := p.locations[function]
	if ok {
		return loc
	}

	id := uint64(len(p.Profile.Location) + 1)
	loc = &profile.Location{
		ID:      id,
		Mapping: p.Profile.Mapping[0],
		Line: []profile.Line{
			{
				Function: p.addFunction(function),
			},
		},
	}
	p.Profile.Location = append(p.Profile.Location, loc)
	p.locations[function] = loc
	return loc
}

func (p *ProfileBuilder) addFunction(function string) *profile.Function {
	f, ok := p.functions[function]
	if ok {
		return f
	}

	id := uint64(len(p.Profile.Function) + 1)
	f = &profile.Function{
		ID:   id,
		Name: function,
	}
	p.Profile.Function = append(p.Profile.Function, f)
	p.functions[function] = f
	return f
}

func (p *ProfileBuilder) Write(dst io.Writer) (int64, error) {
	gzipWriter := gzipWriterPool.Get().(*gzip.Writer)
	gzipWriter.Reset(dst)
	defer func() {
		gzipWriter.Reset(io.Discard)
		gzipWriterPool.Put(gzipWriter)
	}()
	err := p.Profile.WriteUncompressed(gzipWriter)
	if err != nil {
		return 0, fmt.Errorf("ebpf profile encode %w", err)
	}
	err = gzipWriter.Close()
	if err != nil {
		return 0, fmt.Errorf("ebpf profile encode %w", err)
	}
	return 0, nil
}

func uint64Bytes(s []uint64) []byte {
	if len(s) == 0 {
		return nil
	}
	var bs []byte
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&bs))
	hdr.Len = len(s) * 8
	hdr.Cap = hdr.Len
	hdr.Data = uintptr(unsafe.Pointer(&s[0]))
	return bs
}
