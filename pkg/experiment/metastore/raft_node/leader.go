package raft_node

func NewLeader(raft RaftNode) *Leader { return &Leader{raft: raft} }

type Leader struct{ raft RaftNode }

func (l *Leader) CommitIndex() uint64 { return l.raft.CommitIndex() }

func (l *Leader) Verify() error {
	return WithRaftLeaderStatusDetails(l.raft.VerifyLeader().Error(), l.raft)
}

func (l *Leader) ReadIndex() (uint64, error) {
	commitIndex := l.CommitIndex()
	if err := l.Verify(); err != nil {
		return 0, err
	}
	return commitIndex, nil
}
