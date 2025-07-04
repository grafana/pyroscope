package admin

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	metastoreclient "github.com/grafana/pyroscope/pkg/metastore/client"
	"github.com/grafana/pyroscope/pkg/metastore/discovery"
	"github.com/grafana/pyroscope/pkg/metastore/raftnode/raftnodepb"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

type configChangeRequest struct {
	serverId    string
	currentTerm uint64
}

type formActionHandler func(http.ResponseWriter, *http.Request, configChangeRequest) error

type Admin struct {
	service services.Service

	logger log.Logger

	servers      []discovery.Server
	leaderClient raftnodepb.RaftNodeServiceClient // used to make operational calls (e.g., removing nodes)

	metastoreClient *metastoreclient.Client // used to test the metastoreclient.Client implementation

	actionHandlers map[string]formActionHandler
}

func (a *Admin) Service() services.Service {
	return a.service
}

type raftNodeServiceClient struct {
	raftnodepb.RaftNodeServiceClient
	conn *grpc.ClientConn
}

func New(
	client raftnodepb.RaftNodeServiceClient,
	logger log.Logger,
	metastoreAddress string,
	metastoreClient *metastoreclient.Client,
) (*Admin, error) {
	adm := &Admin{
		leaderClient:    client,
		logger:          logger,
		actionHandlers:  make(map[string]formActionHandler),
		metastoreClient: metastoreClient,
	}
	adm.addFormActionHandlers()
	adm.service = services.NewIdleService(adm.starting, adm.stopping)

	disc, err := discovery.NewDiscovery(logger, metastoreAddress, nil)
	if err != nil {
		return nil, err
	}
	disc.Subscribe(adm)

	return adm, nil
}

func (a *Admin) starting(context.Context) error { return nil }
func (a *Admin) stopping(error) error           { return nil }

func (a *Admin) Servers(servers []discovery.Server) {
	a.servers = servers
	slices.SortFunc(a.servers, func(a, b discovery.Server) int {
		return strings.Compare(string(a.Raft.ID), string(b.Raft.ID))
	})
}

func (a *Admin) NodeListHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			for fieldName, handler := range a.actionHandlers {
				field := r.FormValue(fieldName)
				if field != "" {
					changeRequest := configChangeRequest{
						serverId: field,
					}
					if currentTerm := r.FormValue("current-term"); currentTerm != "" {
						parsedTerm, err := strconv.ParseUint(currentTerm, 10, 64)
						if err == nil {
							changeRequest.currentTerm = parsedTerm
						}
					}
					if err := handler(w, r, changeRequest); err != nil {
						httputil.Error(w, err)
						return
					}
					w.Header().Set("Location", "#")
					w.WriteHeader(http.StatusFound)
					return
				}
			}
		}

		raftState := a.fetchRaftState(r.Context())
		err := pageTemplates.nodesTemplate.Execute(w, nodesPageContent{
			DiscoveredServers: a.servers,
			Raft:              raftState,
			Now:               time.Now().UTC(),
		})
		if err != nil {
			httputil.Error(w, err)
		}
	})
}

func (a *Admin) fetchRaftState(ctx context.Context) *raftNodeState {
	observedLeaders := make(map[string]int)
	numRaftNodes := 0
	nodes := make([]*metastoreNode, 0, len(a.servers))

	for _, s := range a.servers {
		cl, err := newClient(s.ResolvedAddress)
		if err != nil {
			level.Warn(a.logger).Log("msg", "missing client for server", "server", s)
			continue
		}
		node := &metastoreNode{
			DiscoveryServerId: string(s.Raft.ID),
			ResolvedAddress:   s.ResolvedAddress,
		}
		nodes = append(nodes, node)

		res, err := cl.NodeInfo(ctx, &raftnodepb.NodeInfoRequest{})
		_ = cl.conn.Close()

		if err != nil {
			level.Warn(a.logger).Log("msg", "error fetching node info", "server", s, "err", err)
			continue
		}
		nInfo := res.Node

		node.RaftServerId = nInfo.ServerId
		node.Member = nInfo.LeaderId != ""
		node.State = nInfo.State
		node.LeaderId = nInfo.LeaderId
		node.ConfigIndex = nInfo.ConfigurationIndex
		node.NumPeers = len(nInfo.Peers)
		node.CurrentTerm = nInfo.CurrentTerm
		node.LastIndex = nInfo.LastIndex
		node.CommitIndex = nInfo.CommitIndex
		node.AppliedIndex = nInfo.AppliedIndex
		node.BuildVersion = nInfo.BuildVersion
		node.BuildRevision = nInfo.BuildRevision
		node.Stats = make(map[string]string)
		for i, n := range nInfo.Stats.Name {
			node.Stats[n] = nInfo.Stats.Value[i]
		}

		if node.Member {
			numRaftNodes++
			observedLeaders[node.LeaderId]++
		}
	}

	currentTerm := findCurrentTerm(nodes)

	return &raftNodeState{
		Nodes:           nodes,
		ObservedLeaders: observedLeaders,
		CurrentTerm:     currentTerm,
		NumNodes:        numRaftNodes,
	}
}

func findCurrentTerm(nodes []*metastoreNode) uint64 {
	terms := make(map[uint64]int)
	for _, node := range nodes {
		if node.Member {
			terms[node.CurrentTerm]++
		}
	}
	// TODO aleks-p: in case of a mismatch in reported current terms, we bypass any validation
	term := uint64(math.MaxUint64)
	if len(terms) == 1 {
		for k := range terms {
			term = k
		}
	}
	return term
}

func newClient(address string) (*raftNodeServiceClient, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &raftNodeServiceClient{
		RaftNodeServiceClient: raftnodepb.NewRaftNodeServiceClient(conn),
		conn:                  conn,
	}, nil
}

func (a *Admin) addFormActionHandlers() {
	a.actionHandlers["add"] = func(w http.ResponseWriter, r *http.Request, cr configChangeRequest) error {
		_, err := a.leaderClient.AddNode(r.Context(), &raftnodepb.AddNodeRequest{
			ServerId:    cr.serverId,
			CurrentTerm: cr.currentTerm,
		})
		return err
	}
	a.actionHandlers["remove"] = func(w http.ResponseWriter, r *http.Request, cr configChangeRequest) error {
		_, err := a.leaderClient.RemoveNode(r.Context(), &raftnodepb.RemoveNodeRequest{
			ServerId:    cr.serverId,
			CurrentTerm: cr.currentTerm,
		})
		return err
	}
	a.actionHandlers["promote"] = func(w http.ResponseWriter, r *http.Request, cr configChangeRequest) error {
		_, err := a.leaderClient.PromoteToLeader(r.Context(), &raftnodepb.PromoteToLeaderRequest{
			ServerId:    cr.serverId,
			CurrentTerm: cr.currentTerm,
		})
		return err
	}
	a.actionHandlers["demote"] = func(w http.ResponseWriter, r *http.Request, cr configChangeRequest) error {
		_, err := a.leaderClient.DemoteLeader(r.Context(), &raftnodepb.DemoteLeaderRequest{
			ServerId:    cr.serverId,
			CurrentTerm: cr.currentTerm,
		})
		return err
	}
}

func (a *Admin) ClientTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raftState := a.fetchRaftState(r.Context())
		content := clientTestPageContent{
			Raft: raftState,
			Now:  time.Now().UTC(),
		}

		if r.Method == http.MethodPost {
			start := time.Now()
			res, err := a.metastoreClient.ReadIndex(r.Context(), &raftnodepb.ReadIndexRequest{})
			content.TestResponseTime = time.Since(start)
			if err != nil {
				content.TestResponse = err.Error()
			} else {
				content.TestResponse = fmt.Sprintf("Success! (index: %d, term: %d)", res.CommitIndex, res.Term)
			}
		}

		err := pageTemplates.clientTestTemplate.Execute(w, content)
		if err != nil {
			httputil.Error(w, err)
		}
	})
}
