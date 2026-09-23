package profiledump

import (
	"context"
	"io"
)

// Payload borrows stable input until Capture returns. Size must count payload and
// all scratch bytes without cloning. Write must honor ctx, use only dst/scratch
// for byte storage, fill dst, and report bytes written. It must not retain buffers,
// mutate input, or start goroutines.
type Payload interface {
	Size() (payloadBytes, scratchBytes int64, err error)
	Write(ctx context.Context, dst, scratch []byte) (int, error)
}

// BytesPayload copies a borrowed native body without intermediate storage.
type BytesPayload []byte

func (p BytesPayload) Size() (int64, int64, error) { return int64(len(p)), 0, nil }
func (p BytesPayload) Write(ctx context.Context, dst, _ []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	n := copy(dst, p)
	if n != len(p) {
		return n, io.ErrShortWrite
	}
	return n, nil
}

// Candidate borrows metadata and payload until Capture returns. Connect and
// ingest evaluate selectors, while OTLP bypasses them. The recorder overwrites
// tenant, time, ID, distributor, policy, activation, and size fields.
type Candidate struct {
	Metadata Metadata
	// SelectorLabels looks up borrowed input without copying labels.
	SelectorLabels LabelLookup
	Payload        Payload
	// StartSpan runs after selection and rate admission. Its finish callback
	// receives the outcome before Capture returns. Neither callback is queued.
	StartSpan func(context.Context) (context.Context, func(Outcome))
}

func (c Candidate) SupportsSelectors() bool {
	return c.Metadata.SourceProtocol == SourceConnect || c.Metadata.SourceProtocol == SourceIngest
}

type DropReason string

const (
	DropDisabled        DropReason = "disabled"
	DropExpired         DropReason = "expired"
	DropSelector        DropReason = "selector"
	DropSampled         DropReason = "sampled"
	DropTenantRate      DropReason = "tenant_rate"
	DropProcessRate     DropReason = "process_rate"
	DropLimiterCapacity DropReason = "limiter_capacity"
	DropTooLarge        DropReason = "too_large"
	DropByteBudget      DropReason = "byte_budget"
	DropInvalid         DropReason = "invalid"
	DropSerialization   DropReason = "serialization"
	DropPanic           DropReason = "panic"
	DropQueueFull       DropReason = "queue_full"
	DropShutdown        DropReason = "shutdown"
)

// Outcome describes local admission, never persistence or an ingestion error.
// Size is the complete encoded object size, or zero if sizing was not reached.
type Outcome struct {
	Enqueued          bool
	Reason            DropReason
	CaptureID         string
	ObjectKey         string
	Format            Format
	SourceProtocol    SourceProtocol
	DistributorID     string
	PayloadSize       int64
	Size              int64
	PolicyFingerprint string
}
