module github.com/grafana/pyroscope

go 1.22.7

require (
	connectrpc.com/connect v1.16.2
	connectrpc.com/grpchealth v1.3.0
	github.com/PuerkitoBio/goquery v1.8.1
	github.com/aybabtme/rgbterm v0.0.0-20170906152045-cc83f3b3ce59
	github.com/briandowns/spinner v1.23.0
	github.com/cespare/xxhash/v2 v2.3.0
	github.com/colega/zeropool v0.0.0-20230505084239-6fb4a4f75381
	github.com/dennwc/varint v1.0.0
	github.com/dgryski/go-groupvarint v0.0.0-20230630160417-2bfb7969fb3c
	github.com/dolthub/swiss v0.2.1
	github.com/drone/envsubst v1.0.3
	github.com/dustin/go-humanize v1.0.1
	github.com/fatih/color v1.15.0
	github.com/felixge/fgprof v0.9.4-0.20221116204635-ececf7638e93
	github.com/felixge/httpsnoop v1.0.4
	github.com/fsnotify/fsnotify v1.7.0
	github.com/go-kit/log v0.2.1
	github.com/gogo/protobuf v1.3.2
	github.com/gogo/status v1.1.1
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da
	github.com/google/go-cmp v0.6.0
	github.com/google/go-github/v58 v58.0.1-0.20240111193443-e9f52699f5e5
	github.com/google/pprof v0.0.0-20240727154555-813a5fbdbec8
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.0
	github.com/grafana/alloy/syntax v0.1.0
	github.com/grafana/dskit v0.0.0-20231221015914-de83901bf4d6
	github.com/grafana/jfr-parser/pprof v0.0.0-20240228024232-8abcb81c304c
	github.com/grafana/pyroscope-go v1.2.0
	github.com/grafana/pyroscope-go/godeltaprof v0.1.8
	github.com/grafana/pyroscope-go/x/k6 v0.0.0-20241003203156-a917cea171d3
	github.com/grafana/pyroscope/api v0.4.0
	github.com/grafana/regexp v0.0.0-20221123153739-15dc172cd2db
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.19.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/hashicorp/raft v1.7.0
	github.com/hashicorp/raft-wal v0.4.1
	github.com/iancoleman/strcase v0.3.0
	github.com/json-iterator/go v1.1.12
	github.com/k0kubun/pp/v3 v3.2.0
	github.com/klauspost/compress v1.17.10
	github.com/kubescape/go-git-url v0.0.27
	github.com/mattn/go-isatty v0.0.19
	github.com/minio/minio-go/v7 v7.0.72
	github.com/mitchellh/go-wordwrap v1.0.1
	github.com/oauth2-proxy/oauth2-proxy/v7 v7.5.1
	github.com/oklog/ulid v1.3.1
	github.com/olekukonko/tablewriter v0.0.5
	github.com/onsi/ginkgo/v2 v2.13.0
	github.com/onsi/gomega v1.29.0
	github.com/opentracing-contrib/go-grpc v0.0.0-20210225150812-73cb765af46e
	github.com/opentracing/opentracing-go v1.2.1-0.20220228012449-10b1cf09e00b
	github.com/parquet-go/parquet-go v0.23.0
	github.com/pkg/errors v0.9.1
	github.com/planetscale/vtprotobuf v0.6.0
	github.com/prometheus/client_golang v1.19.1
	github.com/prometheus/client_model v0.6.1
	github.com/prometheus/common v0.55.0
	github.com/prometheus/prometheus v0.51.2
	github.com/samber/lo v1.38.1
	github.com/simonswine/tempopb v0.2.0
	github.com/sirupsen/logrus v1.9.3
	github.com/sony/gobreaker/v2 v2.0.0
	github.com/spf13/afero v1.11.0
	github.com/stretchr/testify v1.9.0
	github.com/thanos-io/objstore v0.0.0-20240722162417-19b0c0f0ffd8
	github.com/uber/jaeger-client-go v2.30.0+incompatible
	github.com/valyala/bytebufferpool v1.0.0
	github.com/xlab/treeprint v1.2.0
	go.etcd.io/bbolt v1.3.10
	go.opentelemetry.io/otel v1.30.0
	go.opentelemetry.io/proto/otlp v1.1.0
	go.uber.org/atomic v1.11.0
	go.uber.org/automaxprocs v1.5.3
	go.uber.org/goleak v1.3.0
	golang.org/x/exp v0.0.0-20240119083558-1b970713d09a
	golang.org/x/mod v0.17.0
	golang.org/x/net v0.26.0
	golang.org/x/oauth2 v0.21.0
	golang.org/x/sync v0.8.0
	golang.org/x/sys v0.25.0
	golang.org/x/text v0.19.0
	golang.org/x/time v0.5.0
	gonum.org/v1/plot v0.14.0
	google.golang.org/genproto/googleapis/api v0.0.0-20240314234333-6e1732d8331c
	google.golang.org/grpc v1.62.1
	google.golang.org/protobuf v1.34.2
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v3 v3.0.1
	sigs.k8s.io/yaml v1.3.0
)

require (
	cloud.google.com/go v0.112.1 // indirect
	cloud.google.com/go/compute/metadata v0.3.0 // indirect
	cloud.google.com/go/iam v1.1.7 // indirect
	cloud.google.com/go/storage v1.40.0 // indirect
	git.sr.ht/~sbinet/gg v0.5.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.11.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.6.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.8.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.3.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.2.2 // indirect
	github.com/HdrHistogram/hdrhistogram-go v1.1.2 // indirect
	github.com/ajstarks/svgo v0.0.0-20211024235047-1546f124cd8b // indirect
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20231202071711-9a357b53e9c9 // indirect
	github.com/aliyun/aliyun-oss-go-sdk v2.2.6+incompatible // indirect
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/andybalholm/cascadia v1.3.1 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/aws/aws-sdk-go v1.50.32 // indirect
	github.com/aws/aws-sdk-go-v2 v1.18.1 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.18.27 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.13.26 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.13.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.34 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.28 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.35 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.28 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.12.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.14.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.19.2 // indirect
	github.com/aws/smithy-go v1.13.5 // indirect
	github.com/bboreham/go-loser v0.0.0-20230920113527-fcc2c21820a3 // indirect
	github.com/benbjohnson/immutable v0.4.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/campoy/embedmd v1.0.0 // indirect
	github.com/chainguard-dev/git-urls v1.0.2 // indirect
	github.com/clbanning/mxj v1.8.4 // indirect
	github.com/coreos/etcd v3.3.27+incompatible // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/coreos/pkg v0.0.0-20220810130054-c7d1c02cb6cf // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dolthub/maphash v0.1.0 // indirect
	github.com/edsrzf/mmap-go v1.1.0 // indirect
	github.com/efficientgo/core v1.0.0-rc.2 // indirect
	github.com/efficientgo/e2e v0.14.1-0.20230710114240-c316eb95ae5b // indirect
	github.com/facette/natsort v0.0.0-20181210072756-2cd4dd1e2dcb // indirect
	github.com/go-fonts/liberation v0.3.3 // indirect
	github.com/go-latex/latex v0.0.0-20240709081214-31cef3c7570e // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/analysis v0.22.2 // indirect
	github.com/go-openapi/errors v0.21.1 // indirect
	github.com/go-openapi/jsonpointer v0.20.2 // indirect
	github.com/go-openapi/jsonreference v0.20.4 // indirect
	github.com/go-openapi/loads v0.21.5 // indirect
	github.com/go-openapi/spec v0.20.14 // indirect
	github.com/go-openapi/strfmt v0.22.2 // indirect
	github.com/go-openapi/swag v0.22.9 // indirect
	github.com/go-openapi/validate v0.23.0 // indirect
	github.com/go-pdf/fpdf v0.9.0 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/gogo/googleapis v1.4.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.1 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	github.com/googleapis/gax-go/v2 v2.12.3 // indirect
	github.com/grafana/jfr-parser v0.8.1-0.20240228024232-8abcb81c304c // indirect
	github.com/hashicorp/consul/api v1.28.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.6.2 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-msgpack v1.1.5 // indirect
	github.com/hashicorp/go-msgpack/v2 v2.1.1 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.6 // indirect
	github.com/hashicorp/go-version v1.6.0 // indirect
	github.com/hashicorp/golang-lru v0.6.0 // indirect
	github.com/hashicorp/memberlist v0.5.0 // indirect
	github.com/hashicorp/serf v0.10.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.8 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/miekg/dns v1.1.58 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mozillazg/go-httpheader v0.3.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/ncw/swift v1.0.53 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc3 // indirect
	github.com/opentracing-contrib/go-stdlib v1.0.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/alertmanager v0.27.0 // indirect
	github.com/prometheus/common/sigv4 v0.1.0 // indirect
	github.com/prometheus/exporter-toolkit v0.11.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rs/xid v1.5.0 // indirect
	github.com/sean-/seed v0.0.0-20170313163322-e2103e2c3529 // indirect
	github.com/segmentio/encoding v0.4.0 // indirect
	github.com/sercand/kuberesolver/v5 v5.1.1 // indirect
	github.com/soheilhy/cmux v0.1.5 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tencentyun/cos-go-sdk-v5 v0.7.40 // indirect
	github.com/uber/jaeger-lib v2.4.1+incompatible // indirect
	go.etcd.io/etcd/api/v3 v3.5.7 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.7 // indirect
	go.etcd.io/etcd/client/v3 v3.5.7 // indirect
	go.mongodb.org/mongo-driver v1.14.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.3.0 // indirect
	go.opentelemetry.io/collector/pdata v1.3.0 // indirect
	go.opentelemetry.io/collector/semconv v0.96.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.49.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.49.0 // indirect
	go.opentelemetry.io/otel/metric v1.30.0 // indirect
	go.opentelemetry.io/otel/trace v1.30.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.24.0 // indirect
	golang.org/x/crypto v0.24.0 // indirect
	golang.org/x/image v0.21.0 // indirect
	golang.org/x/term v0.21.0 // indirect
	golang.org/x/tools v0.21.1-0.20240508182429-e35e4ccd0d2d // indirect
	google.golang.org/api v0.172.0 // indirect
	google.golang.org/genproto v0.0.0-20240213162025-012b6fc9bca9 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240318140521-94a12d6c2237 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/apimachinery v0.29.2 // indirect
	k8s.io/client-go v0.29.2 // indirect
	k8s.io/klog/v2 v2.120.1 // indirect
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b // indirect
)

replace (
	github.com/grafana/pyroscope/api => ./api

	// Replace memberlist with our fork which includes some fixes that haven't been
	// merged upstream yet.
	github.com/hashicorp/memberlist => github.com/grafana/memberlist v0.3.1-0.20220708130638-bd88e10a3d91

	// gopkg.in/yaml.v3
	// + https://github.com/go-yaml/yaml/pull/691
	// + https://github.com/go-yaml/yaml/pull/876
	gopkg.in/yaml.v3 => github.com/colega/go-yaml-yaml v0.0.0-20220720105220-255a8d16d094
)
