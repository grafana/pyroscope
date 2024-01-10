package version

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/kv/codec"
	"github.com/grafana/dskit/kv/memberlist"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	versionv1 "github.com/grafana/pyroscope/api/gen/proto/go/version/v1"
	"github.com/grafana/pyroscope/pkg/util"
)

// currentQuerierVersion is the current version of the querier API.
// It is used to check if the query path is compatible a new change.
// Increase this number when a new API change is introduced.
const currentQuerierVersion = uint64(1)

var (
	heartbeatInterval                      = 15 * time.Second
	instanceTimeout                        = 1 * time.Minute
	_                 memberlist.Mergeable = (*Versions)(nil)
	_                 services.Service     = (*Service)(nil)
	now                                    = time.Now
)

func GetCodec() codec.Codec {
	return codec.NewProtoCodec("versions", newVersions)
}

func newVersions() proto.Message {
	return &Versions{
		Versions: &versionv1.Versions{
			Instances: make(map[string]*versionv1.InstanceVersion),
		},
	}
}

type Versions struct {
	*versionv1.Versions
}

// Implements proto.Unmarshaler.
func (v *Versions) Unmarshal(in []byte) error {
	return v.UnmarshalVT(in)
}

// Implements proto.Marshaler.
func (v *Versions) Marshal() ([]byte, error) {
	return v.MarshalVT()
}

// Merge merges two versions. This is used when CASing or merging versions from other nodes.
// v is the local version and should be mutated to include the changes from incoming.
// The function should only returned changed instances.
func (v *Versions) Merge(incoming memberlist.Mergeable, localCAS bool) (memberlist.Mergeable, error) {
	if incoming == nil {
		return nil, nil
	}
	other, ok := incoming.(*Versions)
	if !ok {
		return nil, fmt.Errorf("expected *Versions, got %T", incoming)
	}
	if other == nil {
		return nil, nil
	}
	if v == nil {
		v = &Versions{
			Versions: other.CloneVT(),
		}
		return other, nil
	}
	if other.EqualVT(v.Versions) {
		return nil, nil
	}
	if v.Instances == nil {
		v.Instances = make(map[string]*versionv1.InstanceVersion)
	}
	var updated []string

	// Copy over all the instances with newer timestamps.
	for k, new := range other.Instances {
		current, ok := v.Instances[k]
		if !ok || new.Timestamp > current.Timestamp {
			v.Instances[k] = new.CloneVT()
			updated = append(updated, k)
		} else if new.Timestamp == current.Timestamp && !current.Left && new.Left {
			v.Instances[k] = new.CloneVT()
			updated = append(updated, k)
		}

	}

	if localCAS {
		// Mark left all the instances that are not in the other.
		for k, current := range v.Instances {
			if _, ok := other.Instances[k]; !ok && !current.Left {
				current.Left = true
				current.Timestamp = now().UnixNano()
				updated = append(updated, k)
			}
		}
	}
	// No updated members, no need to broadcast.
	if len(updated) == 0 {
		return nil, nil
	}
	// Return the changes to broadcast.
	changes := newVersions().(*Versions)
	for _, k := range updated {
		changes.Instances[k] = v.Instances[k].CloneVT()
	}
	return changes, nil
}

// MergeContent describes content of this Mergeable.
// Versions simply returns list of component that it includes.
func (d *Versions) MergeContent() []string {
	result := []string(nil)
	for k := range d.Instances {
		result = append(result, k)
	}
	return result
}

// RemoveTombstones is not required for version keys.
func (v *Versions) RemoveTombstones(limit time.Time) (total, removed int) {
	for n, inst := range v.Instances {
		if inst.Left {
			if limit.IsZero() || time.Unix(0, inst.Timestamp).Before(limit) {
				// remove it
				delete(v.Instances, n)
				removed++
			} else {
				total++
			}
		}
	}
	return
}

// Implements memberlist.Mergeable.
func (v *Versions) Clone() memberlist.Mergeable {
	return &Versions{
		Versions: v.CloneVT(),
	}
}

type Service struct {
	*services.BasicService
	store    kv.Client
	cfg      util.CommonRingConfig
	logger   log.Logger
	cancel   context.CancelFunc
	ctx      context.Context
	wg       sync.WaitGroup
	addr, id string

	version uint64
}

// New creates a new version service.
func New(cfg util.CommonRingConfig, logger log.Logger, reg prometheus.Registerer) (*Service, error) {
	client, err := kv.NewClient(cfg.KVStore, GetCodec(), kv.RegistererWithKVName(reg, "versions"), logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize versions' KV store")
	}

	instanceAddr, err := ring.GetInstanceAddr(cfg.InstanceAddr, cfg.InstanceInterfaceNames, logger, false)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	instancePort := ring.GetInstancePort(cfg.InstancePort, cfg.ListenPort)
	svc := &Service{
		store:   client,
		id:      cfg.InstanceID,
		addr:    fmt.Sprintf("%s:%d", instanceAddr, instancePort),
		cfg:     cfg,
		logger:  log.With(logger, "component", "versions"),
		cancel:  cancel,
		ctx:     ctx,
		version: currentQuerierVersion,
	}
	// The service is simple only has a running function.
	// Stopping is manual to ensure we stop as part of the shutdown process.
	svc.BasicService = services.NewBasicService(
		func(_ context.Context) error { return nil },
		svc.running,
		func(_ error) error { return nil },
	)
	return svc, nil
}

func (svc *Service) running(ctx context.Context) error {
	go svc.loop()
	<-ctx.Done()
	return nil
}

func (svc *Service) loop() {
	svc.wg.Add(1)
	ticker := time.NewTicker(heartbeatInterval)
	defer func() {
		ticker.Stop()
		svc.wg.Done()
	}()

	for {
		select {
		case <-ticker.C:
			if err := svc.heartbeat(svc.ctx); err != nil {
				level.Error(svc.logger).Log("msg", "failed to heartbeat", "err", err)
			}

		case <-svc.ctx.Done():
			level.Info(svc.logger).Log("msg", "versions is shutting down")
			return
		}
	}
}

func (svc *Service) heartbeat(ctx context.Context) error {
	return svc.store.CAS(ctx, "versions", func(in interface{}) (out interface{}, retry bool, err error) {
		var versions *versionv1.Versions
		if in == nil {
			versions = newVersions().(*Versions).Versions
		} else {
			versions = in.(*Versions).Versions
		}
		current, ok := versions.Instances[svc.id]
		if !ok {
			current = &versionv1.InstanceVersion{}
			versions.Instances[svc.id] = current
		}
		current.Addr = svc.addr
		current.ID = svc.id
		current.Timestamp = now().UnixNano()
		current.QuerierAPI = svc.version
		// Now prune old instances.
		for id, instance := range versions.Instances {
			lastHeartbeat := time.Unix(0, instance.GetTimestamp())
			if time.Since(lastHeartbeat) > instanceTimeout {
				level.Warn(svc.logger).Log("msg", "auto-forgetting instance from the versions because it is unhealthy for a long time", "instance", id, "last_heartbeat", lastHeartbeat.String())
				delete(versions.Instances, id)
			}
		}
		return &Versions{
			Versions: versions,
		}, true, nil
	})
}

func (svc *Service) Version(ctx context.Context, req *connect.Request[versionv1.VersionRequest]) (*connect.Response[versionv1.VersionResponse], error) {
	value, err := svc.store.Get(ctx, "versions")
	if err != nil {
		return nil, err
	}
	versions, ok := value.(*Versions)
	if !ok {
		// we don't have any versions yet.
		return connect.NewResponse(&versionv1.VersionResponse{}), nil
	}
	// collect the minimum querier version.
	minQuerierVersion := uint64(math.MaxUint64)
	for _, instance := range versions.Instances {
		if instance.QuerierAPI < minQuerierVersion {
			minQuerierVersion = instance.QuerierAPI
		}
	}
	return connect.NewResponse(&versionv1.VersionResponse{
		QuerierAPI: minQuerierVersion,
	}), nil
}

// Shutdown stops version reports.
// This should only be called when the service is fully shutting down.
func (svc *Service) Shutdown() {
	svc.cancel()
	svc.wg.Wait()
}
