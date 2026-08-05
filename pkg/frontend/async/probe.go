package async

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/grafana/dskit/backoff"
	"github.com/thanos-io/objstore"
)

// The backend accepted a stale If-Match
var ErrConditionsNotEnforced = errors.New("probe: conditional upload with a stale version unexpectedly succeeded")

// The backend rejected an If-Match against the object's own current version
var ErrConditionsAlwaysRejected = errors.New("probe: conditional upload with the object's current version was unexpectedly rejected as a condition mismatch")

// Only transient probe failures are retried briefly
var probeBackoffConfig = backoff.Config{
	MinBackoff: 250 * time.Millisecond,
	MaxBackoff: 2 * time.Second,
	MaxRetries: 3,
}

// ProbeConditionalWrites verifies with real writes that bucket genuinely
// enforces If-Match, rather than trusting SupportedObjectUploadOptions.
func ProbeConditionalWrites(ctx context.Context, bucket objstore.Bucket) error {
	b := backoff.New(ctx, probeBackoffConfig)
	var err error
	for b.Ongoing() {
		err = probeOnce(ctx, bucket)
		if err == nil || errors.Is(err, ErrConditionsNotEnforced) || errors.Is(err, ErrConditionsAlwaysRejected) {
			return err
		}
		b.Wait()
	}
	return err
}

func probeOnce(ctx context.Context, bucket objstore.Bucket) error {
	name := path.Join(storagePrefix, ".probe-"+uuid.New().String())
	defer func() {
		// TTL cleanup reclaims stray probe objects if this best-effort attempt fails.
		// Detached from ctx, which may already be past its deadline when a probe
		// attempt fails.
		delCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = bucket.Delete(delCtx, name)
	}()

	if err := bucket.Upload(ctx, name, bytes.NewReader([]byte("a"))); err != nil {
		return fmt.Errorf("probe: initial upload: %w", err)
	}

	version, err := probeVersion(ctx, bucket, name)
	if err != nil {
		return err
	}

	// Check matching case succeeds first
	if err := bucket.Upload(ctx, name, bytes.NewReader([]byte("b")), objstore.WithIfMatch(version)); err != nil {
		if bucket.IsConditionNotMetErr(err) {
			return ErrConditionsAlwaysRejected
		}
		return fmt.Errorf("probe: conditional upload with the current version: %w", err)
	}
	staleVersion := version

	// The accepted write bumped the version, so staleVersion is now
	// deterministically stale. Re-read to confirm a version is still
	// reported.
	if _, err := probeVersion(ctx, bucket, name); err != nil {
		return err
	}

	err = bucket.Upload(ctx, name, bytes.NewReader([]byte("c")), objstore.WithIfMatch(staleVersion))
	if err == nil {
		return ErrConditionsNotEnforced
	}
	if !bucket.IsConditionNotMetErr(err) {
		return fmt.Errorf("probe: conditional upload with a stale version failed, but not with a condition-not-met error: %w", err)
	}

	return nil
}

func probeVersion(ctx context.Context, bucket objstore.Bucket, name string) (*objstore.ObjectVersion, error) {
	attrs, err := bucket.Attributes(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("probe: reading attributes: %w", err)
	}
	if attrs.Version == nil || attrs.Version.Value == "" {
		return nil, fmt.Errorf("probe: object reported no version")
	}
	return attrs.Version, nil
}
