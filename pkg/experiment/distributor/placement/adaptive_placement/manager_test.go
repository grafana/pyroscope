package adaptive_placement

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockadaptive_placement"
)

type managerSuite struct {
	suite.Suite

	logger  log.Logger
	config  Config
	limits  *mockLimits
	store   *mockadaptive_placement.MockStore
	manager *Manager
}

func (s *managerSuite) SetupTest() {
	s.logger = log.NewLogfmtLogger(io.Discard)
	s.config = Config{
		PlacementUpdateInterval:  15 * time.Second,
		PlacementRetentionPeriod: 15 * time.Minute,
		StatsAggregationWindow:   3 * time.Minute,
		StatsRetentionPeriod:     5 * time.Minute,
		StatsConfidencePeriod:    5 * time.Minute,
	}
	s.limits = new(mockLimits)
	s.store = new(mockadaptive_placement.MockStore)
	s.manager = NewManager(
		s.logger,
		nil,
		s.config,
		s.limits,
		s.store,
	)
}

func (s *managerSuite) BeforeTest(_, _ string) {
	ctx := context.Background()
	svc := s.manager.Service()
	s.Require().NoError(svc.StartAsync(ctx))
	s.Require().NoError(svc.AwaitRunning(ctx))
}

func (s *managerSuite) AfterTest(_, _ string) {
	svc := s.manager.Service()
	svc.StopAsync()
	s.Require().NoError(svc.AwaitTerminated(context.Background()))
	s.Require().Equal(services.Terminated, svc.State())
	s.limits.AssertExpectations(s.T())
	s.store.AssertExpectations(s.T())
}

func Test_ManagerSuite(t *testing.T) { suite.Run(t, new(managerSuite)) }

func (s *managerSuite) Test_Manager_updates_rules_if_started() {
	oldRules := &adaptive_placementpb.PlacementRules{CreatedAt: 100}
	s.store.On("LoadRules", mock.Anything).Return(oldRules, nil)

	newRules := func(r *adaptive_placementpb.PlacementRules) bool { return r.CreatedAt > 100 }
	s.store.On("StoreRules", mock.Anything, mock.MatchedBy(newRules)).Return(nil).Once()
	s.store.On("StoreStats", mock.Anything, mock.Anything).Return(nil).Once()

	s.manager.Start()
	s.manager.updateRules(context.Background())
}

func (s *managerSuite) Test_Manager_doesnt_update_rules_if_stopped() {
	oldRules := &adaptive_placementpb.PlacementRules{CreatedAt: 100}
	s.store.On("LoadRules", mock.Anything).Return(oldRules, nil)

	newRules := func(r *adaptive_placementpb.PlacementRules) bool { return r.CreatedAt > 100 }
	s.store.On("StoreRules", mock.Anything, mock.MatchedBy(newRules)).Return(nil).Once()
	s.store.On("StoreStats", mock.Anything, mock.Anything).Return(nil).Once()

	s.manager.Start()
	s.manager.updateRules(context.Background())

	s.manager.Stop()
	s.manager.updateRules(context.Background())
}

func (s *managerSuite) Test_Manager_doesnt_update_rules_if_store_fails() {
	s.store.On("LoadRules", mock.Anything).
		Return((*adaptive_placementpb.PlacementRules)(nil), fmt.Errorf("error"))

	s.manager.Start()
	s.manager.updateRules(context.Background())
}

func (s *managerSuite) Test_Manager_updates_rules_if_no_rules_not_found() {
	s.store.On("LoadRules", mock.Anything).
		Return((*adaptive_placementpb.PlacementRules)(nil), ErrRulesNotFound)

	newRules := func(r *adaptive_placementpb.PlacementRules) bool { return r.CreatedAt > 0 }
	s.store.On("StoreRules", mock.Anything, mock.MatchedBy(newRules)).Return(nil).Once()
	s.store.On("StoreStats", mock.Anything, mock.Anything).Return(nil).Once()

	s.manager.Start()
	s.manager.updateRules(context.Background())
}
