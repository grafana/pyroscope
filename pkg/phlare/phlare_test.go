package phlare

import (
	"bytes"
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
	require.Equal(t, c.Server.HTTPListenPort, 4100)
	require.Contains(t, gotFlags[flagToCheck], "(default 4100)")
}
