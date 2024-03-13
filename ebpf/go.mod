module github.com/grafana/pyroscope/ebpf

go 1.19

require (
	connectrpc.com/connect v1.14.0
	github.com/avvmoto/buf-readerat v0.0.0-20171115124131-a17c8cb89270
	github.com/cespare/xxhash/v2 v2.2.0
	github.com/cilium/ebpf v0.11.0
	github.com/go-kit/log v0.2.1
	github.com/google/pprof v0.0.0-20240117000934-35fc243c5815
	github.com/grafana/pyroscope/api v0.4.0
	github.com/hashicorp/golang-lru/v2 v2.0.5
	github.com/ianlancetaylor/demangle v0.0.0-20230524184225-eabc099b10ab
	github.com/klauspost/compress v1.17.4
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.17.0
	github.com/prometheus/common v0.45.0
	github.com/prometheus/prometheus v1.99.0
	github.com/samber/lo v1.38.1
	github.com/stretchr/testify v1.8.4
	golang.org/x/exp v0.0.0-20231206192017-f3f8817b8deb
	golang.org/x/sys v0.16.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/grafana/regexp v0.0.0-20221123153739-15dc172cd2db // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/matttproud/golang_protobuf_extensions/v2 v2.0.0 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/procfs v0.11.1 // indirect
	github.com/rogpeppe/go-internal v1.11.0 // indirect
	golang.org/x/net v0.20.0 // indirect
	golang.org/x/oauth2 v0.15.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240102182953-50ed04b92917 // indirect
	google.golang.org/grpc v1.61.0 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// x/sys: v0.14.0 removes definition of BPF_F_KPROBE_MULTI_RETURN in unix/zerrors_linux.go
// https://github.com/golang/go/issues/63969
replace golang.org/x/sys => golang.org/x/sys v0.13.0

replace github.com/prometheus/prometheus => github.com/prometheus/prometheus v0.48.1
