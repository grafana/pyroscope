module github.com/pyroscope-io/pyroscope

// we do use go1.17 for building pyroscope binaries
// but we don't use any go1.17 specific features
// so for maximum compatibility we only require go1.16 for this module
// I think it would make sense to upgrade to 1.17 when it's more mainstream
go 1.16

require (
	github.com/StackExchange/wmi v0.0.0-20210224194228-fe8f1750fd46 // indirect
	github.com/alecthomas/units v0.0.0-20210927113745-59d0afb8317a // indirect
	github.com/asaskevich/govalidator v0.0.0-20210307081110-f21760c49a8d
	github.com/aybabtme/rgbterm v0.0.0-20170906152045-cc83f3b3ce59
	github.com/blang/semver v3.5.1+incompatible
	github.com/cespare/xxhash/v2 v2.1.2
	github.com/cheggaaa/pb/v3 v3.0.5
	github.com/clarkduvall/hyperloglog v0.0.0-20171127014514-a0107a5d8004
	github.com/cosmtrek/air v1.12.2
	github.com/creack/pty v1.1.11 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/dgraph-io/badger/v2 v2.2007.2
	github.com/dgraph-io/ristretto v0.1.0 // indirect
	github.com/fatih/color v1.10.0
	github.com/felixge/fgprof v0.9.1
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-gormigrate/gormigrate/v2 v2.0.0
	github.com/go-ole/go-ole v1.2.5 // indirect
	github.com/golang-jwt/jwt v3.2.1+incompatible
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.2
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/go-jsonnet v0.17.0
	github.com/google/pprof v0.0.0-20211008130755-947d60d73cc0
	github.com/google/uuid v1.3.0
	github.com/gorilla/handlers v1.5.1
	github.com/gorilla/mux v1.8.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/golang-lru v0.5.1
	github.com/iancoleman/strcase v0.2.0
	github.com/imdario/mergo v0.3.12
	github.com/josephspurrier/goversioninfo v1.2.0
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/jsonnet-bundler/jsonnet-bundler v0.4.0
	github.com/kardianos/service v1.2.0
	github.com/kisielk/godepgraph v0.0.0-20190626013829-57a7e4a651a9
	github.com/klauspost/compress v1.13.6
	github.com/kyoh86/richgo v0.3.3
	github.com/kyoh86/xdg v1.2.0 // indirect
	github.com/mattn/go-runewidth v0.0.10 // indirect
	github.com/mattn/goreman v0.3.5
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mgechev/revive v1.0.3
	github.com/mitchellh/go-ps v1.0.0
	github.com/mitchellh/mapstructure v1.4.1
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f
	github.com/onsi/ginkgo/v2 v2.1.3
	github.com/onsi/gomega v1.17.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/common v0.32.1
	github.com/pyroscope-io/client v0.0.0-20211206204731-3fd0a4b8239c
	github.com/pyroscope-io/dotnetdiag v1.2.1
	github.com/pyroscope-io/goldga v0.4.2-0.20220218190441-817afcc3a7f1
	github.com/pyroscope-io/jfr-parser v0.1.0
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/rlmcpherson/s3gof3r v0.5.0
	github.com/shirou/gopsutil v3.21.4+incompatible
	github.com/sirupsen/logrus v1.8.1
	github.com/slok/go-http-metrics v0.9.0
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/tklauser/go-sysconf v0.3.6 // indirect
	github.com/twmb/murmur3 v1.1.5
	github.com/valyala/bytebufferpool v1.0.0
	github.com/wacul/ptr v1.0.0 // indirect
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519
	golang.org/x/net v0.0.0-20211020060615-d418f374d309
	golang.org/x/oauth2 v0.0.0-20211005180243-6b3c2da341f1
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20211020174200-9d6173849985
	golang.org/x/text v0.3.7
	golang.org/x/tools v0.1.7
	google.golang.org/protobuf v1.27.1
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v2 v2.4.0
	gorm.io/driver/mysql v1.1.2 // indirect
	gorm.io/driver/postgres v1.2.2 // indirect
	gorm.io/driver/sqlite v1.2.6
	gorm.io/driver/sqlserver v1.2.1 // indirect
	gorm.io/gorm v1.22.4
	honnef.co/go/tools v0.0.1-2020.1.6
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/klog/v2 v2.20.0 // indirect
)

replace github.com/mgechev/revive v1.0.3 => github.com/pyroscope-io/revive v1.0.6-0.20210330033039-4a71146f9dc1
