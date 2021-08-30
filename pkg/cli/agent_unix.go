// +build !windows

package cli

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func StartAgent(_ *config.Agent) error {
	return fmt.Errorf("agent mode is supported only on Windows")
}
