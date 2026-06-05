package speedscope

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/api/model/labelset"
	"github.com/grafana/pyroscope/v2/pkg/og/ingestion"
	"github.com/grafana/pyroscope/v2/pkg/og/storage"
	"github.com/grafana/pyroscope/v2/pkg/og/storage/metadata"
)

type mockIngester struct{ actual []*storage.PutInput }

func (m *mockIngester) Put(_ context.Context, p *storage.PutInput) error {
	m.actual = append(m.actual, p)
	return nil
}

func findInputByLabel(inputs []*storage.PutInput, normalizedLabel string) *storage.PutInput {
	for _, in := range inputs {
		if in.LabelSet.Normalized() == normalizedLabel {
			return in
		}
	}
	return nil
}

const expectedTreeResult = `a;b 500
a;b;c 500
a;b;d 400
`

func TestSpeedscope(t *testing.T) {
	t.Run("Can parse an event-format profile", func(t *testing.T) {
		data, err := os.ReadFile("testdata/simple.speedscope.json")
		require.NoError(t, err)

		key, err := labelset.Parse("foo")
		require.NoError(t, err)

		ingester := new(mockIngester)
		profile := &RawProfile{RawData: data}

		md := ingestion.Metadata{LabelSet: key, SampleRate: 100}
		err = profile.Parse(context.Background(), ingester, nil, md)
		require.NoError(t, err)

		require.Len(t, ingester.actual, 1)
		input := ingester.actual[0]

		require.Equal(t, metadata.SamplesUnits, input.Units)
		require.Equal(t, "foo{profile_name=simple.txt}", input.LabelSet.Normalized())
		require.Equal(t, expectedTreeResult, input.Val.String())
		require.Equal(t, uint32(10000), input.SampleRate)
	})

	t.Run("Can parse a sample-format profile", func(t *testing.T) {
		data, err := os.ReadFile("testdata/two-sampled.speedscope.json")
		require.NoError(t, err)

		key, err := labelset.Parse("foo{x=y}")
		require.NoError(t, err)

		ingester := new(mockIngester)
		profile := &RawProfile{RawData: data}

		md := ingestion.Metadata{LabelSet: key, SampleRate: 100}
		err = profile.Parse(context.Background(), ingester, nil, md)
		require.NoError(t, err)

		require.Len(t, ingester.actual, 2)

		input := findInputByLabel(ingester.actual, "foo.seconds{profile_name=one,x=y}")
		require.NotNil(t, input)
		require.Equal(t, metadata.SamplesUnits, input.Units)
		require.Equal(t, "foo.seconds{profile_name=one,x=y}", input.LabelSet.Normalized())
		require.Equal(t, expectedTreeResult, input.Val.String())
		require.Equal(t, uint32(100), input.SampleRate)

		input2 := findInputByLabel(ingester.actual, "foo.seconds{profile_name=two,x=y}")
		require.NotNil(t, input2)
		require.Equal(t, metadata.SamplesUnits, input2.Units)
		require.Equal(t, "foo.seconds{profile_name=two,x=y}", input2.LabelSet.Normalized())
		require.Equal(t, expectedTreeResult, input2.Val.String())
		require.Equal(t, uint32(100), input2.SampleRate)
	})

	t.Run("Returns error for unknown unit in defaultSampleRate", func(t *testing.T) {
		u := unit("UNKNOWN_UNIT")
		_, err := u.defaultSampleRate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown unit")
	})

	t.Run("Returns error for unknown unit in precisionMultiplier", func(t *testing.T) {
		u := unit("UNKNOWN_UNIT")
		_, err := u.precisionMultiplier()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown unit")
	})

	t.Run("Returns error instead of panicking for unknown unit in sampled profile", func(t *testing.T) {
		data := []byte(`{"$schema":"https://www.speedscope.app/file-format-schema.json","shared":{"frames":[{"name":"main"}]},"profiles":[{"type":"sampled","unit":"TRIGGER_PANIC_UNKNOWN_UNIT","name":"poc","startValue":0,"endValue":1,"samples":[[0]],"weights":[1]}]}`)

		key, err := labelset.Parse("foo")
		require.NoError(t, err)

		ingester := new(mockIngester)
		profile := &RawProfile{RawData: data}

		md := ingestion.Metadata{LabelSet: key, SampleRate: 100}
		err = profile.Parse(context.Background(), ingester, nil, md)
		require.Error(t, err)
		require.Empty(t, ingester.actual)
	})

	t.Run("Returns error instead of panicking for unknown unit in evented profile", func(t *testing.T) {
		data := []byte(`{"$schema":"https://www.speedscope.app/file-format-schema.json","shared":{"frames":[{"name":"a"}]},"profiles":[{"type":"evented","unit":"TRIGGER_PANIC_UNKNOWN_UNIT","name":"poc","startValue":0,"endValue":1,"events":[{"type":"O","frame":0,"at":0},{"type":"C","frame":0,"at":1}]}]}`)

		key, err := labelset.Parse("foo")
		require.NoError(t, err)

		ingester := new(mockIngester)
		profile := &RawProfile{RawData: data}

		md := ingestion.Metadata{LabelSet: key, SampleRate: 100}
		err = profile.Parse(context.Background(), ingester, nil, md)
		require.Error(t, err)
		require.Empty(t, ingester.actual)
	})

	t.Run("Merges duplicate profiles", func(t *testing.T) {
		data, err := os.ReadFile("testdata/duplicates.speedscope.json")
		require.NoError(t, err)

		key, err := labelset.Parse("foo{x=y}")
		require.NoError(t, err)

		ingester := new(mockIngester)
		profile := &RawProfile{RawData: data}

		md := ingestion.Metadata{LabelSet: key, SampleRate: 100}
		err = profile.Parse(context.Background(), ingester, nil, md)
		require.NoError(t, err)

		require.Len(t, ingester.actual, 1)

		input := ingester.actual[0]
		require.Equal(t, metadata.SamplesUnits, input.Units)
		require.Equal(t, "foo{profile_name=one,x=y}", input.LabelSet.Normalized())
		expectedResult := `a;b 1500
a;b;c 1500
a;b;d 1200
`
		require.Equal(t, expectedResult, input.Val.String())
		require.Equal(t, uint32(100), input.SampleRate)
	})
}
