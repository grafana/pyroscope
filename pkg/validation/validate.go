package validation

import (
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/validation"
)

type Reason string

const (
	ReasonLabel string = "reason"
	Unknown     Reason = "unknown"
	// InvalidLabels is a reason for discarding profiles which have labels that are invalid.
	InvalidLabels Reason = "invalid_labels"
	// MissingLabels is a reason for discarding profiles which have no labels.
	MissingLabels Reason = "missing_labels"
	// RateLimited is one of the values for the reason to discard samples.
	RateLimited Reason = "rate_limited"

	// NotInIngestionWindow is a reason for discarding profiles when Pyroscope doesn't accept profiles
	// that are outside of the ingestion window.
	NotInIngestionWindow Reason = "not_in_ingestion_window"

	// MaxLabelNamesPerSeries is a reason for discarding a request which has too many label names
	MaxLabelNamesPerSeries Reason = "max_label_names_per_series"
	// LabelNameTooLong is a reason for discarding a request which has a label name too long
	LabelNameTooLong Reason = "label_name_too_long"
	// LabelValueTooLong is a reason for discarding a request which has a label value too long
	LabelValueTooLong Reason = "label_value_too_long"
	// DuplicateLabelNames is a reason for discarding a request which has duplicate label names
	DuplicateLabelNames Reason = "duplicate_label_names"
	// SeriesLimit is a reason for discarding lines when we can't create a new stream
	// because the limit of active streams has been reached.
	SeriesLimit           Reason = "series_limit"
	QueryLimit            Reason = "query_limit"
	SamplesLimit          Reason = "samples_limit"
	ProfileSizeLimit      Reason = "profile_size_limit"
	SampleLabelsLimit     Reason = "sample_labels_limit"
	MalformedProfile      Reason = "malformed_profile"
	FlameGraphLimit       Reason = "flamegraph_limit"
	QueryMissingTimeRange Reason = "missing_time_range"
	QueryInvalidTimeRange Reason = "invalid_time_range"

	IngestLimitReached     Reason = "ingest_limit_reached"
	SkippedBySamplingRules Reason = "dropped_by_sampling_rules"

	BodySizeLimit Reason = "body_size_limit_exceeded"

	// Those profiles were dropped because of relabeling rules
	DroppedByRelabelRules Reason = "dropped_by_relabel_rules"

	SeriesLimitErrorMsg                          = "Maximum active series limit exceeded (%d/%d), reduce the number of active streams (reduce labels or reduce label values), or contact your administrator to see if the limit can be increased"
	MissingLabelsErrorMsg                        = "error at least one label pair is required per profile"
	InvalidLabelsErrorMsg                        = "invalid labels '%s' with error: %s"
	MaxLabelNamesPerSeriesErrorMsg               = "profile series '%s' has %d label names; limit %d"
	LabelNameTooLongErrorMsg                     = "profile with labels '%s' has label name too long: '%s'"
	LabelValueTooLongErrorMsg                    = "profile with labels '%s' has label value too long: '%s'"
	DuplicateLabelNamesErrorMsg                  = "profile with labels '%s' has duplicate label name: '%s'"
	DuplicateLabelNamesAfterSanitizationErrorMsg = "profile with labels '%s' has duplicate label name '%s' after label name sanitization from '%s'"
	QueryTooLongErrorMsg                         = "the query time range exceeds the limit (max_query_length, actual: %s, limit: %s)"
	ProfileTooBigErrorMsg                        = "the profile with labels '%s' exceeds the size limit (max_profile_size_byte, actual: %d, limit: %d)"
	ProfileTooManySamplesErrorMsg                = "the profile with labels '%s' exceeds the samples count limit (max_profile_stacktrace_samples, actual: %d, limit: %d)"
	ProfileTooManySampleLabelsErrorMsg           = "the profile with labels '%s' exceeds the sample labels limit (max_profile_stacktrace_sample_labels, actual: %d, limit: %d)"
	NotInIngestionWindowErrorMsg                 = "profile with labels '%s' is outside of ingestion window (profile timestamp: %s, %s)"
	MaxFlameGraphNodesErrorMsg                   = "max flamegraph nodes limit %d is greater than allowed %d"
	MaxFlameGraphNodesUnlimitedErrorMsg          = "max flamegraph nodes limit must be set (max allowed %d)"
	QueryMissingTimeRangeErrorMsg                = "missing time range in the query"
	QueryStartAfterEndErrorMsg                   = "query start time is after end time"
)

var (
	// DiscardedBytes is a metric of the total discarded bytes, by reason.
	DiscardedBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "discarded_bytes_total",
			Help:      "The total number of bytes that were discarded.",
		},
		[]string{ReasonLabel, "tenant"},
	)

	// DiscardedProfiles is a metric of the number of discarded profiles, by reason.
	DiscardedProfiles = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "discarded_samples_total",
			Help:      "The total number of samples that were discarded.",
		},
		[]string{ReasonLabel, "tenant"},
	)
)

type LabelValidationLimits interface {
	MaxLabelNameLength(tenantID string) int
	MaxLabelValueLength(tenantID string) int
	MaxLabelNamesPerSeries(tenantID string) int
}

// ValidateLabels validates the labels of a profile.
func ValidateLabels(limits LabelValidationLimits, tenantID string, ls []*typesv1.LabelPair) error {
	if len(ls) == 0 {
		return NewErrorf(MissingLabels, MissingLabelsErrorMsg)
	}
	sort.Sort(phlaremodel.Labels(ls))
	numLabelNames := len(ls)
	maxLabels := limits.MaxLabelNamesPerSeries(tenantID)
	if numLabelNames > maxLabels {
		return NewErrorf(MaxLabelNamesPerSeries, MaxLabelNamesPerSeriesErrorMsg, phlaremodel.LabelPairsString(ls), numLabelNames, maxLabels)
	}
	metricNameValue := phlaremodel.Labels(ls).Get(model.MetricNameLabel)
	if !model.IsValidMetricName(model.LabelValue(metricNameValue)) {
		return NewErrorf(InvalidLabels, InvalidLabelsErrorMsg, phlaremodel.LabelPairsString(ls), "invalid metric name")
	}
	serviceNameValue := phlaremodel.Labels(ls).Get(phlaremodel.LabelNameServiceName)
	if !isValidServiceName(serviceNameValue) {
		return NewErrorf(MissingLabels, InvalidLabelsErrorMsg, phlaremodel.LabelPairsString(ls), "service name is not provided")
	}

	var (
		lastLabelName = ""
		idx           = 0
	)
	for idx < len(ls) {
		l := ls[idx]
		if len(l.Name) > limits.MaxLabelNameLength(tenantID) {
			return NewErrorf(LabelNameTooLong, LabelNameTooLongErrorMsg, phlaremodel.LabelPairsString(ls), l.Name)
		}
		if len(l.Value) > limits.MaxLabelValueLength(tenantID) {
			return NewErrorf(LabelValueTooLong, LabelValueTooLongErrorMsg, phlaremodel.LabelPairsString(ls), l.Value)
		}
		if origName, newName, ok := SanitizeLabelName(l.Name); ok && origName != newName {
			var err error
			ls, idx, err = handleSanitizedLabel(ls, idx, origName, newName)
			if err != nil {
				return err
			}
			lastLabelName = ""
			if idx > 0 && idx <= len(ls) {
				lastLabelName = ls[idx-1].Name
			}
			continue
		} else if !ok {
			return NewErrorf(InvalidLabels, InvalidLabelsErrorMsg, phlaremodel.LabelPairsString(ls), "invalid label name '"+origName+"'")
		}
		if !model.LabelValue(l.Value).IsValid() {
			return NewErrorf(InvalidLabels, InvalidLabelsErrorMsg, phlaremodel.LabelPairsString(ls), "invalid label value '"+l.Value+"'")
		}
		if cmp := strings.Compare(lastLabelName, l.Name); cmp == 0 {
			return NewErrorf(DuplicateLabelNames, DuplicateLabelNamesErrorMsg, phlaremodel.LabelPairsString(ls), l.Name)
		}
		lastLabelName = l.Name
		idx += 1
	}

	return nil
}

// handleSanitizedLabel handles the case where a label name is sanitized. It ensures that the label name is unique and fails if the value is distinct.
func handleSanitizedLabel(ls []*typesv1.LabelPair, origIdx int, origName, newName string) ([]*typesv1.LabelPair, int, error) {
	newLabel := &typesv1.LabelPair{Name: newName, Value: ls[origIdx].Value}

	// Create new slice without the original element
	newSlice := make([]*typesv1.LabelPair, 0, len(ls))
	newSlice = append(newSlice, ls[:origIdx]...)
	newSlice = append(newSlice, ls[origIdx+1:]...)

	insertIdx, found := slices.BinarySearchFunc(newSlice, newLabel,
		func(a, b *typesv1.LabelPair) int {
			return strings.Compare(a.Name, b.Name)
		})

	if found {
		if newSlice[insertIdx].Value == newLabel.Value {
			// Same name and value we are done and can just return newSlice
			return newSlice, origIdx, nil
		} else {
			// Same name, different value - error
			return nil, 0, NewErrorf(DuplicateLabelNames,
				DuplicateLabelNamesAfterSanitizationErrorMsg,
				phlaremodel.LabelPairsString(ls), newName, origName)
		}
	}

	// Insert the new label at correct position
	newSlice = slices.Insert(newSlice, insertIdx, newLabel)

	finalIdx := insertIdx
	if insertIdx >= origIdx {
		finalIdx = origIdx
	}

	copy(ls, newSlice)
	return ls[:len(newSlice)], finalIdx, nil
}

// SanitizeLabelName reports whether the label name is valid,
// and returns the sanitized value.
//
// The only change the function makes is replacing dots with underscores.
func SanitizeLabelName(ln string) (old, sanitized string, ok bool) {
	if len(ln) == 0 {
		return ln, ln, false
	}
	hasDots := false
	for i, b := range ln {
		if (b < 'a' || b > 'z') && (b < 'A' || b > 'Z') && b != '_' && (b < '0' || b > '9' || i == 0) {
			if b == '.' {
				hasDots = true
			} else {
				return ln, ln, false
			}
		}
	}
	if !hasDots {
		return ln, ln, true
	}
	r := []rune(ln)
	for i, b := range r {
		if b == '.' {
			r[i] = '_'
		}
	}
	return ln, string(r), true
}

type ProfileValidationLimits interface {
	MaxProfileSizeBytes(tenantID string) int
	MaxProfileStacktraceSamples(tenantID string) int
	MaxProfileStacktraceSampleLabels(tenantID string) int
	MaxProfileStacktraceDepth(tenantID string) int
	MaxProfileSymbolValueLength(tenantID string) int
	RejectNewerThan(tenantID string) time.Duration
	RejectOlderThan(tenantID string) time.Duration
}

type ingestionWindow struct {
	from, to model.Time
}

func newIngestionWindow(limits ProfileValidationLimits, tenantID string, now model.Time) *ingestionWindow {
	var iw ingestionWindow
	if d := limits.RejectNewerThan(tenantID); d != 0 {
		iw.to = now.Add(d)
	}
	if d := limits.RejectOlderThan(tenantID); d != 0 {
		iw.from = now.Add(-d)
	}
	return &iw
}

func (iw *ingestionWindow) errorDetail() string {
	if iw.to == 0 {
		return fmt.Sprintf("the ingestion window starts at %s", util.FormatTimeMillis(int64(iw.from)))
	}
	if iw.from == 0 {
		return fmt.Sprintf("the ingestion window ends at %s", util.FormatTimeMillis(int64(iw.to)))
	}
	return fmt.Sprintf("the ingestion window starts at %s and ends at %s", util.FormatTimeMillis(int64(iw.from)), util.FormatTimeMillis(int64(iw.to)))

}

func (iw *ingestionWindow) valid(t model.Time, ls phlaremodel.Labels) error {
	if (iw.from == 0 || t.After(iw.from)) && (iw.to == 0 || t.Before(iw.to)) {
		return nil
	}

	return NewErrorf(NotInIngestionWindow, NotInIngestionWindowErrorMsg, phlaremodel.LabelPairsString(ls), util.FormatTimeMillis(int64(t)), iw.errorDetail())
}

func ValidateProfile(limits ProfileValidationLimits, tenantID string, prof *googlev1.Profile, uncompressedSize int, ls phlaremodel.Labels, now model.Time) error {
	if prof == nil {
		return nil
	}

	if prof.TimeNanos > 0 {
		// check profile timestamp within ingestion window
		if err := newIngestionWindow(limits, tenantID, now).valid(model.TimeFromUnixNano(prof.TimeNanos), ls); err != nil {
			return err
		}
	} else {
		prof.TimeNanos = now.UnixNano()
	}

	if limit := limits.MaxProfileSizeBytes(tenantID); limit != 0 && uncompressedSize > limit {
		return NewErrorf(ProfileSizeLimit, ProfileTooBigErrorMsg, phlaremodel.LabelPairsString(ls), uncompressedSize, limit)
	}
	if limit, size := limits.MaxProfileStacktraceSamples(tenantID), len(prof.Sample); limit != 0 && size > limit {
		return NewErrorf(SamplesLimit, ProfileTooManySamplesErrorMsg, phlaremodel.LabelPairsString(ls), size, limit)
	}
	var (
		depthLimit        = limits.MaxProfileStacktraceDepth(tenantID)
		labelsLimit       = limits.MaxProfileStacktraceSampleLabels(tenantID)
		symbolLengthLimit = limits.MaxProfileSymbolValueLength(tenantID)
	)
	for _, s := range prof.Sample {
		if depthLimit != 0 && len(s.LocationId) > depthLimit {
			// Truncate the deepest frames: s.LocationId[0] is the leaf.
			s.LocationId = s.LocationId[len(s.LocationId)-depthLimit:]
		}
		if labelsLimit != 0 && len(s.Label) > labelsLimit {
			return NewErrorf(SampleLabelsLimit, ProfileTooManySampleLabelsErrorMsg, phlaremodel.LabelPairsString(ls), len(s.Label), labelsLimit)
		}
	}
	if symbolLengthLimit > 0 {
		for i := range prof.StringTable {
			if len(prof.StringTable[i]) > symbolLengthLimit {
				prof.StringTable[i] = prof.StringTable[i][len(prof.StringTable[i])-symbolLengthLimit:]
			}
		}
	}
	for _, location := range prof.Location {
		if location.Id == 0 {
			return NewErrorf(MalformedProfile, "location id is 0")
		}
	}
	for _, function := range prof.Function {
		if function.Id == 0 {
			return NewErrorf(MalformedProfile, "function id is 0")
		}
	}

	if err := validateStringTableAccess(prof); err != nil {
		return err
	}
	for _, valueType := range prof.SampleType {
		stt := prof.StringTable[valueType.Type]
		if strings.Contains(stt, "-") {
			return NewErrorf(MalformedProfile, "sample type contains -")
		}
		// todo check if sample type is valid from the promql parser perspective
	}

	for _, s := range prof.StringTable {
		if !utf8.ValidString(s) {
			return NewErrorf(MalformedProfile, "invalid utf8 string hex: %s", hex.EncodeToString([]byte(s)))
		}
	}
	return nil
}

func validateStringTableAccess(prof *googlev1.Profile) error {
	if len(prof.StringTable) == 0 || prof.StringTable[0] != "" {
		return NewErrorf(MalformedProfile, "string 0 should be empty string")
	}
	for _, valueType := range prof.SampleType {
		if int(valueType.Type) >= len(prof.StringTable) {
			return NewErrorf(MalformedProfile, "sample type type string index out of range")
		}
	}
	for _, sample := range prof.Sample {
		for _, lbl := range sample.Label {
			if int(lbl.Str) >= len(prof.StringTable) || int(lbl.Key) >= len(prof.StringTable) {
				return NewErrorf(MalformedProfile, "sample label string index out of range")
			}
		}
	}
	for _, function := range prof.Function {
		if int(function.Name) >= len(prof.StringTable) {
			return NewErrorf(MalformedProfile, "function name string index out of range")
		}
		if int(function.SystemName) >= len(prof.StringTable) {
			return NewErrorf(MalformedProfile, "function system name index string out of range")
		}
		if int(function.Filename) >= len(prof.StringTable) {
			return NewErrorf(MalformedProfile, "function file name string index out of range")
		}
	}
	for _, mapping := range prof.Mapping {
		if int(mapping.Filename) >= len(prof.StringTable) {
			return NewErrorf(MalformedProfile, "mapping file name string index out of range")
		}
		if int(mapping.BuildId) >= len(prof.StringTable) {
			return NewErrorf(MalformedProfile, "mapping build id string index out of range")
		}
	}
	return nil
}

func isValidServiceName(serviceNameValue string) bool {
	return serviceNameValue != ""
}

type Error struct {
	Reason Reason
	msg    string
}

func (e *Error) Error() string {
	return e.msg
}

func NewErrorf(reason Reason, msg string, args ...interface{}) *Error {
	return &Error{
		Reason: reason,
		msg:    fmt.Sprintf(msg, args...),
	}
}

func ReasonOf(err error) Reason {
	var validationErr *Error
	ok := errors.As(err, &validationErr)
	if !ok {
		return Unknown
	}
	return validationErr.Reason
}

type RangeRequestLimits interface {
	MaxQueryLength(tenantID string) time.Duration
	MaxQueryLookback(tenantID string) time.Duration
}

type ValidatedRangeRequest struct {
	model.Interval
	IsEmpty bool
}

func ValidateRangeRequest(limits RangeRequestLimits, tenantIDs []string, req model.Interval, now model.Time) (ValidatedRangeRequest, error) {
	if req.Start == 0 || req.End == 0 {
		return ValidatedRangeRequest{}, NewErrorf(QueryMissingTimeRange, QueryMissingTimeRangeErrorMsg)
	}

	if req.Start > req.End {
		return ValidatedRangeRequest{}, NewErrorf(QueryInvalidTimeRange, QueryStartAfterEndErrorMsg)
	}

	if maxQueryLookback := validation.SmallestPositiveNonZeroDurationPerTenant(tenantIDs, limits.MaxQueryLookback); maxQueryLookback > 0 {
		minStartTime := now.Add(-maxQueryLookback)

		if req.End < minStartTime {
			// The request is fully outside the allowed range, so we can return an
			// empty response.
			level.Debug(util.Logger).Log(
				"msg", "skipping the execution of the query because its time range is before the 'max query lookback' setting",
				"reqStart", util.FormatTimeMillis(int64(req.Start)),
				"redEnd", util.FormatTimeMillis(int64(req.End)),
				"maxQueryLookback", maxQueryLookback)

			return ValidatedRangeRequest{IsEmpty: true, Interval: req}, nil
		}

		if req.Start < minStartTime {
			// Replace the start time in the request.
			level.Debug(util.Logger).Log(
				"msg", "the start time of the query has been manipulated because of the 'max query lookback' setting",
				"original", util.FormatTimeMillis(int64(req.Start)),
				"updated", util.FormatTimeMillis(int64(minStartTime)))

			req.Start = minStartTime
		}
	}

	// Enforce the max query length.
	if maxQueryLength := validation.SmallestPositiveNonZeroDurationPerTenant(tenantIDs, limits.MaxQueryLength); maxQueryLength > 0 {
		queryLen := req.End.Sub(req.Start)
		if queryLen > maxQueryLength {
			return ValidatedRangeRequest{}, NewErrorf(QueryLimit, QueryTooLongErrorMsg, queryLen, model.Duration(maxQueryLength))
		}
	}

	return ValidatedRangeRequest{Interval: req}, nil
}

func SanitizeTimeRange(limits RangeRequestLimits, tenant []string, start, end *int64) (empty bool, err error) {
	var interval model.Interval
	if start != nil {
		interval.Start = model.Time(*start)
	}
	if end != nil {
		interval.End = model.Time(*end)
	}
	validated, err := ValidateRangeRequest(limits, tenant, interval, model.Now())
	if err != nil {
		return false, err
	}
	if validated.IsEmpty {
		return true, nil
	}
	*start = int64(validated.Start)
	*end = int64(validated.End)
	return false, nil
}

type FlameGraphLimits interface {
	MaxFlameGraphNodesDefault(string) int
	MaxFlameGraphNodesMax(string) int
}

func ValidateMaxNodes(l FlameGraphLimits, tenantIDs []string, n int64) (int64, error) {
	if n == 0 {
		return int64(validation.SmallestPositiveNonZeroIntPerTenant(tenantIDs, l.MaxFlameGraphNodesDefault)), nil
	}
	maxNodes := int64(validation.SmallestPositiveNonZeroIntPerTenant(tenantIDs, l.MaxFlameGraphNodesMax))
	if maxNodes != 0 {
		if n > maxNodes {
			return 0, NewErrorf(FlameGraphLimit, MaxFlameGraphNodesErrorMsg, n, maxNodes)
		}
		if n < 0 {
			return 0, NewErrorf(FlameGraphLimit, MaxFlameGraphNodesUnlimitedErrorMsg, maxNodes)
		}
	}
	return n, nil
}
