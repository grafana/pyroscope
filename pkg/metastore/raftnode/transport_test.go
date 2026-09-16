package raftnode

import (
	"net"
	"testing"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/require"
)

// TestTransport_PipeliningDisabled pins the workaround in newTransport.
// Raft's pipeline replication deadlocks on v1.7.3 (see the
// raftMaxRPCsInFlight comment, and TestRaftPipelineDeadlock in
// pkg/metastore/test for the end-to-end reproduction), so the transport must
// refuse to pipeline and leave replicate() in synchronous RPC mode.
func TestTransport_PipeliningDisabled(t *testing.T) {
	advertise, err := net.ResolveTCPAddr("tcp", "localhost:0")
	require.NoError(t, err)

	transport, err := newTransport(Config{
		BindAddress:           "localhost:0",
		TransportConnPoolSize: defaultTransportConnPoolSize,
		TransportTimeout:      defaultTransportTimeout,
	}, advertise)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, transport.Close()) })

	// Raft checks maxInFlight before dialing, so no peer has to exist.
	_, err = transport.AppendEntriesPipeline("peer", transport.LocalAddr())
	require.ErrorIs(t, err, raft.ErrPipelineReplicationNotSupported)
}
