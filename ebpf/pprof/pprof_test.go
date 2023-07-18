package pprof

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/pprof/profile"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

func TestBackAndForth(t *testing.T) {
	const sampleRate = 97
	period := time.Second.Nanoseconds() / int64(sampleRate)

	builders := NewProfileBuilders(97)

	builder := builders.BuilderForTarget(1, labels.Labels{{Name: "foo", Value: "bar"}})
	builder.AddSample([]string{"a", "b", "c"}, 239)
	builder.AddSample([]string{"a", "b", "d"}, 4242)

	buf := bytes.NewBuffer(nil)
	_, err := builder.Write(buf)
	require.NoError(t, err)

	rawProfile := buf.Bytes()

	parsed, err := profile.Parse(bytes.NewBuffer(rawProfile))
	require.NoError(t, err)
	require.NotNil(t, parsed)
	require.Equal(t, 2, len(parsed.Sample))
	require.Equal(t, 4, len(parsed.Function))
	require.Equal(t, 4, len(parsed.Location))

	stacks := map[string]int64{}
	for _, sample := range parsed.Sample {
		stack := ""
		for i, location := range sample.Location {
			if i != 0 {
				stack += ";"
			}
			stack += location.Line[0].Function.Name
		}
		stacks[stack] = sample.Value[0]
	}

	require.Equal(t, 239*period, stacks["a;b;c"])
	require.Equal(t, 4242*period, stacks["a;b;d"])
}
