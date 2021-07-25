package cli

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func StartServer(_ *config.Server) error {
	return fmt.Errorf("server mode is not supported on Windows")
}
