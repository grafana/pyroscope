package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/thanos-io/objstore"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/operations"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
)

var (
	errDumpMissing = errors.New("capture object not found (upload may be pending, may have failed, or retention may have removed it)")
	errDumpInvalid = errors.New("invalid or unsupported capture")
)

const dumpList = "list"
const dumpInspect = "inspect"
const dumpExtract = "extract"

type profileDumpParams struct {
	*bucketParams
	key, tenant, from, to, path string
	limit                       int
	force                       bool
}

func addProfileDumpListParams(cmd commander) *profileDumpParams {
	p := &profileDumpParams{bucketParams: addBucketParams(cmd)}
	cmd.Flag("tenant-id", "Tenant ID to list captures for.").Required().StringVar(&p.tenant)
	cmd.Flag("from", "Start time for captured profiles (inclusive). Accepts relative times (now-1h), RFC3339, or Unix timestamps, truncated to milliseconds.").Default("now-1h").StringVar(&p.from)
	cmd.Flag("to", "End time for captured profiles (exclusive). Uses the same formats and millisecond truncation as --from.").Default("now").StringVar(&p.to)
	cmd.Flag("limit", "Maximum number of captures to return.").Default("100").IntVar(&p.limit)
	return p
}

func addProfileDumpInspectParams(cmd commander) *profileDumpParams {
	p := &profileDumpParams{bucketParams: addBucketParams(cmd)}
	cmd.Arg("key", "Exact "+profiledump.ObjectPrefix+"... payload or JSON sidecar key, relative to storage.prefix.").Required().StringVar(&p.key)
	return p
}

func addProfileDumpExtractParams(cmd commander) *profileDumpParams {
	p := addProfileDumpInspectParams(cmd)
	cmd.Flag("output", "Output path for the profile (default: CAPTURE_ID.pprof) and its metadata.").Short('o').StringVar(&p.path)
	cmd.Flag("force", "Overwrite the output files if they already exist.").Short('f').Default("false").BoolVar(&p.force)
	return p
}

func profileDump(ctx context.Context, p *profileDumpParams, run func(context.Context, objstore.BucketReader, *profileDumpParams, io.Writer) error) (err error) {
	if err := p.objectStoreCfg.Validate(logger); err != nil {
		return err
	}
	b, err := p.initClient(ctx)
	if err != nil {
		return fmt.Errorf("open capture storage: %w", err)
	}
	defer func() { err = errors.Join(err, b.Close()) }()
	return run(ctx, b, p, output(ctx))
}

func printProfileDump(ctx context.Context, b objstore.BucketReader, p *profileDumpParams, out io.Writer) error {
	info, err := inspectProfileDump(ctx, b, p.key)
	if err != nil {
		return err
	}
	return writeDumpJSON(out, struct {
		Key      string               `json:"key"`
		Metadata profiledump.Metadata `json:"metadata"`
	}{p.key, info})
}

func writeDumpJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func dumpStorageError(b objstore.BucketReader, err error) error {
	if b.IsObjNotFoundErr(err) {
		return fmt.Errorf("%w: %w", errDumpMissing, err)
	}
	return fmt.Errorf("capture storage failure: %w", err)
}

func validateDumpKey(key string) (profiledump.ObjectKey, error) {
	k, err := profiledump.ParseObjectKey(key)
	if err != nil {
		return k, fmt.Errorf("%w: key: %w", errDumpInvalid, err)
	}
	return k, nil
}

func inspectProfileDump(ctx context.Context, b objstore.BucketReader, key string) (profiledump.Metadata, error) {
	k, err := validateDumpKey(key)
	if err != nil {
		return profiledump.Metadata{}, err
	}
	rd, err := b.Get(ctx, k.MetadataKey)
	if err != nil {
		return profiledump.Metadata{}, fmt.Errorf("metadata sidecar %s: %w", k.MetadataKey, dumpStorageError(b, err))
	}
	data, readErr := io.ReadAll(rd)
	closeErr := rd.Close()
	var m profiledump.Metadata
	if err := errors.Join(readErr, closeErr); err != nil {
		return m, dumpStorageError(b, err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("%w: decode metadata: %w", errDumpInvalid, err)
	}
	return m, nil
}

type dumpListEntry struct {
	Key               string               `json:"key"`
	CapturedAt        time.Time            `json:"captured_at"`
	Labels            []*typesv1.LabelPair `json:"labels,omitempty"`
	OriginalProfileID string               `json:"original_profile_id,omitempty"`
	MetadataStatus    string               `json:"metadata_status,omitempty"`
}

type dumpListResult struct {
	Captures []dumpListEntry `json:"captures"`
	Limited  bool            `json:"limited"`
}

func listProfileDumps(ctx context.Context, b objstore.BucketReader, p *profileDumpParams, out io.Writer) error {
	if err := profiledump.ValidateTenant(p.tenant); err != nil {
		return err
	}
	from, err := operations.ParseTime(p.from)
	if err != nil {
		return fmt.Errorf("failed to parse from: %w", err)
	}
	to, err := operations.ParseTime(p.to)
	if err != nil {
		return fmt.Errorf("failed to parse to: %w", err)
	}
	from, to = from.UTC(), to.UTC()
	if from.After(to) || from.Year() < 1970 || to.Year() > 9999 {
		return errors.New("require from <= to within years 1970–9999")
	}
	if p.limit <= 0 {
		return errors.New("limit must be positive")
	}
	from = from.Truncate(time.Millisecond)
	to = to.Truncate(time.Millisecond)
	result := dumpListResult{Captures: []dumpListEntry{}}
	if from.Equal(to) {
		return writeDumpJSON(out, result)
	}
	// Emit JSON only after a successful listing, including a --limit stop.
	// Generate one intersecting UTC minute at a time, without assuming provider order.
	for minute := from.Truncate(time.Minute); minute.Before(to); minute = minute.Add(time.Minute) {
		prefix := profiledump.ObjectPrefix + p.tenant + "/" + minute.Format("2006-01-02/15/04") + "/"
		// List this minute's immediate entries without descending into subdirectories.
		err := b.Iter(ctx, prefix, func(key string) error {
			k, err := validateDumpKey(key)
			if err != nil || !strings.HasPrefix(key, prefix) || k.TenantID != p.tenant {
				return nil
			}
			if key != k.PayloadKey {
				return nil
			}
			if k.CaptureTime.Before(from) || !k.CaptureTime.Before(to) {
				return nil
			}
			entry := dumpListEntry{Key: k.PayloadKey, CapturedAt: k.CaptureTime}
			m, err := inspectProfileDump(ctx, b, key)
			if errors.Is(err, errDumpMissing) {
				entry.MetadataStatus = "missing"
			} else if errors.Is(err, errDumpInvalid) {
				entry.MetadataStatus = "invalid"
			} else if err != nil {
				return err
			} else {
				entry.Labels, entry.OriginalProfileID = m.Labels, m.OriginalProfileID
			}
			result.Captures = append(result.Captures, entry)
			if len(result.Captures) >= p.limit {
				result.Limited = true
				return errDumpResultLimit
			}
			return nil
		})
		if errors.Is(err, errDumpResultLimit) {
			break
		}
		if err != nil {
			return err
		}
	}
	return writeDumpJSON(out, result)
}

var errDumpResultLimit = errors.New("result limit reached")

func extractProfileDump(ctx context.Context, b objstore.BucketReader, p *profileDumpParams, out io.Writer) error {
	k, err := validateDumpKey(p.key)
	if err != nil {
		return err
	}
	path := p.path
	if path == "" {
		path = k.CaptureID.String() + ".pprof"
	}
	metadataPath := path + ".metadata.json"
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if p.force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	} else {
		// Check both destinations before downloading either object.
		for _, name := range []string{path, metadataPath} {
			if _, err := os.Lstat(name); err == nil {
				return fmt.Errorf("output file %s already exists (use --force to overwrite)", name)
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("failed to stat output file: %w", err)
			}
		}
	}
	for _, object := range []struct{ name, key, path string }{
		{"payload", k.PayloadKey, path},
		{"metadata", k.MetadataKey, metadataPath},
	} {
		rd, err := b.Get(ctx, object.key)
		if err != nil {
			return dumpStorageError(b, err)
		}
		file, err := os.OpenFile(object.path, flags, 0600)
		if errors.Is(err, os.ErrExist) {
			return errors.Join(fmt.Errorf("output file %s already exists (use --force to overwrite): %w", object.path, err), rd.Close())
		}
		if err != nil {
			return errors.Join(fmt.Errorf("open %s output: %w", object.name, err), rd.Close())
		}
		_, copyErr := io.Copy(file, rd)
		if err := errors.Join(copyErr, file.Close(), rd.Close()); err != nil {
			return fmt.Errorf("write %s output: %w", object.name, err)
		}
	}
	return writeDumpJSON(out, struct {
		Payload  string `json:"payload"`
		Metadata string `json:"metadata"`
	}{path, metadataPath})
}
