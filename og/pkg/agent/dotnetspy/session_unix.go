//go:build dotnetspy && !windows
// +build dotnetspy,!windows

package dotnetspy

import "github.com/pyroscope-io/dotnetdiag/nettrace/profiler"

var profilerOptions = []profiler.Option{profiler.WithManagedCodeOnly()}
