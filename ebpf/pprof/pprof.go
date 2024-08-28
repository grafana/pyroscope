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
	"github.com/grafana/pyroscope/ebpf/sd"
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

type SampleType uint32

var SampleTypeCpu = SampleType(0)
var SampleTypeMem = SampleType(1)

type SampleAggregation bool

var (
	// SampleAggregated mean samples are accumulated in ebpf, no need to dedup these
	SampleAggregated = SampleAggregation(true)
)

type CollectProfilesCallback func(sample ProfileSample)

type SamplesCollector interface {
	CollectProfiles(callback CollectProfilesCallback) error
}

type ProfileSample struct {
	Target      *sd.Target
	Pid         uint32
	SampleType  SampleType
	Aggregation SampleAggregation
	Stack       []string
	Value       uint64
	Value2      uint64
}

type BuildersOptions struct {
	SampleRate    int64
	PerPIDProfile bool
}

type builderHashKey struct {
	labelsHash uint64
	pid        uint32
	sampleType SampleType
}

type ProfileBuilders struct {
	Builders map[builderHashKey]*ProfileBuilder
	opt      BuildersOptions
}

func NewProfileBuilders(options BuildersOptions) *ProfileBuilders {
	return &ProfileBuilders{Builders: make(map[builderHashKey]*ProfileBuilder), opt: options}
}

func Collect(builders *ProfileBuilders, collector SamplesCollector) error {
	return collector.CollectProfiles(func(sample ProfileSample) {
		builders.AddSample(&sample)
	})
}

func (b *ProfileBuilders) AddSample(sample *ProfileSample) {
	bb := b.BuilderForSample(sample)
	if sample.Aggregation == SampleAggregated {
		bb.CreateSample(sample)
	} else {
		bb.CreateSampleOrAddValue(sample)
	}
}

func (b *ProfileBuilders) BuilderForSample(sample *ProfileSample) *ProfileBuilder {
	labelsHash, labels := sample.Target.Labels()

	k := builderHashKey{labelsHash: labelsHash, sampleType: sample.SampleType}
	if b.opt.PerPIDProfile {
		k.pid = sample.Pid
	}
	res := b.Builders[k]
	if res != nil {
		return res
	}

	var sampleType []*profile.ValueType
	var periodType *profile.ValueType
	var period int64
	if sample.SampleType == SampleTypeCpu {
		sampleType = []*profile.ValueType{{Type: "cpu", Unit: "nanoseconds"}}
		periodType = &profile.ValueType{Type: "cpu", Unit: "nanoseconds"}
		period = time.Second.Nanoseconds() / b.opt.SampleRate
	} else {
		sampleType = []*profile.ValueType{{Type: "alloc_objects", Unit: "count"}, {Type: "alloc_space", Unit: "bytes"}}
		periodType = &profile.ValueType{Type: "space", Unit: "bytes"}
		period = 512 * 1024 // todo
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
			SampleType: sampleType,
			Period:     period,
			PeriodType: periodType,
			TimeNanos:  time.Now().UnixNano(),
		},
		tmpLocationIDs: make([]uint64, 0, 128),
		tmpLocations:   make([]*profile.Location, 0, 128),
	}
	res = builder
	b.Builders[k] = res
	return res
}

type ProfileBuilder struct {
	locations          map[string]*profile.Location
	functions          map[string]*profile.Function
	sampleHashToSample map[uint64]*profile.Sample
	Profile            *profile.Profile
	Labels             labels.Labels

	tmpLocations   []*profile.Location
	tmpLocationIDs []uint64
}

func (p *ProfileBuilder) CreateSample(inputSample *ProfileSample) {
	sample := p.newSample(inputSample)
	p.addValue(inputSample, sample)
	for i, s := range inputSample.Stack {
		sample.Location[i] = p.addLocation(s)
	}
	p.Profile.Sample = append(p.Profile.Sample, sample)
}

func (p *ProfileBuilder) CreateSampleOrAddValue(inputSample *ProfileSample) {
	p.tmpLocations = p.tmpLocations[:0]
	p.tmpLocationIDs = p.tmpLocationIDs[:0]
	for _, s := range inputSample.Stack {
		loc := p.addLocation(s)
		p.tmpLocations = append(p.tmpLocations, loc)
		p.tmpLocationIDs = append(p.tmpLocationIDs, loc.ID)
	}
	h := xxhash.Sum64(uint64Bytes(p.tmpLocationIDs))
	sample := p.sampleHashToSample[h]
	if sample != nil {
		p.addValue(inputSample, sample)
		return
	}
	sample = p.newSample(inputSample)
	p.addValue(inputSample, sample)
	copy(sample.Location, p.tmpLocations)
	p.sampleHashToSample[h] = sample
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
func (p *ProfileBuilder) newSample(inputSample *ProfileSample) *profile.Sample {
	sample := new(profile.Sample)
	if inputSample.SampleType == SampleTypeCpu {
		sample.Value = []int64{0}
	} else {
		sample.Value = []int64{0, 0}
	}
	sample.Location = make([]*profile.Location, len(inputSample.Stack))
	return sample
}

func (p *ProfileBuilder) addValue(inputSample *ProfileSample, sample *profile.Sample) {
	if inputSample.SampleType == SampleTypeCpu {
		sample.Value[0] += int64(inputSample.Value) * p.Profile.Period
	} else {
		sample.Value[0] += int64(inputSample.Value)
		sample.Value[1] += int64(inputSample.Value2)
	}
}
