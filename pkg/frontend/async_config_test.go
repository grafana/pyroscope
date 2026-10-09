package frontend

import (
	"bytes"
	"flag"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/usage"
)

func TestAsyncQueriesStorageHelp(t *testing.T) {
	var cfg AsyncQueriesConfig
	var output bytes.Buffer
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(&output)
	cfg.RegisterFlags(fs)

	previous := flag.CommandLine
	flag.CommandLine = fs
	t.Cleanup(func() { flag.CommandLine = previous })

	require.NoError(t, usage.Usage(false, &cfg))
	require.NotContains(t, output.String(), "-query-frontend.async-queries.storage.")

	output.Reset()
	require.NoError(t, usage.Usage(true, &cfg))
	fs.VisitAll(func(fl *flag.Flag) {
		require.Contains(t, output.String(), "-"+fl.Name)
	})
	require.Contains(t, output.String(), "[experimental]")
}

func TestLegacyAsyncQueriesEnabledFlag(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want bool
		warn bool
	}{
		{name: "default"},
		{name: "legacy bare", args: []string{"-query-frontend.async-queries-enabled"}, want: true, warn: true},
		{name: "legacy true", args: []string{"-query-frontend.async-queries-enabled=true"}, want: true, warn: true},
		{name: "legacy false", args: []string{"-query-frontend.async-queries-enabled=false"}, warn: true},
		{name: "new flag", args: []string{"-query-frontend.async-queries.enabled=true"}, want: true},
		{name: "new overrides legacy", args: []string{"-query-frontend.async-queries-enabled=true", "-query-frontend.async-queries.enabled=false"}, warn: true},
		{name: "legacy overrides new", args: []string{"-query-frontend.async-queries.enabled=true", "-query-frontend.async-queries-enabled=false"}, warn: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg Config
			var output bytes.Buffer
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			cfg.RegisterFlags(fs, log.NewLogfmtLogger(&output))
			require.NoError(t, fs.Parse(tc.args))
			require.Equal(t, tc.want, cfg.AsyncQueries.Enabled)
			if tc.warn {
				require.Contains(t, output.String(), "level=warn")
				require.Contains(t, output.String(), "-query-frontend.async-queries-enabled is deprecated")
				require.Contains(t, output.String(), "use -query-frontend.async-queries.enabled instead")
			} else {
				require.NotContains(t, output.String(), "deprecated")
			}
		})
	}
}

func TestLegacyAsyncQueriesEnabledFlagInvalid(t *testing.T) {
	var cfg Config
	var output bytes.Buffer
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg.RegisterFlags(fs, log.NewLogfmtLogger(&output))
	require.Error(t, fs.Parse([]string{"-query-frontend.async-queries-enabled=invalid"}))
	require.False(t, cfg.AsyncQueries.Enabled)
	require.NotContains(t, output.String(), "deprecated")
}
