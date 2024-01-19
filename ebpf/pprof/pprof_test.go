package pprof

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/pprof/profile"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackAndForth(t *testing.T) {
	const sampleRate = 97
	period := time.Second.Nanoseconds() / int64(sampleRate)

	builders := NewProfileBuilders(97)

	builder := builders.BuilderForTarget(1, labels.Labels{{Name: "foo", Value: "bar"}})
	builder.CreateSample([]string{"a", "b", "c"}, 239)
	builder.CreateSample([]string{"a", "b", "d"}, 4242)

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

func TestMergeSamples(t *testing.T) {
	const sampleRate = 97
	period := time.Second.Nanoseconds() / int64(sampleRate)

	builders := NewProfileBuilders(97)

	builder := builders.BuilderForTarget(1, nil)
	builder.CreateSampleOrAddValue([]string{"a", "b", "d"}, 4242)

	for i := 0; i < 14; i++ {
		builder.CreateSampleOrAddValue([]string{"a", "b", "c"}, 239)
	}

	var longStack []string
	for i := 0; i < 512; i++ {
		longStack = append(longStack, fmt.Sprintf("l_%d", i))
	}
	builder.CreateSampleOrAddValue(longStack, 3)
	builder.CreateSampleOrAddValue([]string{"a", "b"}, 42)

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
