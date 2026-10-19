package store

import (
	"errors"
	"fmt"

	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/metastore/store"
)

var jobPlanBucketName = []byte("compaction_job_plan")

var ErrInvalidJobPlan = errors.New("invalid job plan entry")

type JobPlanStore struct{ bucketName []byte }

func NewJobPlanStore() *JobPlanStore {
	return &JobPlanStore{bucketName: jobPlanBucketName}
}

func (s JobPlanStore) CreateBuckets(tx *bbolt.Tx) error {
	_, err := tx.CreateBucketIfNotExists(s.bucketName)
	return err
}

func (s JobPlanStore) StoreJobPlan(tx *bbolt.Tx, plan *raft_log.CompactionJobPlan) error {
	v, _ := plan.MarshalVT()
	return tx.Bucket(s.bucketName).Put([]byte(plan.Name), v)
}

func (s JobPlanStore) GetJobPlan(tx *bbolt.Tx, name string) (*raft_log.CompactionJobPlan, error) {
	b := tx.Bucket(s.bucketName).Get([]byte(name))
	if b == nil {
		return nil, fmt.Errorf("loading job plan %s: %w", name, store.ErrNotFound)
	}
	var v raft_log.CompactionJobPlan
	if err := v.UnmarshalVT(b); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJobPlan, err)
	}
	return &v, nil
}

func (s JobPlanStore) DeleteJobPlan(tx *bbolt.Tx, name string) error {
	return tx.Bucket(s.bucketName).Delete([]byte(name))
}

// Field numbers of raft_log.CompactionJobPlan, as read by
// unmarshalJobPlanSummary. TestJobPlanSummary_fields pins them against the
// generated message, so a change to the schema breaks the test rather than
// the parser.
const (
	jobPlanFieldTenant          = 2
	jobPlanFieldShard           = 3
	jobPlanFieldCompactionLevel = 4
	jobPlanFieldSourceBlocks    = 5
)

// JobPlanSummary describes a compaction job without the parts of its plan
// that only the assigned worker needs.
type JobPlanSummary struct {
	Tenant          string
	Shard           uint32
	CompactionLevel uint32
	// SourceBlocks is the number of source blocks, always reported.
	SourceBlocks uint32
	// SourceBlockIDs is only populated when asked for.
	SourceBlockIDs []string
}

// GetJobPlanSummary reads the parts of a job plan that describe the job.
//
// A plan also carries the tombstone list and the source block identifiers,
// which together are the bulk of it and which listing the schedule does not
// need: the tombstones are never reported, and the identifiers only when
// they were asked for. Decoding the whole message and discarding them made
// listing the schedule several times more expensive than it had to be, so
// this reads the fields it wants and steps over the rest.
func (s JobPlanStore) GetJobPlanSummary(tx *bbolt.Tx, name string, withSourceBlocks bool) (*JobPlanSummary, error) {
	b := tx.Bucket(s.bucketName).Get([]byte(name))
	if b == nil {
		return nil, fmt.Errorf("loading job plan %s: %w", name, store.ErrNotFound)
	}
	var v JobPlanSummary
	if err := unmarshalJobPlanSummary(&v, b, withSourceBlocks); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJobPlan, err)
	}
	return &v, nil
}

func unmarshalJobPlanSummary(dst *JobPlanSummary, b []byte, withSourceBlocks bool) error {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return protowire.ParseError(n)
		}
		b = b[n:]
		switch {
		case num == jobPlanFieldTenant && typ == protowire.BytesType:
			v, m := protowire.ConsumeBytes(b)
			if m < 0 {
				return protowire.ParseError(m)
			}
			dst.Tenant = string(v)
			b = b[m:]

		case num == jobPlanFieldShard && typ == protowire.VarintType:
			v, m := protowire.ConsumeVarint(b)
			if m < 0 {
				return protowire.ParseError(m)
			}
			dst.Shard = uint32(v)
			b = b[m:]

		case num == jobPlanFieldCompactionLevel && typ == protowire.VarintType:
			v, m := protowire.ConsumeVarint(b)
			if m < 0 {
				return protowire.ParseError(m)
			}
			dst.CompactionLevel = uint32(v)
			b = b[m:]

		case num == jobPlanFieldSourceBlocks && typ == protowire.BytesType:
			v, m := protowire.ConsumeBytes(b)
			if m < 0 {
				return protowire.ParseError(m)
			}
			dst.SourceBlocks++
			if withSourceBlocks {
				dst.SourceBlockIDs = append(dst.SourceBlockIDs, string(v))
			}
			b = b[m:]

		default:
			m := protowire.ConsumeFieldValue(num, typ, b)
			if m < 0 {
				return protowire.ParseError(m)
			}
			b = b[m:]
		}
	}
	return nil
}
