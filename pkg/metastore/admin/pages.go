package admin

import (
	_ "embed"
	"html/template"
	"time"

	"github.com/grafana/pyroscope/pkg/metastore/discovery"
)

//go:embed metastore.nodes.gohtml
var nodesPageHtml string

//go:embed metastore.client.gohtml
var clientTestPageHtml string

type metastoreNode struct {
	// from Discovery
	DiscoveryServerId string
	RaftServerId      string
	ResolvedAddress   string

	// from Raft
	Member        bool
	State         string
	CommitIndex   uint64
	AppliedIndex  uint64
	LastIndex     uint64
	LeaderId      string
	ConfigIndex   uint64
	NumPeers      int
	CurrentTerm   uint64
	BuildVersion  string
	BuildRevision string
	Stats         map[string]string
}

type raftNodeState struct {
	Nodes           []*metastoreNode
	ObservedLeaders map[string]int
	CurrentTerm     uint64
	NumNodes        int
}

type nodesPageContent struct {
	DiscoveredServers []discovery.Server
	Raft              *raftNodeState
	Now               time.Time
}

type clientTestPageContent struct {
	Raft             *raftNodeState
	Now              time.Time
	TestResponse     string
	TestResponseTime time.Duration
}

type templates struct {
	nodesTemplate      *template.Template
	clientTestTemplate *template.Template
}

var pageTemplates = initTemplates()

func initTemplates() *templates {
	nodesTemplate := template.New("nodes")
	template.Must(nodesTemplate.Parse(nodesPageHtml))
	clientTestTemplate := template.New("clientTest")
	template.Must(clientTestTemplate.Parse(clientTestPageHtml))
	t := &templates{
		nodesTemplate:      nodesTemplate,
		clientTestTemplate: clientTestTemplate,
	}
	return t
}
