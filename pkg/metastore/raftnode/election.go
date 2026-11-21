package raftnode

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const (
	slowWALWriteThreshold = 100 * time.Millisecond
	slowWALCampaignDelay  = 5 * time.Second
)

var errSlowWALCampaign = errors.New("delaying pre-vote campaign due to slow WAL writes")

// slowWALTransport delays only outgoing pre-votes. Incoming votes, heartbeats,
// and explicit leadership transfers use the underlying transport unchanged.
type slowWALTransport struct {
	*raft.NetworkTransport
	latency prometheus.Summary

	mu         sync.Mutex
	term       uint64
	delayUntil time.Time
}

func (t *slowWALTransport) RequestPreVote(id raft.ServerID, target raft.ServerAddress, args *raft.RequestPreVoteRequest, resp *raft.RequestPreVoteResponse) error {
	delayed, err := t.delayCampaign(args.Term)
	if err != nil {
		return err
	}
	if delayed {
		return errSlowWALCampaign
	}
	return t.NetworkTransport.RequestPreVote(id, target, args, resp)
}

func (t *slowWALTransport) delayCampaign(term uint64) (bool, error) {
	var metric dto.Metric
	if err := t.latency.Write(&metric); err != nil {
		return false, fmt.Errorf("read WAL write latency: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if term > t.term {
		t.term = term
		t.delayUntil = time.Time{}
	}
	for _, quantile := range metric.GetSummary().GetQuantile() {
		// An empty Summary reports NaN, which does not exceed the threshold.
		if quantile.GetQuantile() == 0.99 && quantile.GetValue() > slowWALWriteThreshold.Seconds() {
			if t.delayUntil.IsZero() {
				t.delayUntil = time.Now().Add(slowWALCampaignDelay)
			}
			// Pre-vote retries keep the same term, so they cannot renew the penalty.
			return time.Now().Before(t.delayUntil), nil
		}
	}
	return false, nil
}
