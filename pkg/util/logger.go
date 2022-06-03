package util

import "github.com/go-kit/log"

// Logger is a global logger to use only where you cannot inject a logger.
var Logger = log.NewNopLogger()
