package validation

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"

	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/util"
	"github.com/grafana/phlare/pkg/util/validation"
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
	// OutOfOrder is a reason for discarding profiles when Phlare doesn't accept out
	// of order profiles.
	OutOfOrder Reason = "out_of_order"
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
	SeriesLimit Reason = "series_limit"
	QueryLimit  Reason = "query_limit"

	SeriesLimitErrorMsg            = "Maximum active series limit exceeded (%d/%d), reduce the number of active streams (reduce labels or reduce label values), or contact your administrator to see if the limit can be increased"
	MissingLabelsErrorMsg          = "error at least one label pair is required per profile"
	InvalidLabelsErrorMsg          = "invalid labels '%s' with error: %s"
	MaxLabelNamesPerSeriesErrorMsg = "profile series '%s' has %d label names; limit %d"
	LabelNameTooLongErrorMsg       = "profile with labels '%s' has label name too long: '%s'"
	LabelValueTooLongErrorMsg      = "profile with labels '%s' has label value too long: '%s'"
	DuplicateLabelNamesErrorMsg    = "profile with labels '%s' has duplicate label name: '%s'"
	QueryTooLongErrorMsg           = "the query time range exceeds the limit (query length: %s, limit: %s)"
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
	MaxLabelNameLength(userID string) int
	MaxLabelValueLength(userID string) int
	MaxLabelNamesPerSeries(userID string) int
}

// ValidateLabels validates the labels of a profile.
func ValidateLabels(limits LabelValidationLimits, userID string, ls []*typesv1.LabelPair) error {
	if len(ls) == 0 {
		return NewErrorf(MissingLabels, MissingLabelsErrorMsg)
	}
	sort.Sort(phlaremodel.Labels(ls))
	numLabelNames := len(ls)
	maxLabels := limits.MaxLabelNamesPerSeries(userID)
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
	lastLabelName := ""

	for _, l := range ls {
		if len(l.Name) > limits.MaxLabelNameLength(userID) {
			return NewErrorf(LabelNameTooLong, LabelNameTooLongErrorMsg, phlaremodel.LabelPairsString(ls), l.Name)
		} else if len(l.Value) > limits.MaxLabelValueLength(userID) {
			return NewErrorf(LabelValueTooLong, LabelValueTooLongErrorMsg, phlaremodel.LabelPairsString(ls), l.Value)
		} else if !model.LabelName(l.Name).IsValid() {
			return NewErrorf(InvalidLabels, InvalidLabelsErrorMsg, phlaremodel.LabelPairsString(ls), "invalid label name '"+l.Name+"'")
		} else if !model.LabelValue(l.Value).IsValid() {
			return NewErrorf(InvalidLabels, InvalidLabelsErrorMsg, phlaremodel.LabelPairsString(ls), "invalid label value '"+l.Value+"'")
		} else if cmp := strings.Compare(lastLabelName, l.Name); cmp == 0 {
			return NewErrorf(DuplicateLabelNames, DuplicateLabelNamesErrorMsg, phlaremodel.LabelPairsString(ls), l.Name)
		}
		lastLabelName = l.Name
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
