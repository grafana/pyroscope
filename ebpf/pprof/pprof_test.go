package pprof

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/pprof/profile"
	"github.com/grafana/pyroscope/ebpf/sd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackAndForth(t *testing.T) {
	const sampleRate = 97
	period := time.Second.Nanoseconds() / int64(sampleRate)

	builders := NewProfileBuilders(BuildersOptions{
		SampleRate:    int64(97),
		PerPIDProfile: false,
	})

	builders.AddSample(
		sample([]string{"a", "b", "c"}, 239))
	builders.AddSample(
		sample([]string{"a", "b", "d"}, 4242))

	builder := builders.BuilderForSample(sample([]string{"a", "b", "c"}, 0))

	buf := bytes.NewBuffer(nil)

	_, err := builder.Write(buf)
	assert.NoError(t, err)

	rawProfile := buf.Bytes()

	parsed, err := profile.Parse(bytes.NewBuffer(rawProfile))
	assert.NoError(t, err)
	require.NotNil(t, parsed)
	assert.Equal(t, 2, len(parsed.Sample))
	assert.Equal(t, 4, len(parsed.Function))
	assert.Equal(t, 4, len(parsed.Location))

	stacks := stackCollapse(parsed)

	assert.Equal(t, 239*period, stacks["a;b;c"])
	assert.Equal(t, 4242*period, stacks["a;b;d"])
}

var testTarget = sd.NewTarget("", 1, sd.DiscoveryTarget{"foo": "bar"})

func sample(stack []string, v uint64) *ProfileSample {
	return &ProfileSample{
		Pid:         1,
		Target:      testTarget,
		SampleType:  SampleTypeCpu,
		Aggregation: SampleAggregated,
		Stack:       stack,
		Value:       v,
	}
}

func TestMergeSamples(t *testing.T) {
	const sampleRate = 97
	period := time.Second.Nanoseconds() / int64(sampleRate)

	builders := NewProfileBuilders(BuildersOptions{
		SampleRate: int64(97),
	})

	builder := builders.BuilderForSample(sample([]string{"a", "b", "c"}, 0))

	builder.CreateSampleOrAddValue(sample([]string{"a", "b", "d"}, 4242))

	for i := 0; i < 14; i++ {
		builder.CreateSampleOrAddValue(sample([]string{"a", "b", "c"}, 239))
	}

	var longStack []string
	for i := 0; i < 512; i++ {
		longStack = append(longStack, fmt.Sprintf("l_%d", i))
	}
	builder.CreateSampleOrAddValue(sample(longStack, 3))
	builder.CreateSampleOrAddValue(sample([]string{"a", "b"}, 42))

	assert.Equal(t, 4, len(builder.Profile.Sample))

	buf := bytes.NewBuffer(nil)
	_, err := builder.Write(buf)
	assert.NoError(t, err)
	rawProfile := buf.Bytes()

	parsed, err := profile.Parse(bytes.NewBuffer(rawProfile))
	assert.NoError(t, err)
	require.NotNil(t, parsed)
	assert.Equal(t, 4, len(parsed.Sample))
	assert.Equal(t, 4+512, len(parsed.Function))
	assert.Equal(t, 4+512, len(parsed.Location))

	stacks := stackCollapse(parsed)

	assert.Equal(t, 14*239*period, stacks["a;b;c"])
	assert.Equal(t, 4242*period, stacks["a;b;d"])
	assert.Equal(t, 42*period, stacks["a;b"])
	assert.Equal(t, 3*period, stacks[strings.Join(longStack, ";")])
	assert.Equal(t, 4, len(parsed.Sample))
	if t.Failed() {
		for s, i := range stacks {
			t.Logf("%s: %d", s, i)
		}
	}
}

func stackCollapse(parsed *profile.Profile) map[string]int64 {
	stacks := map[string]int64{}
	for _, sample := range parsed.Sample {
		stack := ""
		for i, location := range sample.Location {
			if i != 0 {
				stack += ";"
			}
			stack += location.Line[0].Function.Name
		}
		stacks[stack] += sample.Value[0]
	}
	return stacks
}
