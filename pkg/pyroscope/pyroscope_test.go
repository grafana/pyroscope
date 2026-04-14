package pyroscope

import (
	"bytes"
	"context"
	"flag"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	statusv1 "github.com/grafana/pyroscope/api/gen/proto/go/status/v1"
)

func TestFlagDefaults(t *testing.T) {
	c := Config{}

	f := flag.NewFlagSet("test", flag.PanicOnError)
	c.RegisterFlags(f)

	buf := bytes.Buffer{}

	f.SetOutput(&buf)
	f.PrintDefaults()

	const delim = '\n'
	// Because this is a short flag, it will be printed on the same line as the
	// flag name. So we need to ignore this special case.
	const ignoredHelpFlags = "-h\tPrint basic help."

	// Populate map with parsed default flags.
	// LabelSet is the flag and value is the default text.
	gotFlags := make(map[string]string)
	for {
		line, err := buf.ReadString(delim)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		if strings.Contains(line, ignoredHelpFlags) {
			continue
		}

		nextLine, err := buf.ReadString(delim)
		require.NoError(t, err)

		trimmedLine := strings.Trim(line, " \n")
		splittedLine := strings.Split(trimmedLine, " ")[0]
		gotFlags[splittedLine] = nextLine
	}

	flagToCheck := "-server.http-listen-port"
	require.Contains(t, gotFlags, flagToCheck)
	require.Equal(t, c.Server.HTTPListenPort, 4040)
	require.Contains(t, gotFlags[flagToCheck], "(default 4040)")
}

// newTestConfig creates a Config with flags registered and parsed.
func newTestConfig(t *testing.T, args []string) Config {
	t.Helper()
	cfg := Config{}
	fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	cfg.RegisterFlags(fs)
	require.NoError(t, fs.Parse(args))
	return cfg
}

func TestSetupModuleManager_V2_ExcludesV1Components(t *testing.T) {
	t.Run("excludes V1 by default", func(t *testing.T) {
		cfg := newTestConfig(t, []string{})
		f := &Pyroscope{Cfg: cfg}
		require.NoError(t, f.setupModuleManager())

		allDeps := f.deps[All]
		for _, mod := range []string{Ingester, Compactor, Querier, QueryScheduler, StoreGateway} {
			assert.False(t, slices.Contains(allDeps, mod), "%s should not be in All deps", mod)
		}
		for _, mod := range []string{SegmentWriter, Metastore, CompactionWorker, QueryBackend} {
			assert.True(t, slices.Contains(allDeps, mod), "%s should be in All deps", mod)
		}
	})

	t.Run("includes all components when dual mode is enabled", func(t *testing.T) {
		cfg := newTestConfig(t, []string{"-architecture.storage=v1-v2-dual"})
		f := &Pyroscope{Cfg: cfg}
		require.NoError(t, f.setupModuleManager())

		allDeps := f.deps[All]
		for _, mod := range []string{Ingester, Compactor, Querier, QueryScheduler, StoreGateway} {
			assert.True(t, slices.Contains(allDeps, mod), "%s should be in All deps", mod)
		}
		for _, mod := range []string{SegmentWriter, Metastore, CompactionWorker, QueryBackend} {
			assert.True(t, slices.Contains(allDeps, mod), "%s should be in All deps", mod)
		}
	})

	t.Run("exclude V2 when legacy storage", func(t *testing.T) {
		cfg := newTestConfig(t, []string{"-architecture.storage=v1"})
		f := &Pyroscope{Cfg: cfg}
		require.NoError(t, f.setupModuleManager())

		allDeps := f.deps[All]
		for _, mod := range []string{Ingester, Compactor, Querier, QueryScheduler, StoreGateway} {
			assert.True(t, slices.Contains(allDeps, mod), "%s should be in All deps", mod)
		}
		for _, mod := range []string{SegmentWriter, Metastore, CompactionWorker, QueryBackend} {
			assert.False(t, slices.Contains(allDeps, mod), "%s should not be in All deps", mod)
		}
	})
}

func TestRegisterServerFlagsWithChangedDefaultValues_V2(t *testing.T) {
	t.Run("registers default v2 architecture flag with default value", func(t *testing.T) {
		cfg := newTestConfig(t, []string{})
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		cfg.RegisterFlagsWithContext(fs)

		archStorage := fs.Lookup("architecture.storage")
		require.NotNil(t, archStorage)
		assert.Equal(t, "v2", archStorage.DefValue)
	})

	t.Run("V2 applies additional default overrides", func(t *testing.T) {
		cfg := newTestConfig(t, []string{})
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		cfg.RegisterFlagsWithContext(fs)

		grpcRecv := fs.Lookup("server.grpc-max-recv-msg-size-bytes")
		require.NotNil(t, grpcRecv)
		assert.Equal(t, "104857600", grpcRecv.DefValue)
	})
}

func TestConfigDiff(t *testing.T) {
	defaultCfg := Config{}
	f := flag.NewFlagSet("test", flag.PanicOnError)
	defaultCfg.RegisterFlags(f)
	require.NoError(t, f.Parse([]string{}))
	phlare, err := New(defaultCfg)
	require.NoError(t, err)

	t.Run("default config unchanged", func(t *testing.T) {
		result, err := phlare.statusService().GetDiffConfig(context.Background(), &statusv1.GetConfigRequest{})
		require.NoError(t, err)
		require.Equal(t, "text/plain; charset=utf-8", result.ContentType)
		require.Equal(t, "{}\n", string(result.Data))
	})
	t.Run("change a limit", func(t *testing.T) {
		phlare.Cfg.LimitsConfig.MaxLabelNameLength = 123

		result, err := phlare.statusService().GetDiffConfig(context.Background(), &statusv1.GetConfigRequest{})
		require.NoError(t, err)
		require.Equal(t, "text/plain; charset=utf-8", result.ContentType)
		require.Equal(t, "limits:\n    max_label_name_length: 123\n", string(result.Data))
	})
}
