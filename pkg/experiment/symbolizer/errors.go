package symbolizer

import "fmt"

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
	status     string
}

func (e httpStatusError) Error() string {
	return fmt.Sprintf("unexpected HTTP status: %d %s", e.statusCode, e.status)
}

// Helper function to check if an error is of a specific type
func isInvalidBuildIDError(err error) bool {
	_, ok := err.(invalidBuildIDError)
	return ok
}

func isHTTPStatusError(err error) (int, bool) {
	httpErr, ok := err.(httpStatusError)
	if ok {
		return httpErr.statusCode, true
	}
	return 0, false
}
