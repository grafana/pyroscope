package adhoc

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/sirupsen/logrus"
)

func newPull(cfg *config.Adhoc, args []string, storage *storage.Storage, logger *logrus.Logger) (runner, error) {
	return nil, fmt.Errorf("unsupported mode")
}
