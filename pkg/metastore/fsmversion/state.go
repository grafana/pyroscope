package fsmversion

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"go.etcd.io/bbolt"
	bbolterrors "go.etcd.io/bbolt/errors"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

var ErrUnsupportedVersion = errors.New("unsupported FSM version")

var (
	bucketName = []byte("fsm_version")
	versionKey = []byte("version")
)

type State struct {
	logger    log.Logger
	supported Version
	active    atomic.Uint32
	metrics   *metrics
}

func NewState(logger log.Logger, supported Version, reg prometheus.Registerer) *State {
	s := &State{
		logger:    logger,
		supported: supported,
		metrics:   newMetrics(reg),
	}
	s.metrics.supported.Set(float64(supported))
	return s
}

func (s *State) Supported() Version { return s.supported }

func (s *State) Active() Version { return Version(s.active.Load()) }

func (s *State) Init(tx *bbolt.Tx) error {
	_, err := tx.CreateBucketIfNotExists(bucketName)
	return err
}

func (s *State) Restore(tx *bbolt.Tx) error {
	v := Get(tx)
	if err := s.checkSupported(v); err != nil {
		level.Error(s.logger).Log("msg", "refusing to restore state", "err", err)
		return err
	}
	s.setActive(v)
	return nil
}

func (s *State) ValidateSnapshot(tx *bbolt.Tx) error {
	if err := s.checkSupported(Get(tx)); err != nil {
		level.Error(s.logger).Log("msg", "refusing to restore snapshot", "err", err)
		return err
	}
	return nil
}

func (s *State) SetVersion(
	_ context.Context, tx *bbolt.Tx, cmd *raft.Log, req *raft_log.SetFSMVersionRequest,
) (*raft_log.SetFSMVersionResponse, error) {
	current := Get(tx)
	if req.Term != cmd.Term {
		level.Warn(s.logger).Log(
			"msg", "rejecting FSM version change; term mismatch: leader has changed",
			"current_term", cmd.Term,
			"request_term", req.Term,
			"version", req.Version,
		)
		return &raft_log.SetFSMVersionResponse{Version: uint32(current)}, nil
	}
	v := Version(req.Version)
	if v <= current {
		return &raft_log.SetFSMVersionResponse{Version: uint32(current)}, nil
	}
	if err := s.checkSupported(v); err != nil {
		level.Error(s.logger).Log("msg", "cannot apply FSM version change", "err", err, "raft_log_index", cmd.Index)
		return nil, err
	}
	if err := put(tx, v); err != nil {
		return nil, err
	}
	s.setActive(v)
	level.Info(s.logger).Log(
		"msg", "FSM version activated",
		"version", v,
		"previous_version", current,
		"raft_log_index", cmd.Index,
		"raft_log_term", cmd.Term,
	)
	return &raft_log.SetFSMVersionResponse{Version: uint32(v)}, nil
}

func (s *State) checkSupported(v Version) error {
	if v > s.supported {
		return fmt.Errorf("%w: the replicated state requires FSM version %d, but this binary supports up to %d", ErrUnsupportedVersion, v, s.supported)
	}
	return nil
}

func (s *State) setActive(v Version) {
	s.active.Store(uint32(v))
	s.metrics.active.Set(float64(v))
}

func Get(tx *bbolt.Tx) Version {
	b := tx.Bucket(bucketName)
	if b == nil {
		return Unversioned
	}
	v := b.Get(versionKey)
	if len(v) != 4 {
		return Unversioned
	}
	return Version(binary.BigEndian.Uint32(v))
}

func IsActive(tx *bbolt.Tx, v Version) bool { return Get(tx) >= v }

func put(tx *bbolt.Tx, v Version) error {
	b := tx.Bucket(bucketName)
	if b == nil {
		return bbolterrors.ErrBucketNotFound
	}
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(v))
	return b.Put(versionKey, buf)
}

type metrics struct {
	active    prometheus.Gauge
	supported prometheus.Gauge
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		active: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "fsm_version_active",
			Help: "FSM version active in the replicated state of this replica.",
		}),
		supported: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "fsm_version_supported",
			Help: "Highest FSM version supported by this binary.",
		}),
	}
	if reg != nil {
		m.active = util.RegisterOrGet(reg, m.active)
		m.supported = util.RegisterOrGet(reg, m.supported)
	}
	return m
}
