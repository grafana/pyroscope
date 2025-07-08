package queryfrontend

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

var (
	errMissingServiceName = errors.New("service name is missing")
	errMissingProfileType = errors.New("profile type is missing")
	errInvalidProfileType = errors.New("invalid profile type")
)

var profileTypeLabels2 = []string{
	phlaremodel.LabelNameProfileType,
	phlaremodel.LabelNameServiceName,
}

var profileTypeLabels5 = []string{
	phlaremodel.LabelNameProfileName,
	phlaremodel.LabelNameProfileType,
	phlaremodel.LabelNameType,
	"pyroscope_app",
	phlaremodel.LabelNameServiceName,
}

func (q *QueryFrontend) isProfileTypeQuery(labels, matchers []string) bool {
	if len(matchers) > 0 {
		return false
	}
	var s []string
	switch len(labels) {
	case 2:
		s = profileTypeLabels2
	case 5:
		s = profileTypeLabels5
	default:
		return false
	}
	sort.Strings(labels)
	return slices.Compare(s, labels) == 0
}

func (q *QueryFrontend) queryProfileTypeMetadataLabels(
	ctx context.Context,
	tenants []string,
	startTime int64,
	endTime int64,
	labels []string,
) (*connect.Response[querierv1.SeriesResponse], error) {
	meta, err := q.metadataQueryClient.QueryMetadataLabels(ctx, &metastorev1.QueryMetadataLabelsRequest{
		TenantId:  tenants,
		StartTime: startTime,
		EndTime:   endTime,
		Query:     "{}",
		Labels: []string{
			phlaremodel.LabelNameServiceName,
			phlaremodel.LabelNameProfileType,
		},
	})
	if err != nil {
		return nil, err
	}
	meta.Labels = q.buildProfileTypeMetadataLabels(meta.Labels, labels)
	return connect.NewResponse(&querierv1.SeriesResponse{LabelsSet: meta.Labels}), nil
}

func (q *QueryFrontend) buildProfileTypeMetadataLabels(labels []*typesv1.Labels, names []string) []*typesv1.Labels {
	for _, ls := range labels {
		if err := sanitizeProfileTypeMetadataLabels(ls, names); err != nil {
			level.Warn(q.logger).Log("msg", "malformed label set", "labels", phlaremodel.LabelPairsString(ls.Labels), "err", err)
			ls.Labels = nil
		}
	}
	labels = slices.DeleteFunc(labels, func(ls *typesv1.Labels) bool {
		return len(ls.Labels) == 0
	})
	slices.SortFunc(labels, func(a, b *typesv1.Labels) int {
		return phlaremodel.CompareLabelPairs(a.Labels, b.Labels)
	})
	return labels
}

func sanitizeProfileTypeMetadataLabels(ls *typesv1.Labels, names []string) error {
	var serviceName, profileType string
	for _, l := range ls.Labels {
		switch l.Name {
		case phlaremodel.LabelNameServiceName:
			serviceName = l.Value
		case phlaremodel.LabelNameProfileType:
			profileType = l.Value
		}
	}
	if serviceName == "" {
		return errMissingServiceName
	}
	if profileType == "" {
		return errMissingProfileType
	}
	pt, err := phlaremodel.ParseProfileTypeSelector(profileType)
	if err != nil {
		return fmt.Errorf("%w: %w", errInvalidProfileType, err)
	}
	if len(names) == 5 {
		// Replace the labels with the expected ones.
		ls.Labels = append(ls.Labels[:0], []*typesv1.LabelPair{
			{Name: phlaremodel.LabelNameProfileType, Value: profileType},
			{Name: phlaremodel.LabelNameServiceName, Value: serviceName},
			{Name: phlaremodel.LabelNameProfileName, Value: pt.Name},
			{Name: phlaremodel.LabelNameType, Value: pt.SampleType},
		}...)
	}
	sort.Sort(phlaremodel.Labels(ls.Labels))
	return nil
}
