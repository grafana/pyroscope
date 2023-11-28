package version

import (
	"context"
	"fmt"
	"time"

	"github.com/bufbuild/connect-go"
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

func MergeVersionResponses(responses ...*connect.Response[versionv1.VersionResponse]) *connect.Response[versionv1.VersionResponse] {
	return nil
}

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
	if proto.Equal(other.Versions, v.Versions) {
		return nil, nil
	}
	if v == nil {
		return other, nil
	}
	out := v.Clone().(*Versions)
	if out.Instances == nil {
		out.Instances = make(map[string]*versionv1.InstanceVersion)
	}
	change := false
	// todo should properly merge missing keys from other.
	// test this
	// copy over all the instances with newer timestamps.
	for k, v := range v.Instances {
		other, ok := other.Instances[k]
		if !ok {
			out.Instances[k] = v
			change = true
			continue
		}
		if proto.Equal(v, other) {
			continue
		}
		if other.Timestamp > v.Timestamp {
			out.Instances[k] = v
			change = true
		}
	}
	if !change {
		return nil, nil
	}
	return out, nil
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
func (c *Versions) RemoveTombstones(limit time.Time) (total, removed int) {
	return 0, 0
}

func (c *Versions) Clone() memberlist.Mergeable {
	return &Versions{
		Versions: c.CloneVT(),
	}
}

var (
	_ memberlist.Mergeable = (*Versions)(nil)
	_ services.Service     = (*Service)(nil)
)

type Service struct {
	*services.BasicService
	store    kv.Client
	cfg      util.CommonRingConfig
	logger   log.Logger
	cancel   context.CancelFunc
	ctx      context.Context
	addr, id string
}

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
		store:  client,
		id:     cfg.InstanceID,
		addr:   fmt.Sprintf("%s:%d", instanceAddr, instancePort),
		cfg:    cfg,
		logger: log.With(logger, "component", "versions"),
		cancel: cancel,
		ctx:    ctx,
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
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

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
		current.Timestamp = time.Now().Unix()
		current.QuerierAPI = currentQuerierVersion
		// Now prune old instances.
		for id, instance := range versions.Instances {
			lastHeartbeat := time.Unix(instance.GetTimestamp(), 0)

			if time.Since(lastHeartbeat) > 1*time.Minute {
				level.Warn(svc.logger).Log("msg", "auto-forgetting instance from the ring because it is unhealthy for a long time", "instance", id, "last_heartbeat", lastHeartbeat.String())
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
	minQuerierVersion := uint64(0)
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
}
