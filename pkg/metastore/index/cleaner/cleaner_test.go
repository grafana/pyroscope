package cleaner

import (
	"flag"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
