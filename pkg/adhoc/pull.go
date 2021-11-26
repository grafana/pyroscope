package adhoc

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

func newPull(cfg *config.Adhoc, args []string, storage *storage.Storage, logger *logrus.Logger) (runner, error) {
	return nil, fmt.Errorf("unsupported mode")
}
