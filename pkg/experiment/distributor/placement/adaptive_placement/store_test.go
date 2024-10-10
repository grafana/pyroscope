package adaptive_placement

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockobjstore"
)

type storeSuite struct {
	suite.Suite

	bucket *mockobjstore.MockBucket
	store  *BucketStore
}

func (s *storeSuite) SetupTest() {
	s.bucket = new(mockobjstore.MockBucket)
	s.store = NewStore(s.bucket)
}

func Test_StoreSuite(t *testing.T) { suite.Run(t, new(storeSuite)) }

func (s *storeSuite) Test_LoadRules() {
	rules := &adaptive_placementpb.PlacementRules{CreatedAt: 1}
	s.bucket.On("Get", mock.Anything, rulesFilePath).
		Return(s.marshal(rules), nil)
	loaded, err := s.store.LoadRules(context.Background())
	s.NoError(err)
	s.Equal(rules, loaded)
	s.bucket.AssertExpectations(s.T())
}

func (s *storeSuite) Test_LoadRules_not_found() {
	notFound := fmt.Errorf("not found")
	s.bucket.On("Get", mock.Anything, rulesFilePath).
		Return(nil, notFound)
	s.bucket.On("IsObjNotFoundErr", notFound).Return(true)
	_, err := s.store.LoadRules(context.Background())
	s.ErrorIs(err, ErrRulesNotFound)
	s.bucket.AssertExpectations(s.T())
}

func (s *storeSuite) Test_StoreRules() {
	rules := &adaptive_placementpb.PlacementRules{CreatedAt: 1}
	s.bucket.On("Upload", mock.Anything, rulesFilePath, mock.Anything).
		Run(func(args mock.Arguments) {
			var stored adaptive_placementpb.PlacementRules
			s.unmarshal(args[2].(io.Reader), &stored)
			s.Equal(rules, &stored)
		}).
		Return(nil).
		Once()
	s.NoError(s.store.StoreRules(context.Background(), rules))
	s.bucket.AssertExpectations(s.T())
}

func (s *storeSuite) Test_LoadStats() {
	stats := &adaptive_placementpb.DistributionStats{CreatedAt: 1}
	s.bucket.On("Get", mock.Anything, statsFilePath).
		Return(s.marshal(stats), nil)
	loaded, err := s.store.LoadStats(context.Background())
	s.NoError(err)
	s.Equal(stats, loaded)
	s.bucket.AssertExpectations(s.T())
}

func (s *storeSuite) Test_LoadStats_not_found() {
	notFound := fmt.Errorf("not found")
	s.bucket.On("Get", mock.Anything, statsFilePath).
		Return(nil, notFound)
	s.bucket.On("IsObjNotFoundErr", notFound).Return(true)
	_, err := s.store.LoadStats(context.Background())
	s.ErrorIs(err, ErrStatsNotFound)
	s.bucket.AssertExpectations(s.T())
}

func (s *storeSuite) Test_StoreStats() {
	stats := &adaptive_placementpb.DistributionStats{CreatedAt: 1}
	s.bucket.On("Upload", mock.Anything, statsFilePath, mock.Anything).
		Run(func(args mock.Arguments) {
			var stored adaptive_placementpb.DistributionStats
			s.unmarshal(args[2].(io.Reader), &stored)
			s.Equal(stats, &stored)
		}).
		Return(nil).
		Once()
	s.NoError(s.store.StoreStats(context.Background(), stats))
	s.bucket.AssertExpectations(s.T())
}

func (s *storeSuite) marshal(m vtProtoMessage) io.ReadCloser {
	b, err := m.MarshalVT()
	s.Require().NoError(err)
	return io.NopCloser(bytes.NewBuffer(b))
}

func (s *storeSuite) unmarshal(r io.Reader, m vtProtoMessage) {
	b, err := io.ReadAll(r)
	s.Require().NoError(err)
	s.Require().NoError(m.UnmarshalVT(b))
}
