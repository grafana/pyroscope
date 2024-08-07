package phlaredb

import (
	"context"
	"io"

	"connectrpc.com/connect"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

type BidiServerMerge[Res any, Req any] interface {
	Send(Res) error
	Receive() (Req, error)
}

type ProfileWithIndex struct {
	Profile
	Index int
}

type indexedProfileIterator struct {
	iter.Iterator[Profile]
	querierIndex int
}

func (pqi *indexedProfileIterator) At() ProfileWithIndex {
	return ProfileWithIndex{
		Profile: pqi.Iterator.At(),
		Index:   pqi.querierIndex,
	}
}

type filterRequest interface {
	*ingestv1.MergeProfilesStacktracesRequest |
		*ingestv1.MergeProfilesLabelsRequest |
		*ingestv1.MergeProfilesPprofRequest |
		*ingestv1.MergeSpanProfileRequest
}

type filterResponse interface {
	*ingestv1.MergeProfilesStacktracesResponse |
		*ingestv1.MergeProfilesLabelsResponse |
		*ingestv1.MergeProfilesPprofResponse |
		*ingestv1.MergeSpanProfileResponse
}

func rewriteEOFError(err error) error {
	if errors.Is(err, io.EOF) {
		return connect.NewError(connect.CodeCanceled, errors.New("client closed stream"))
	}
	return err
}

// filterProfiles merges and dedupe profiles from different iterators and allow filtering via a bidi stream.
func filterProfiles[B BidiServerMerge[Res, Req], Res filterResponse, Req filterRequest](
	ctx context.Context, profiles []iter.Iterator[Profile], batchProfileSize int, stream B,
) ([][]Profile, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "filterProfiles")
	defer sp.Finish()
	selection := make([][]Profile, len(profiles))
	selectProfileResult := &ingestv1.ProfileSets{
		Profiles:     make([]*ingestv1.SeriesProfile, 0, batchProfileSize),
		Fingerprints: make([]uint64, 0, batchProfileSize),
	}
	its := make([]iter.Iterator[ProfileWithIndex], len(profiles))
	for i, iter := range profiles {
		iter := iter
		its[i] = &indexedProfileIterator{
			Iterator:     iter,
			querierIndex: i,
		}
	}
	if err := iter.ReadBatch(ctx, phlaremodel.NewMergeIterator(ProfileWithIndex{
		Profile: maxBlockProfile,
		Index:   0,
	}, true, its...), batchProfileSize, func(ctx context.Context, batch []ProfileWithIndex) error {
		sp.LogFields(
			otlog.Int("batch_len", len(batch)),
			otlog.Int("batch_requested_size", batchProfileSize),
		)

		seriesByFP := map[model.Fingerprint]int{}
		selectProfileResult.Profiles = selectProfileResult.Profiles[:0]
		selectProfileResult.Fingerprints = selectProfileResult.Fingerprints[:0]

		for _, profile := range batch {
			var ok bool
			var idx int
			fp := profile.Fingerprint()
			idx, ok = seriesByFP[fp]
			if !ok {
				idx = len(selectProfileResult.Fingerprints)
				seriesByFP[fp] = idx
				selectProfileResult.Fingerprints = append(selectProfileResult.Fingerprints, uint64(fp))
			}
			selectProfileResult.Profiles = append(selectProfileResult.Profiles, &ingestv1.SeriesProfile{
				LabelIndex: int32(idx),
				Timestamp:  int64(profile.Timestamp()),
			})
		}

		sp.LogFields(otlog.String("msg", "sending batch to client"))
		var err error
		switch s := BidiServerMerge[Res, Req](stream).(type) {
		case BidiServerMerge[*ingestv1.MergeProfilesStacktracesResponse, *ingestv1.MergeProfilesStacktracesRequest]:
			err = s.Send(&ingestv1.MergeProfilesStacktracesResponse{
				SelectedProfiles: selectProfileResult,
			})
		case BidiServerMerge[*ingestv1.MergeProfilesLabelsResponse, *ingestv1.MergeProfilesLabelsRequest]:
			err = s.Send(&ingestv1.MergeProfilesLabelsResponse{
				SelectedProfiles: selectProfileResult,
			})
		case BidiServerMerge[*ingestv1.MergeProfilesPprofResponse, *ingestv1.MergeProfilesPprofRequest]:
			err = s.Send(&ingestv1.MergeProfilesPprofResponse{
				SelectedProfiles: selectProfileResult,
			})
		case BidiServerMerge[*ingestv1.MergeSpanProfileResponse, *ingestv1.MergeSpanProfileRequest]:
			err = s.Send(&ingestv1.MergeSpanProfileResponse{
				SelectedProfiles: selectProfileResult,
			})
		}
		// read a batch of profiles and sends it.

		if err != nil {
			return rewriteEOFError(err)
		}
		sp.LogFields(otlog.String("msg", "batch sent to client"))

		sp.LogFields(otlog.String("msg", "reading selection from client"))

		// handle response for the batch.
		var selected []bool
		switch s := BidiServerMerge[Res, Req](stream).(type) {
		case BidiServerMerge[*ingestv1.MergeProfilesStacktracesResponse, *ingestv1.MergeProfilesStacktracesRequest]:
			selectionResponse, err := s.Receive()
			if err != nil {
				return rewriteEOFError(err)
			}
			selected = selectionResponse.Profiles
		case BidiServerMerge[*ingestv1.MergeProfilesLabelsResponse, *ingestv1.MergeProfilesLabelsRequest]:
			selectionResponse, err := s.Receive()
			if err != nil {
				return rewriteEOFError(err)
			}
			selected = selectionResponse.Profiles
		case BidiServerMerge[*ingestv1.MergeProfilesPprofResponse, *ingestv1.MergeProfilesPprofRequest]:
			selectionResponse, err := s.Receive()
			if err != nil {
				return rewriteEOFError(err)
			}
			selected = selectionResponse.Profiles
		case BidiServerMerge[*ingestv1.MergeSpanProfileResponse, *ingestv1.MergeSpanProfileRequest]:
			selectionResponse, err := s.Receive()
			if err != nil {
				return rewriteEOFError(err)
			}
			selected = selectionResponse.Profiles
		}
		sp.LogFields(otlog.String("msg", "selection received"))
		for i, k := range selected {
			if k {
				selection[batch[i].Index] = append(selection[batch[i].Index], batch[i].Profile)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return selection, nil
}
