package phlare

import (
	"bytes"
	"context"
	"flag"
	"io"
	"strings"
	"testing"

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
	// Key is the flag and value is the default text.
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
