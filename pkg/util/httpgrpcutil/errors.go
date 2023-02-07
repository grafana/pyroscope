// SPDX-License-Identifier: AGPL-3.0-only

package httpgrpcutil

import (
	"net/http"

	"github.com/grafana/phlare/pkg/util/httpgrpc"
)

// PrioritizeRecoverableErr checks whether in the given slice of errors there is a recoverable error, if yes then it will
// return the first recoverable error, if not then it will return the first non-recoverable error, if there is no
// error at all then it will return nil.
func PrioritizeRecoverableErr(errs ...error) error {
	var firstErr error

	for _, err := range errs {
		if err == nil {
			continue
		}

		resp, ok := httpgrpc.HTTPResponseFromError(err)
		if !ok {
			// Not a gRPC HTTP error, assume it is recoverable to fail gracefully.
			return err
		}
		if resp.Code/100 == 5 || resp.Code == http.StatusTooManyRequests {
			// Found a recoverable error, return it.
			return err
		} else if firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}
