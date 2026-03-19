package readpath

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestQueryBackendFrom_Set(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantAuto bool
		wantTime time.Time
		wantErr  bool
	}{
		{
			name:     "auto",
			input:    "auto",
			wantAuto: true,
		},
		{
			name:     "RFC3339 timestamp",
			input:    "2025-01-15T10:30:00Z",
			wantTime: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:     "date only",
			input:    "2025-01-15",
			wantTime: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "zero",
			input:    "0",
			wantTime: time.Time{},
		},
		{
			name:    "invalid",
			input:   "not-a-date",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var q QueryBackendFrom
			err := q.Set(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantAuto, q.Auto)
			if !tt.wantAuto {
				assert.True(t, tt.wantTime.Equal(q.Time), "expected %v, got %v", tt.wantTime, q.Time)
			}
		})
	}
}

func TestQueryBackendFrom_String(t *testing.T) {
	assert.Equal(t, "auto", QueryBackendFrom{Auto: true}.String())
	assert.Equal(t, "0", QueryBackendFrom{}.String())
}

func TestQueryBackendFrom_IsZero(t *testing.T) {
	assert.True(t, QueryBackendFrom{}.IsZero())
	assert.False(t, QueryBackendFrom{Auto: true}.IsZero())
	assert.False(t, QueryBackendFrom{Time: time.Now()}.IsZero())
}

func TestQueryBackendFrom_JSON(t *testing.T) {
	tests := []struct {
		name  string
		value QueryBackendFrom
		json  string
	}{
		{
			name:  "auto",
			value: QueryBackendFrom{Auto: true},
			json:  `"auto"`,
		},
		{
			name:  "zero",
			value: QueryBackendFrom{},
			json:  `"0"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.json, string(data))

			var decoded QueryBackendFrom
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.value.Auto, decoded.Auto)
		})
	}
}

func TestQueryBackendFrom_YAML(t *testing.T) {
	tests := []struct {
		name  string
		value QueryBackendFrom
		yaml  string
	}{
		{
			name:  "auto",
			value: QueryBackendFrom{Auto: true},
			yaml:  "auto\n",
		},
		{
			name:  "zero",
			value: QueryBackendFrom{},
			yaml:  "\"0\"\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := yaml.Marshal(tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.yaml, string(data))

			var decoded QueryBackendFrom
			err = yaml.Unmarshal(data, &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.value.Auto, decoded.Auto)
		})
	}
}

func TestQueryBackendFrom_SplitTime(t *testing.T) {
	t.Run("fixed time", func(t *testing.T) {
		ts := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
		q := QueryBackendFrom{Time: ts}
		result, err := q.SplitTime(func() (time.Time, error) {
			t.Fatal("should not be called")
			return time.Time{}, nil
		})
		require.NoError(t, err)
		assert.True(t, ts.Equal(result))
	})

	t.Run("auto resolves from callback", func(t *testing.T) {
		expected := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
		q := QueryBackendFrom{Auto: true}
		result, err := q.SplitTime(func() (time.Time, error) {
			return expected, nil
		})
		require.NoError(t, err)
		assert.True(t, expected.Equal(result))
	})
}
