package symbolizer

import (
	"errors"
	"fmt"
)

// errObjectNotFound marks a bucket cache probe that found no object — the
// expected outcome for a build ID that was never fetched or uploaded.
var errObjectNotFound = errors.New("object not found in bucket")

type invalidBuildIDError struct {
	buildID string
}

func (e invalidBuildIDError) Error() string {
	return fmt.Sprintf("invalid build ID: %s", e.buildID)
}

type buildIDNotFoundError struct {
	buildID string
}

func (e buildIDNotFoundError) Error() string {
	return fmt.Sprintf("build ID not found: %s", e.buildID)
}

type upstreamUnavailableError struct {
	buildID string
}

func (e upstreamUnavailableError) Error() string {
	return fmt.Sprintf("debuginfod upstream unavailable, not fetching build ID: %s", e.buildID)
}

type httpStatusError struct {
	statusCode int
	body       string
}

func (e httpStatusError) Error() string {
	if e.body != "" {
		return fmt.Sprintf("HTTP error %d: %s", e.statusCode, e.body)
	}
	return fmt.Sprintf("HTTP error %d", e.statusCode)
}

// Helper function to check if an error is of a specific type
func isInvalidBuildIDError(err error) bool {
	var invalidBuildIDError invalidBuildIDError
	ok := errors.As(err, &invalidBuildIDError)
	return ok
}

func isHTTPStatusError(err error) (int, bool) {
	var httpErr httpStatusError
	ok := errors.As(err, &httpErr)
	if ok {
		return httpErr.statusCode, true
	}
	return 0, false
}
