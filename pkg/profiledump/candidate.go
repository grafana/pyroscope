package profiledump

// Candidate borrows stable metadata, labels and payload until Capture returns.
// The recorder replaces tenant, time, ID, distributor and size.
// Payload is copied directly into the owned buffer after admission.
type Candidate struct {
	Metadata       NativeMetadata
	SelectorLabels LabelLookup
	Payload        []byte
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
	DropQueueFull       DropReason = "queue_full"
	DropShutdown        DropReason = "shutdown"
)

// Outcome reports enqueue or drop status, without waiting for upload.
// Size is the combined payload and serialized JSON size, or zero if sizing was not reached.
type Outcome struct {
	Enqueued       bool
	Reason         DropReason
	CaptureID      string
	ObjectKey      string
	Format         Format
	SourceProtocol SourceProtocol
	DistributorID  string
	PayloadSize    int64
	Size           int64
}
