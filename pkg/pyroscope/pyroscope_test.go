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
func newTestConfig(t *testing.T, v2 bool, args []string) Config {
	t.Helper()
	cfg := Config{V2: v2}
	fs := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	cfg.RegisterFlags(fs)
	require.NoError(t, fs.Parse(args))
	return cfg
}

func TestSetupModuleManager_V2_ExcludesV1Components(t *testing.T) {
	t.Run("excludes V1 write path when disabled", func(t *testing.T) {
		cfg := newTestConfig(t, true, []string{"-all.enable-v1-write-path=false"})
		f := &Pyroscope{Cfg: cfg}
		require.NoError(t, f.setupModuleManager())

		allDeps := f.deps[All]
		assert.False(t, slices.Contains(allDeps, Ingester), "Ingester should not be in All deps")
		assert.False(t, slices.Contains(allDeps, Compactor), "Compactor should not be in All deps")
		assert.True(t, slices.Contains(allDeps, SegmentWriter), "SegmentWriter should still be in All deps")
		assert.True(t, slices.Contains(allDeps, CompactionWorker), "CompactionWorker should still be in All deps")
	})

	t.Run("excludes V1 read path when disabled", func(t *testing.T) {
		cfg := newTestConfig(t, true, []string{"-all.enable-v1-read-path=false"})
		f := &Pyroscope{Cfg: cfg}
		require.NoError(t, f.setupModuleManager())

		allDeps := f.deps[All]
		assert.False(t, slices.Contains(allDeps, Querier), "Querier should not be in All deps")
		assert.False(t, slices.Contains(allDeps, QueryScheduler), "QueryScheduler should not be in All deps")
		assert.False(t, slices.Contains(allDeps, StoreGateway), "StoreGateway should not be in All deps")
		assert.True(t, slices.Contains(allDeps, QueryBackend), "QueryBackend should still be in All deps")
	})

	t.Run("includes all components when both V1 paths enabled", func(t *testing.T) {
		cfg := newTestConfig(t, true, []string{
			"-all.enable-v1-write-path=true",
			"-all.enable-v1-read-path=true",
		})
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

	t.Run("non-V2 has no V2 modules", func(t *testing.T) {
		cfg := newTestConfig(t, false, nil)
		f := &Pyroscope{Cfg: cfg}
		require.NoError(t, f.setupModuleManager())

		allDeps := f.deps[All]
		assert.True(t, slices.Contains(allDeps, Ingester))
		assert.True(t, slices.Contains(allDeps, Compactor))
		assert.True(t, slices.Contains(allDeps, Querier))
		assert.False(t, slices.Contains(allDeps, SegmentWriter))
		assert.False(t, slices.Contains(allDeps, QueryBackend))
	})
}

func TestRegisterServerFlagsWithChangedDefaultValues_V2(t *testing.T) {
	t.Run("registers enable-v1 flags with default true", func(t *testing.T) {
		cfg := Config{V2: true}
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		cfg.RegisterFlagsWithContext(fs)

		writePath := fs.Lookup("all.enable-v1-write-path")
		require.NotNil(t, writePath)
		assert.Equal(t, "true", writePath.DefValue)

		readPath := fs.Lookup("all.enable-v1-read-path")
		require.NotNil(t, readPath)
		assert.Equal(t, "true", readPath.DefValue)
	})

	t.Run("non-V2 does not register enable-v1 flags", func(t *testing.T) {
		cfg := Config{V2: false}
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		cfg.RegisterFlagsWithContext(fs)

		assert.Nil(t, fs.Lookup("all.enable-v1-write-path"))
		assert.Nil(t, fs.Lookup("all.enable-v1-read-path"))
	})

	t.Run("V2 applies additional default overrides", func(t *testing.T) {
		cfg := Config{V2: true}
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
