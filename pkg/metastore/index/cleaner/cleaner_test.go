package cleaner

import (
	"flag"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestConfig_RegisterFlagsWithPrefix_DefaultCleanupInterval(t *testing.T) {
	var cfg Config
	fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	cfg.RegisterFlagsWithPrefix("metastore.index.", fs)

	require.NoError(t, fs.Parse(nil))
	require.Equal(t, 15*time.Minute, cfg.CleanupInterval)
}

func TestConfig_RegisterFlagsWithPrefix_ZeroCleanupIntervalDisablesCleanup(t *testing.T) {
	var cfg Config
	fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	cfg.RegisterFlagsWithPrefix("metastore.index.", fs)

	require.NoError(t, fs.Parse([]string{"-metastore.index.cleanup-interval=0"}))
	require.Zero(t, cfg.CleanupInterval)
}

func TestConfig_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    time.Duration
		wantMax int
		wantErr bool
	}{
		{
			name:    "integer zero disables cleanup",
			yaml:    "cleanup_interval: 0\n",
			want:    0,
			wantMax: 32,
		},
		{
			name:    "duration zero disables cleanup",
			yaml:    "cleanup_interval: 0s\n",
			want:    0,
			wantMax: 32,
		},
		{
			name:    "quoted zero disables cleanup",
			yaml:    "cleanup_interval: \"0\"\n",
			want:    0,
			wantMax: 32,
		},
		{
			name:    "non-zero duration",
			yaml:    "cleanup_interval: 5m\n",
			want:    5 * time.Minute,
			wantMax: 32,
		},
		{
			name:    "omitted cleanup_interval preserves default",
			yaml:    "cleanup_max_partitions: 10\n",
			want:    DefaultCleanupInterval,
			wantMax: 10,
		},
		{
			name:    "non-zero integer rejected",
			yaml:    "cleanup_interval: 5\n",
			wantErr: true,
		},
		{
			name:    "invalid duration rejected",
			yaml:    "cleanup_interval: invalid\n",
			wantErr: true,
		},
		{
			name:    "unknown field rejected",
			yaml:    "unknown_field: 1\n",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var cfg Config
			fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
			cfg.RegisterFlagsWithPrefix("metastore.index.", fs)

			dec := yaml.NewDecoder(strings.NewReader(tc.yaml))
			dec.KnownFields(true)
			err := dec.Decode(&cfg)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.CleanupInterval)
			require.Equal(t, tc.wantMax, cfg.CleanupMaxPartitions)
			require.Equal(t, 6*time.Hour, cfg.CleanupGracePeriod)
		})
	}
}
