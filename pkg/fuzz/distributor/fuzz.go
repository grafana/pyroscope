//go:build libfuzzer

package main

import (
	"connectrpc.com/connect"
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/ring/client"
	fuzzd "github.com/grafana/pyroscope/api/gen/proto/go/fuzz/distributor"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/pkg/distributor"
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	"github.com/grafana/pyroscope/pkg/fuzz/common"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/testhelper"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/validation"
	model2 "github.com/prometheus/common/model"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"unsafe"

	mutator "github.com/yandex-cloud/go-protobuf-mutator"
)

// #include <stdint.h>
import "C"

//export LLVMFuzzerInitialize
func LLVMFuzzerInitialize(argc *C.int, argv ***C.char) C.int {

	//todo fuzz limits as well
	return 0
}

//export LLVMFuzzerCustomMutator
func LLVMFuzzerCustomMutator(data *C.char, size C.size_t, maxSize C.size_t, seed C.uint) C.size_t {
	gdata := unsafe.Slice((*byte)(unsafe.Pointer(data)), size)
	message := new(fuzzd.FuzzDistributor)
	if err := message.UnmarshalVT(gdata); err != nil {
		return 0
	}
	mutator := mutator.New(int64(seed), int(maxSize-size))
	if err := mutator.MutateProto(message); err != nil {
		fmt.Printf("Failed to mutate message: %+v", err)
		return 0
	}
	gdata = unsafe.Slice((*byte)(unsafe.Pointer(data)), maxSize)
	if message.SizeVT() > len(gdata) {
		return 0
	}

	if sz, err := message.MarshalToVT(gdata); err != nil {
		panic(err)
	} else {
		return C.size_t(sz)
	}
}

//export LLVMFuzzerTestOneInput
func LLVMFuzzerTestOneInput(data *C.char, size C.size_t) C.int {
	gdata := unsafe.Slice((*byte)(unsafe.Pointer(data)), size)
	message := new(fuzzd.FuzzDistributor)
	if err := message.UnmarshalVT(gdata); err != nil {
		return 0
	}

	ing := &fakeIngester{errors: message.IngesterErrors}

	limits, err := common.ConvertValidationLimits(message.Limits)
	if err != nil || message.Limits == nil {
		limits = validation.Limits{}
		limits.RegisterFlags(flag.NewFlagSet("", flag.ContinueOnError))
	}
	overrides, _ := validation.NewOverrides(limits, nil)

	d, err := distributor.New(distributor.Config{
		DistributorRing: ringConfig,
	}, testhelper.NewMockRing([]ring.InstanceDesc{
		{Addr: "foo"},
	}, 3), &poolFactory{func(addr string) (client.PoolClient, error) {
		return ing, nil
	}}, overrides, nil, log.NewNopLogger(), nil)
	if err != nil {
		return 0
	}
	for _, r := range message.Requests {
		rr := &distributormodel.PushRequest{
			Series:                        nil,
			ReceivedCompressedProfileSize: int(r.ReceivedCompressedProfileSize),
			RawProfileType:                distributormodel.RawProfileType(r.RawProfileType),
		}
		for _, series := range r.Series {
			labels := model.NewLabelsBuilder(series.Labels)
			labels.Set(model2.MetricNameLabel, series.MetricName)
			if labels.Get(model2.MetricNameLabel) == "" {
				labels.Set(model2.MetricNameLabel, "hui")
			}
			ss := &distributormodel.ProfileSeries{
				Labels:     labels.Labels(),
				Profile:    pprof.RawFromProto(series.Profile),
				RawProfile: nil,
				ID:         "",
				TenantID:   r.Tenant,
			}
			rr.Series = append(rr.Series, ss)
		}
		ctx := tenant.InjectTenantID(context.Background(), r.Tenant)
		_ = d.PushBatch(ctx, rr)
	}
	// todo stop and wait for all distriubutor background activities (aggregation?)

	return 0
}

// todo this was copypasted from distributor test. reuse the code instead.
type poolFactory struct {
	f func(addr string) (client.PoolClient, error)
}

func (pf *poolFactory) FromInstance(inst ring.InstanceDesc) (client.PoolClient, error) {
	return pf.f(inst.Addr)
}

type fakeIngester struct {
	testhelper.FakePoolClient
	res    pushv1.PushResponse
	errors []bool // should this be typed?
	cnt    atomic.Int64
}

func (i *fakeIngester) List(ctx context.Context, in *grpc_health_v1.HealthListRequest, opts ...grpc.CallOption) (*grpc_health_v1.HealthListResponse, error) {
	return nil, errors.New("not implemented")
}

func (i *fakeIngester) Push(_ context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	no := i.cnt.Inc()
	err := false
	if int(no) >= len(i.errors) {
		err = false
	} else {
		err = i.errors[no]
	}
	if err {
		return connect.NewResponse[pushv1.PushResponse](&i.res), fmt.Errorf("error pushing %v %d", req, no)
	}
	return connect.NewResponse[pushv1.PushResponse](&i.res), nil
}

var ringConfig = util.CommonRingConfig{
	KVStore:      kv.Config{Store: "inmemory"},
	InstanceID:   "foo",
	InstancePort: 8080,
	InstanceAddr: "127.0.0.1",
	ListenPort:   8080,
}

func main() {

}

func init() {
	go func() {
		util.RecoverPanic = func(f func() error) func() error {
			return func() error {
				return f()
			}
		}
		util.Recover = func(f func()) {
			f()
		}
		DumpCoverage(context.Background())
	}()
}
