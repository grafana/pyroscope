package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

var logger = log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout))

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, os.Args); err != nil {
		_ = level.Error(logger).Log("msg", "test command failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	command, err := commandName(args)
	if err != nil {
		return err
	}

	switch command {
	case "idle":
		return Idle(ctx)
	case "first_seed":
		return FirstSeed(ctx)
	case "parallel_driver_roundtrip":
		return fmt.Errorf("command %q is not implemented", command)
	case "parallel_driver_metadata":
		return fmt.Errorf("command %q is not implemented", command)
	case "anytime_tenant_isolation":
		return fmt.Errorf("command %q is not implemented", command)
	case "eventually_recover":
		return fmt.Errorf("command %q is not implemented", command)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

// commandName supports both ways the driver is expected to run:
//
//   - as the container process: antithesis idle
//   - through a Test Composer symlink: /opt/antithesis/test/v1/core/first_seed
func commandName(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("missing argv[0]")
	}

	base := filepath.Base(args[0])
	if isCommand(base) {
		return base, nil
	}

	if len(args) == 1 {
		if isTestComposerCommand(base) {
			return "", fmt.Errorf("unknown Test Composer command %q", base)
		}
		return "idle", nil
	}
	if len(args) == 2 && isCommand(args[1]) {
		return args[1], nil
	}
	return "", fmt.Errorf("unknown command %q", args[1])
}

func isCommand(command string) bool {
	switch command {
	case "idle",
		"first_seed",
		"parallel_driver_roundtrip",
		"parallel_driver_metadata",
		"anytime_tenant_isolation",
		"eventually_recover":
		return true
	default:
		return false
	}
}

func isTestComposerCommand(command string) bool {
	for _, prefix := range []string{
		"first_",
		"parallel_driver_",
		"serial_driver_",
		"singleton_driver_",
		"anytime_",
		"eventually_",
		"finally_",
	} {
		if strings.HasPrefix(command, prefix) {
			return true
		}
	}
	return false
}
