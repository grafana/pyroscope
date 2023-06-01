package clientpool

import ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"

type BidiClientMergeProfilesStacktraces interface {
	Send(*ingestv1.MergeProfilesStacktracesRequest) error
	Receive() (*ingestv1.MergeProfilesStacktracesResponse, error)
	CloseRequest() error
	CloseResponse() error
}

type BidiClientMergeProfilesLabels interface {
	Send(*ingestv1.MergeProfilesLabelsRequest) error
	Receive() (*ingestv1.MergeProfilesLabelsResponse, error)
	CloseRequest() error
	CloseResponse() error
}

type BidiClientMergeProfilesPprof interface {
	Send(*ingestv1.MergeProfilesPprofRequest) error
	Receive() (*ingestv1.MergeProfilesPprofResponse, error)
	CloseRequest() error
	CloseResponse() error
}
