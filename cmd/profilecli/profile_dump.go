package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/v2/pkg/profiledump"
)

var (
	errDumpMissing = errors.New("capture object not found (upload may be pending, may have failed, or retention may have removed it)")
	errDumpInvalid = errors.New("invalid or unsupported capture")
	errDumpLimit   = errors.New("work limit reached")
)

const dumpExtract = "extract"
const dumpEncodingUnknown = "unknown"

const dumpValidation = "header and metadata validated with matching object size, payload not read or verified, and no envelope checksum"
const dumpWarning = "WARNING: extracted files contain raw customer data, including metadata. Pyroscope cannot technically control subsequent local handling.\n"

type profileDumpParams struct {
	*bucketParams
	operation, key, tenant, from, to, path string
	limit, maxWork                         int
	maxSize                                int64
	timeout                                time.Duration
}

func addProfileDumpCommands(app *kingpin.Application) map[string]*profileDumpParams {
	group := app.Command("profile-dump", "Retrieve raw captures from the customer bucket. Tenant filters are not authorization boundaries.")
	commands := make(map[string]*profileDumpParams)
	for _, operation := range []string{"list", "inspect", dumpExtract} {
		cmd := group.Command(operation, map[string]string{
			"list":      "List captures within a tenant and capture-time window (bounded JSON output).",
			"inspect":   "Print envelope metadata as JSON without reading the payload. Metadata may contain customer identifiers.",
			dumpExtract: "Extract unchanged native bytes and a .metadata.json sidecar. Never overwrite existing files.",
		}[operation])
		p := &profileDumpParams{bucketParams: &bucketParams{}, operation: operation}
		// Reuse storage flags for the same providers and credentials.
		fs := flag.NewFlagSet("storage", flag.ContinueOnError)
		p.objectStoreCfg.RegisterFlagsWithPrefix("storage.", fs)
		fs.VisitAll(func(f *flag.Flag) { cmd.Flag(f.Name, f.Usage).SetValue(f.Value) })
		cmd.Flag("timeout", "Command context deadline. Cancellation, including Ctrl-C, depends on provider support (Swift may wait for provider timeouts).").Default("2m").DurationVar(&p.timeout)
		cmd.Flag("max-object-size", "Maximum complete envelope size in bytes.").Default("134217728").Int64Var(&p.maxSize)
		if operation == "list" {
			cmd.Flag("tenant-id", "Tenant to select. Customer-cloud permissions remain authoritative.").Required().StringVar(&p.tenant)
			cmd.Flag("from", "Inclusive capture time, RFC3339 (not client profile time).").Required().StringVar(&p.from)
			cmd.Flag("to", "Exclusive capture time, RFC3339.").Required().StringVar(&p.to)
			cmd.Flag("limit", "Maximum results. Reaching it marks results limited.").Default("100").IntVar(&p.limit)
			cmd.Flag("max-work", "Maximum sum of listing calls, entries visited, and metadata storage calls (3 per candidate).").Default("10000").IntVar(&p.maxWork)
		} else {
			cmd.Arg("key", "Exact profile-debug-dumps/... key, relative to storage.prefix (as in capture traces).").Required().StringVar(&p.key)
			if operation == dumpExtract {
				cmd.Flag("output", "Native output path. Default is capture ID plus stored-representation extension. Metadata goes to PATH.metadata.json.").Short('o').StringVar(&p.path)
			}
		}
		commands[cmd.FullCommand()] = p
	}
	return commands
}

func profileDump(ctx context.Context, p *profileDumpParams) (err error) {
	if p.timeout <= 0 || p.maxSize < int64(profiledump.HeaderSize) {
		return errors.New("timeout and max-object-size must be positive (size at least header size)")
	}
	if err := p.objectStoreCfg.Validate(logger); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	b, err := p.initClient(ctx)
	if err != nil {
		return fmt.Errorf("open capture storage: %w", err)
	}
	defer func() { err = errors.Join(err, b.Close()) }()
	return runProfileDump(ctx, b, p, output(ctx), consoleOutput)
}

func runProfileDump(ctx context.Context, b objstore.BucketReader, p *profileDumpParams, out, status io.Writer) error {
	switch p.operation {
	case "list":
		return listProfileDumps(ctx, b, p, out)
	case "inspect":
		info, err := inspectProfileDump(ctx, b, p.key, p.maxSize)
		if err != nil {
			return err
		}
		return writeDumpJSON(out, struct {
			Key        string               `json:"key"`
			Validation string               `json:"validation"`
			Metadata   profiledump.Metadata `json:"metadata"`
		}{p.key, dumpValidation, info.Metadata})
	case dumpExtract:
		return extractProfileDump(ctx, b, p, out, status)
	default:
		return errors.New("unknown profile-dump command")
	}
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

func matchDumpMetadata(k profiledump.ObjectKey, m profiledump.Metadata) error {
	if k.TenantID != m.TenantID || k.CaptureID.String() != m.CaptureID || k.Format != m.NativeFormat {
		return fmt.Errorf("%w: key and metadata disagree", errDumpInvalid)
	}
	return nil
}

// Inspect requests exactly header and metadata, using the codec's Read sizes.
// Range readers are closed after each request, even on short reads/cancellation.
type dumpRangeReader struct {
	ctx        context.Context
	bucket     objstore.BucketReader
	key        string
	offset     int64
	storageErr error
}

func (r *dumpRangeReader) Read(p []byte) (n int, err error) {
	if err := r.ctx.Err(); err != nil {
		r.storageErr = err
		return 0, err
	}
	rd, err := r.bucket.GetRange(r.ctx, r.key, r.offset, int64(len(p)))
	if err != nil {
		r.storageErr = err
		return 0, err
	}
	n, err = io.ReadFull(rd, p)
	closeErr := rd.Close()
	r.offset += int64(n)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		r.storageErr = err
	}
	if closeErr != nil {
		r.storageErr = closeErr
	}
	return n, errors.Join(err, closeErr)
}

func inspectProfileDump(ctx context.Context, b objstore.BucketReader, key string, maxSize int64) (profiledump.Inspection, error) {
	k, err := validateDumpKey(key)
	if err != nil {
		return profiledump.Inspection{}, err
	}
	if err := ctx.Err(); err != nil {
		return profiledump.Inspection{}, err
	}
	attrs, err := b.Attributes(ctx, key)
	if err != nil {
		return profiledump.Inspection{}, dumpStorageError(b, err)
	}
	if attrs.Size < int64(profiledump.HeaderSize) || attrs.Size > maxSize {
		return profiledump.Inspection{}, fmt.Errorf("%w: object size %d outside bounds", errDumpInvalid, attrs.Size)
	}
	r := &dumpRangeReader{ctx: ctx, bucket: b, key: key}
	// The object has already passed maxSize. Use its actual size as the codec's
	// bound so impossible metadata lengths fail before a range request at or
	// beyond EOF, which providers such as S3 reject as InvalidRange.
	info, err := profiledump.Inspect(r, attrs.Size)
	if r.storageErr != nil {
		return info, dumpStorageError(b, r.storageErr)
	}
	if err != nil {
		return info, fmt.Errorf("%w: %w", errDumpInvalid, err)
	}
	if info.ObjectSize != attrs.Size {
		return info, fmt.Errorf("%w: declared size %d differs from object size %d", errDumpInvalid, info.ObjectSize, attrs.Size)
	}
	if err := ctx.Err(); err != nil {
		return info, err
	}
	return info, matchDumpMetadata(k, info.Metadata)
}

type dumpListEntry struct {
	Key         string                     `json:"key"`
	CapturedAt  time.Time                  `json:"captured_at"`
	Source      profiledump.SourceProtocol `json:"source"`
	Format      profiledump.Format         `json:"format"`
	PayloadSize int64                      `json:"payload_size"`
}

type dumpListResult struct {
	Captures       []dumpListEntry `json:"captures"`
	Limited        bool            `json:"limited"`
	Reason         string          `json:"reason,omitempty"`
	Work           int             `json:"work"`
	SkippedInvalid int             `json:"skipped_invalid"`
	SkippedMissing int             `json:"skipped_missing"`
	Validation     string          `json:"validation"`
}

func listProfileDumps(ctx context.Context, b objstore.BucketReader, p *profileDumpParams, out io.Writer) (err error) {
	segment, err := profiledump.EncodeTenant(p.tenant)
	if err != nil {
		return err
	}
	from, err := time.Parse(time.RFC3339Nano, p.from)
	if err != nil {
		return fmt.Errorf("from: %w", err)
	}
	to, err := time.Parse(time.RFC3339Nano, p.to)
	if err != nil {
		return fmt.Errorf("to: %w", err)
	}
	from, to = from.UTC(), to.UTC()
	if !from.Before(to) || from.Year() < 1970 || to.Year() > 9999 {
		return errors.New("require from < to within years 1970–9999")
	}
	if p.limit <= 0 || p.maxWork <= 0 {
		return errors.New("limit and max-work must be positive")
	}
	result := dumpListResult{Captures: []dumpListEntry{}, Validation: dumpValidation}
	defer func() {
		if err != nil {
			result.Limited = true
			result.Reason = err.Error()
		}
		if errors.Is(err, errDumpLimit) {
			err = nil
		}
		err = errors.Join(err, writeDumpJSON(out, result))
	}()
	spend := func(n int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if n > p.maxWork-result.Work {
			return errDumpLimit
		}
		result.Work += n
		return nil
	}
	// Direct day prefixes avoid listing unrelated tenants/months. Both calls for
	// empty partitions and every provider callback count toward the work bound.
	for day := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC); day.Before(to); day = day.AddDate(0, 0, 1) {
		if err := spend(1); err != nil {
			return err
		}
		prefix := profiledump.ObjectPrefix + segment + "/" + day.Format("2006/01/02") + "/"
		err := b.Iter(ctx, prefix, func(key string) error {
			if err := spend(1); err != nil {
				return err
			}
			k, err := validateDumpKey(key)
			if err != nil || !strings.HasPrefix(key, prefix) || k.TenantID != p.tenant {
				result.SkippedInvalid++
				return nil
			}
			// Keys are millisecond precision: include the boundary millisecond and
			// apply exact sub-millisecond filtering using captured_at below.
			if k.CaptureTime.Before(from.Truncate(time.Millisecond)) || !k.CaptureTime.Before(to) {
				return nil
			}
			if err := spend(3); err != nil {
				return err
			}
			info, err := inspectProfileDump(ctx, b, key, p.maxSize)
			if errors.Is(err, errDumpMissing) {
				result.SkippedMissing++
				return nil
			}
			if errors.Is(err, errDumpInvalid) {
				result.SkippedInvalid++
				return nil
			}
			if err != nil {
				return err
			}
			m := info.Metadata
			if m.CapturedAt.Before(from) || !m.CapturedAt.Before(to) {
				return nil
			}
			result.Captures = append(result.Captures, dumpListEntry{key, m.CapturedAt, m.SourceProtocol, m.NativeFormat, m.PayloadSize})
			if len(result.Captures) >= p.limit {
				result.Limited = true
				result.Reason = "result limit reached. More captures may exist"
				return errDumpResultLimit
			}
			return nil
		}) // Deliberately non-recursive: only files immediately beneath this day.
		if errors.Is(err, errDumpResultLimit) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	// A provider may ignore cancellation and return nil without invoking any
	// callbacks for the final empty partition. Report that cancellation too.
	return ctx.Err()
}

var errDumpResultLimit = errors.New("result limit reached")

// Track read failures separately from codec errors so permission/network failures
// are not mislabeled as corrupt envelopes. Also check cancellation with fake/local readers.
type dumpPayloadReader struct {
	ctx     context.Context
	r       io.Reader
	readErr error
}

func (r *dumpPayloadReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		r.readErr = err
		return 0, err
	}
	n, err := r.r.Read(p)
	if err != nil && err != io.EOF {
		r.readErr = err
	}
	return n, err
}

type dumpPayloadWriter struct {
	w   io.Writer
	err error
}

func (w *dumpPayloadWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}

func extractProfileDump(ctx context.Context, b objstore.BucketReader, p *profileDumpParams, out, status io.Writer) (err error) {
	k, err := validateDumpKey(p.key)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(status, dumpWarning); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	rd, err := b.Get(ctx, p.key)
	if err != nil {
		return dumpStorageError(b, err)
	}
	// Close before publishing files. a close failure must not report success.
	defer func() {
		if rd != nil {
			err = errors.Join(err, rd.Close())
		}
	}()
	dir := "."
	if p.path != "" {
		dir = filepath.Dir(p.path)
	}
	payload, err := os.CreateTemp(dir, ".profile-dump-*")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.Remove(payload.Name())) }()
	r := &dumpPayloadReader{ctx: ctx, r: rd}
	w := &dumpPayloadWriter{w: payload}
	m, decodeErr := profiledump.Decode(r, w, p.maxSize)
	closeErr := payload.Close()
	readerCloseErr := rd.Close()
	rd = nil
	if r.readErr != nil {
		return dumpStorageError(b, r.readErr)
	}
	if w.err != nil {
		return fmt.Errorf("write native output: %w", w.err)
	}
	if decodeErr != nil {
		return fmt.Errorf("%w: %w", errDumpInvalid, decodeErr)
	}
	if err := errors.Join(closeErr, readerCloseErr); err != nil {
		return err
	}
	if err := matchDumpMetadata(k, m); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	path := p.path
	if path == "" {
		path = m.CaptureID + dumpExtension(m)
	}
	metadata, err := os.CreateTemp(dir, ".profile-dump-metadata-*")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.Remove(metadata.Name())) }()
	encodeErr := writeDumpJSON(metadata, m)
	if err := errors.Join(encodeErr, metadata.Close()); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Hard links publish complete files without overwriting files or following
	// existing symlinks. Keep the metadata available before exposing the payload.
	metadataPath := path + ".metadata.json"
	if err := os.Link(metadata.Name(), metadataPath); err != nil {
		return fmt.Errorf("publish metadata (existing files are not overwritten): %w", err)
	}
	if err := os.Link(payload.Name(), path); err != nil {
		return errors.Join(fmt.Errorf("publish payload (existing files are not overwritten): %w", err), os.Remove(metadataPath))
	}
	err = writeDumpJSON(out, struct {
		Payload    string `json:"payload"`
		Metadata   string `json:"metadata"`
		Validation string `json:"validation"`
	}{path, metadataPath, "complete envelope framing and length validated. no checksum, authenticity, or native-format validation"})
	if err != nil {
		return errors.Join(err, os.Remove(path), os.Remove(metadataPath))
	}
	return nil
}

func dumpExtension(m profiledump.Metadata) string {
	switch m.PayloadEncoding {
	case "gzip":
		return ".pprof.gz"
	case dumpEncodingUnknown:
		return ".pprof.encoded"
	default:
		return ".pprof"
	}
}
