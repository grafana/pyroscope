package pyroscope

import (
	"bytes"
	"context"
	"flag"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	statusv1 "github.com/grafana/pyroscope/api/gen/proto/go/status/v1"
	configpkg "github.com/grafana/pyroscope/v2/pkg/cfg"
	objstoreclient "github.com/grafana/pyroscope/v2/pkg/objstore/client"
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
		cfg := newTestConfig(t, []string{"-architecture.storage=v2"})
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
		assert.Equal(t, "v1-v2-dual", archStorage.DefValue)
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

func TestRegisterFlags_MarksV1StorageOnlyFlags(t *testing.T) {
	cfg := Config{}
	fs := flag.NewFlagSet("test", flag.PanicOnError)
	cfg.RegisterFlags(fs)

	seenPrefixes := make(map[string]bool, len(v1StorageOnlyFlagPrefixes))
	fs.VisitAll(func(f *flag.Flag) {
		for _, prefix := range v1StorageOnlyFlagPrefixes {
			if strings.HasPrefix(f.Name, prefix) {
				seenPrefixes[prefix] = true
				assert.Contains(t, f.Usage, v1StorageOnlyFlagUsagePrefix, "flag %s should be marked as V1-only", f.Name)
			}
		}
	})

	for _, prefix := range v1StorageOnlyFlagPrefixes {
		assert.True(t, seenPrefixes[prefix], "expected at least one registered flag with prefix %s", prefix)
	}
}

func TestDynamicUnmarshalRecordsSetFlags(t *testing.T) {
	var cfg Config
	fs := flag.NewFlagSet("test", flag.ContinueOnError)

	require.NoError(t, configpkg.DynamicUnmarshal(&cfg, []string{
		"-architecture.storage=v2",
		"-compactor.data-dir=/tmp/compactor",
		"-pyroscopedb.data-path=/tmp/pyroscopedb",
	}, fs))

	assert.Contains(t, cfg.SetFlags, "architecture.storage")
	assert.Contains(t, cfg.SetFlags, "compactor.data-dir")
	assert.Contains(t, cfg.SetFlags, "pyroscopedb.data-path")
	assert.ElementsMatch(t, []string{"compactor.data-dir", "pyroscopedb.data-path"}, cfg.setV1StorageOnlyFlags())
}

func TestConfig_WarnAboutV1StorageOnlyFlags(t *testing.T) {
	t.Run("warns when v1-only flags are set with v2 storage", func(t *testing.T) {
		cfg := Config{ArchitectureStorage: V2}
		cfg.RecordSetFlag("compactor.data-dir")
		cfg.RecordSetFlag("pyroscopedb.data-path")
		cfg.RecordSetFlag("query-frontend.max-async-query-concurrency")

		var buf bytes.Buffer
		cfg.warnAboutV1StorageOnlyFlags(log.NewLogfmtLogger(&buf))

		output := buf.String()
		assert.Contains(t, output, "flag=compactor.data-dir")
		assert.Contains(t, output, "flag=pyroscopedb.data-path")
		assert.NotContains(t, output, "query-frontend.max-async-query-concurrency")
	})

	t.Run("does not warn when v1 storage is active", func(t *testing.T) {
		cfg := Config{ArchitectureStorage: V1}
		cfg.RecordSetFlag("compactor.data-dir")

		var buf bytes.Buffer
		cfg.warnAboutV1StorageOnlyFlags(log.NewLogfmtLogger(&buf))

		assert.Empty(t, buf.String())
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

func TestConfigValidate_StorageBackendRequired(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantErr        bool
		wantErrContain string
	}{
		{
			name:           "v2 with no backend errors",
			args:           []string{"-architecture.storage=v2", "-storage.backend=", "-write-path=segment-writer"},
			wantErr:        true,
			wantErrContain: "storage.backend is required",
		},
		{
			name:           "v1-v2-dual with no backend errors",
			args:           []string{"-architecture.storage=v1-v2-dual", "-storage.backend=", "-write-path=segment-writer"},
			wantErr:        true,
			wantErrContain: "storage.backend is required",
		},
		{
			name:    "v1 with no backend is allowed",
			args:    []string{"-architecture.storage=v1", "-storage.backend=", "-write-path=ingester"},
			wantErr: false,
		},
		{
			name:    "v2 with filesystem backend is valid",
			args:    []string{"-architecture.storage=v2", "-storage.backend=" + objstoreclient.Filesystem, "-write-path=segment-writer"},
			wantErr: false,
		},
		{
			name:    "v1-v2-dual with filesystem backend is valid",
			args:    []string{"-architecture.storage=v1-v2-dual", "-storage.backend=" + objstoreclient.Filesystem, "-write-path=segment-writer"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newTestConfig(t, tt.args)
			err := cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrContain)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfigValidate_V2IgnoresV1OnlyCompactorConfig(t *testing.T) {
	t.Run("v2 ignores invalid V1 compactor block range", func(t *testing.T) {
		cfg := newTestConfig(t, []string{
			"-architecture.storage=v2",
			"-storage.backend=" + objstoreclient.Filesystem,
			"-write-path=segment-writer",
			"-compactor.block-ranges=30m",
		})

		require.NoError(t, cfg.Validate())
	})

	t.Run("v1 validates invalid V1 compactor block range", func(t *testing.T) {
		cfg := newTestConfig(t, []string{
			"-architecture.storage=v1",
			"-write-path=ingester",
			"-compactor.block-ranges=30m",
		})

		require.Error(t, cfg.Validate())
	})
}
