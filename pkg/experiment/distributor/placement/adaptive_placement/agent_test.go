package adaptive_placement

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement/adaptive_placementpb"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockadaptive_placement"
)

type agentSuite struct {
	suite.Suite

	logger log.Logger
	reg    *prometheus.Registry
	config Config
	limits *mockLimits
	store  *mockadaptive_placement.MockStore
	agent  *Agent
	error  error
}

func (s *agentSuite) SetupTest() {
	s.logger = log.NewLogfmtLogger(io.Discard)
	s.reg = prometheus.NewRegistry()
	s.config.PlacementUpdateInterval = 15 * time.Second
	s.limits = new(mockLimits)
	s.store = new(mockadaptive_placement.MockStore)
	s.agent = NewAgent(
		s.logger,
		s.reg,
		s.config,
		s.limits,
		s.store,
	)
}

func (s *agentSuite) AfterTest(_, _ string) {
	svc := s.agent.Service()
	svc.StopAsync()
	if s.error == nil {
		s.Require().NoError(svc.AwaitTerminated(context.Background()))
		s.Require().Equal(services.Terminated, svc.State())
	} else {
		s.Require().Equal(services.Failed, svc.State())
	}
	s.limits.AssertExpectations(s.T())
	s.store.AssertExpectations(s.T())
}

func (s *agentSuite) start() error {
	ctx := context.Background()
	svc := s.agent.Service()
	s.Require().NoError(svc.StartAsync(ctx))
	s.error = svc.AwaitRunning(ctx)
	return s.error
}

func Test_AgentSuite(t *testing.T) { suite.Run(t, new(agentSuite)) }

func (s *agentSuite) Test_Agent_loads_rules_on_start() {
	s.store.On("LoadRules", mock.Anything).
		Return(&adaptive_placementpb.PlacementRules{}, nil)
	s.Require().NoError(s.start())
	s.Assert().NotNil(s.agent.rules)
	s.Assert().NotNil(s.agent.Placement())
}

func (s *agentSuite) Test_Agent_service_doesnt_fail_if_rules_cant_be_found() {
	s.store.On("LoadRules", mock.Anything).
		Return((*adaptive_placementpb.PlacementRules)(nil), ErrRulesNotFound)
	s.Require().NoError(s.start())
	s.Assert().NotNil(s.agent.rules)
	s.Assert().NotNil(s.agent.Placement())
}

func (s *agentSuite) Test_Agent_service_fails_if_rules_cant_be_loaded() {
	s.store.On("LoadRules", mock.Anything).
		Return((*adaptive_placementpb.PlacementRules)(nil), fmt.Errorf("error"))
	s.Require().Error(s.start())
	s.Assert().Nil(s.agent.rules)
	s.Assert().NotNil(s.agent.Placement())
}

func (s *agentSuite) Test_Agent_updates_placement_rules() {
	s.limits.On("PlacementLimits", "tenant-a").Return(PlacementLimits{
		TenantShards:         1,
		DefaultDatasetShards: 1,
	})

	s.store.On("LoadRules", mock.Anything).
		Return(&adaptive_placementpb.PlacementRules{CreatedAt: 100}, nil).
		Once()

	s.Require().NoError(s.start())

	p := s.agent.Placement()
	s.Require().NotNil(p)
	policy := p.Policy(placement.Key{
		TenantID:    "tenant-a",
		DatasetName: "dataset-a",
	})
	s.Assert().Equal(1, policy.TenantShards)
	s.Assert().Equal(1, policy.DatasetShards)

	s.store.On("LoadRules", mock.Anything).
		Return(&adaptive_placementpb.PlacementRules{
			CreatedAt: 150,
			Tenants: []*adaptive_placementpb.TenantPlacement{
				{TenantId: "tenant-a"},
			},
			Datasets: []*adaptive_placementpb.DatasetPlacement{
				{
					Name:              "dataset-a",
					TenantShardLimit:  10,
					DatasetShardLimit: 10,
				},
			},
		}, nil).
		Once()

	s.agent.loadRules(context.Background())
	policy = p.Policy(placement.Key{
		TenantID:    "tenant-a",
		DatasetName: "dataset-a",
	})
	s.Assert().Equal(10, policy.TenantShards)
	s.Assert().Equal(10, policy.DatasetShards)
}

func (s *agentSuite) Test_Agent_ignored_outdated_rules() {
	s.limits.On("PlacementLimits", "tenant-a").Return(PlacementLimits{
		TenantShards:         1,
		DefaultDatasetShards: 1,
	})

	s.store.On("LoadRules", mock.Anything).
		Return(&adaptive_placementpb.PlacementRules{CreatedAt: 100}, nil).
		Once()

	s.Require().NoError(s.start())

	p := s.agent.Placement()
	s.Require().NotNil(p)
	policy := p.Policy(placement.Key{
		TenantID:    "tenant-a",
		DatasetName: "dataset-a",
	})
	s.Assert().Equal(1, policy.TenantShards)
	s.Assert().Equal(1, policy.DatasetShards)

	s.store.On("LoadRules", mock.Anything).
		Return(&adaptive_placementpb.PlacementRules{
			CreatedAt: 10,
			Tenants: []*adaptive_placementpb.TenantPlacement{
				{TenantId: "tenant-a"},
			},
			Datasets: []*adaptive_placementpb.DatasetPlacement{
				{
					Name:              "dataset-a",
					TenantShardLimit:  10,
					DatasetShardLimit: 10,
				},
			},
		}, nil).
		Once()

	s.agent.loadRules(context.Background())
	policy = p.Policy(placement.Key{
		TenantID:    "tenant-a",
		DatasetName: "dataset-a",
	})
	s.Assert().Equal(1, policy.TenantShards)
	s.Assert().Equal(1, policy.DatasetShards)
}
