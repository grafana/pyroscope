package main

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/phlare/pkg/test"
)

func TestFlagParsing(t *testing.T) {
	for name, tc := range map[string]struct {
		arguments      []string
		stdoutMessage  string // string that must be included in stdout
		stderrMessage  string // string that must be included in stderr
		stdoutExcluded string // string that must NOT be included in stdout
		stderrExcluded string // string that must NOT be included in stderr
	}{
		"help-short": {
			arguments:      []string{"-h"},
			stdoutMessage:  "Usage of", // Usage must be on stdout, not stderr.
			stderrExcluded: "Usage of",
		},
		"help": {
			arguments:      []string{"-help"},
			stdoutMessage:  "Usage of", // Usage must be on stdout, not stderr.
			stderrExcluded: "Usage of",
		},
		"help-all": {
			arguments:      []string{"-help-all"},
			stdoutMessage:  "Usage of", // Usage must be on stdout, not stderr.
			stderrExcluded: "Usage of",
		},
		"user visible module listing": {
			arguments:      []string{"-modules"},
			stdoutMessage:  "ingester *\n",
			stderrExcluded: "ingester\n",
		},
		"version": {
			arguments:      []string{"-version"},
			stdoutMessage:  "phlare, version",
			stderrExcluded: "phlare, version",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_ = os.Setenv("TARGET", "ingester")
			oldDefaultRegistry := prometheus.DefaultRegisterer
			defer func() {
				prometheus.DefaultRegisterer = oldDefaultRegistry
			}()
			// We need to reset the default registry to avoid
			// "duplicate metrics collector registration attempted" errors.
			prometheus.DefaultRegisterer = prometheus.NewRegistry()
			testSingle(t, tc.arguments, tc.stdoutMessage, tc.stderrMessage, tc.stdoutExcluded, tc.stderrExcluded)
		})
	}
}

func testSingle(t *testing.T, arguments []string, stdoutMessage, stderrMessage, stdoutExcluded, stderrExcluded string) {
	t.Helper()
	oldArgs, oldStdout, oldStderr := os.Args, os.Stdout, os.Stderr
	restored := false
	restoreIfNeeded := func() {
		if restored {
			return
		}
		os.Stdout = oldStdout
		os.Stderr = oldStderr
		os.Args = oldArgs
		restored = true
	}
	defer restoreIfNeeded()

	arguments = append([]string{"./phlare"}, arguments...)

	os.Args = arguments
	co := test.CaptureOutput(t)

	// reset default flags
	flag.CommandLine = flag.NewFlagSet(arguments[0], flag.ExitOnError)

	main()

	stdout, stderr := co.Done()

	// Restore stdout and stderr before reporting errors to make them visible.
	restoreIfNeeded()
	if !strings.Contains(stdout, stdoutMessage) {
		t.Errorf("Expected on stdout: %q, stdout: %s\n", stdoutMessage, stdout)
	}
	if !strings.Contains(stderr, stderrMessage) {
		t.Errorf("Expected on stderr: %q, stderr: %s\n", stderrMessage, stderr)
	}
	if len(stdoutExcluded) > 0 && strings.Contains(stdout, stdoutExcluded) {
		t.Errorf("Unexpected output on stdout: %q, stdout: %s\n", stdoutExcluded, stdout)
	}
	if len(stderrExcluded) > 0 && strings.Contains(stderr, stderrExcluded) {
		t.Errorf("Unexpected output on stderr: %q, stderr: %s\n", stderrExcluded, stderr)
	}
}
