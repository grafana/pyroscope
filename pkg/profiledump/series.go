package profiledump

import "context"

// SeriesCapture reuses selection for one series under an unchanged policy.
// It borrows stable labels for its entire lifetime and must not be shared across
// series or used concurrently. The recorder never retains it after Capture.
type SeriesCapture struct {
	recorder    *Recorder
	tenant      string
	labels      LabelLookup
	fingerprint string
	matched     bool
}

// PrepareSeries binds labels and tenant without evaluating or admitting a capture.
// Discard the returned state before changing or releasing the borrowed labels.
func (r *Recorder) PrepareSeries(tenant string, labels LabelLookup) SeriesCapture {
	return SeriesCapture{recorder: r, tenant: tenant, labels: labels}
}

// Capture uses the series labels instead of Candidate.SelectorLabels. Each call
// checks the current policy and deadline, then samples and rate-limits independently.
func (s *SeriesCapture) Capture(ctx context.Context, c Candidate) Outcome {
	return s.recorder.capture(ctx, s.tenant, c, s)
}
