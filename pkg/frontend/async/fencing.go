package async

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/backoff"
	"github.com/thanos-io/objstore"

	phlareobj "github.com/grafana/pyroscope/v2/pkg/objstore"
)

// leaseHandle is an owner's cached copy of metadata.json plus the object
// version its last successful write produced. Owner writes are blind,
// conditioned on that version alone; they never re-read the object.
type leaseHandle struct {
	meta    Metadata
	version *objstore.ObjectVersion
}

// Seeding a lease's version is a one-time cost per create or claim, so a
// short bounded retry is cheap insurance against a single bad response.
var currentVersionBackoffConfig = backoff.Config{
	MinBackoff: 25 * time.Millisecond,
	MaxBackoff: 100 * time.Millisecond,
	MaxRetries: 3,
}

// currentVersionOrNil returns path's version for seeding a new leaseHandle.
// nil is safe only when Attributes itself reports no version (a backend
// limitation, or degrade mode) -- the unconditional fallback
// saveJSONConditional needs anyway. A persistent read failure is an error
// instead: seeding nil there would silently disable fencing for the lease's
// whole lifetime.
func (s *Store) currentVersionOrNil(ctx context.Context, path string) (*objstore.ObjectVersion, error) {
	if !s.conditionalWritesSupported {
		return nil, nil
	}
	var attrs objstore.ObjectAttributes
	var err error
	b := backoff.New(ctx, currentVersionBackoffConfig)
	for b.Ongoing() {
		attrs, err = s.bucket.Attributes(ctx, path)
		if err == nil {
			return attrs.Version, nil
		}
		b.Wait()
	}
	if err == nil {
		// Ongoing() can be false before the first attempt (ctx canceled);
		// don't wrap a nil error.
		err = ctx.Err()
	}
	return nil, fmt.Errorf("failed to read metadata version: %w", err)
}

// readMetadataVersioned returns metadata.json's content with the version it
// corresponds to. A write landing between the two reads is caught by the
// caller's own If-Match condition failing, not re-checked here.
func (s *Store) readMetadataVersioned(ctx context.Context, path string, meta *Metadata) (*objstore.ObjectVersion, error) {
	attrs, err := s.bucket.Attributes(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := s.readJSON(ctx, path, meta); err != nil {
		return nil, err
	}
	return attrs.Version, nil
}

// saveJSONConditional uploads meta with an If-Match precondition on version.
// A nil version (no-version backend, or degrade mode) writes unconditionally
// rather than becoming permanently unwritable.
func (s *Store) saveJSONConditional(ctx context.Context, path string, meta *Metadata, version *objstore.ObjectVersion) error {
	if version == nil {
		if s.conditionalWritesSupported {
			level.Warn(s.logger).Log("msg", "metadata object reports no version; writing unconditionally", "request_id", meta.RequestID)
		}
		return s.saveJSON(ctx, path, meta)
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return s.bucket.Upload(ctx, path, bytes.NewReader(data), objstore.WithIfMatch(version))
}

// updateMetadata runs a read-decide-write cycle, conditioning the write on
// the version just read. One losing race gets one fresh retry; after that
// the condition-not-met error is returned so callers choose whether it is a
// silent give-up or reportable. decide returns write=false for "nothing to
// do".
func (s *Store) updateMetadata(ctx context.Context, path string, decide func(*Metadata) (bool, error)) error {
	var lastErr error
	for range 2 {
		var meta Metadata
		version, err := s.readMetadataVersioned(ctx, path, &meta)
		if err != nil {
			return err
		}
		write, err := decide(&meta)
		if err != nil || !write {
			return err
		}
		if err := s.saveJSONConditional(ctx, path, &meta, version); err != nil {
			if s.bucket.IsConditionNotMetErr(err) {
				lastErr = err
				continue
			}
			return err
		}
		return nil
	}
	return lastErr
}

// wrapExpectedErrs keeps lost CAS races out of bucket failure metrics:
// losing to a concurrent writer is routine contention, not a bucket problem.
// Both instrumented-bucket interfaces are checked because pkg/objstore wraps
// thanos buckets with its own.
func wrapExpectedErrs(bucket objstore.Bucket) objstore.Bucket {
	if ib, ok := bucket.(phlareobj.InstrumentedBucket); ok {
		return ib.WithExpectedErrs(bucket.IsConditionNotMetErr)
	}
	if ib, ok := bucket.(objstore.InstrumentedBucket); ok {
		return ib.WithExpectedErrs(bucket.IsConditionNotMetErr)
	}
	return bucket
}
