package symbolizer

import (
	"errors"
	"fmt"
)

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
