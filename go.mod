module github.com/pyroscope-io/pyroscope

go 1.16

require (
	github.com/StackExchange/wmi v0.0.0-20210224194228-fe8f1750fd46 // indirect
	github.com/aybabtme/rgbterm v0.0.0-20170906152045-cc83f3b3ce59
	github.com/blang/semver v3.5.1+incompatible
	github.com/cheggaaa/pb/v3 v3.0.5
	github.com/clarkduvall/hyperloglog v0.0.0-20171127014514-a0107a5d8004
	github.com/cosmtrek/air v1.12.2
	github.com/creack/pty v1.1.11 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/dgraph-io/badger/v2 v2.2007.2
	github.com/dgraph-io/ristretto v0.1.0 // indirect
	github.com/fatih/color v1.10.0
	github.com/felixge/fgprof v0.9.1
	github.com/go-ole/go-ole v1.2.5 // indirect
	github.com/golang-jwt/jwt v3.2.1+incompatible
	github.com/golang/protobuf v1.5.2
	github.com/golang/snappy v0.0.3 // indirect
	github.com/google/go-jsonnet v0.17.0
	github.com/google/pprof v0.0.0-20210226084205-cbba55b83ad5
	github.com/google/uuid v1.1.2
	github.com/iancoleman/strcase v0.2.0
	github.com/imdario/mergo v0.3.11 // indirect
	github.com/josephspurrier/goversioninfo v1.2.0
	github.com/jsonnet-bundler/jsonnet-bundler v0.4.0
	github.com/kardianos/service v1.2.0
	github.com/kisielk/godepgraph v0.0.0-20190626013829-57a7e4a651a9
	github.com/kr/pretty v0.2.0 // indirect
	github.com/kyoh86/richgo v0.3.3
	github.com/kyoh86/xdg v1.2.0 // indirect
	github.com/markbates/pkger v0.17.1
	github.com/mattn/go-runewidth v0.0.10 // indirect
	github.com/mattn/goreman v0.3.5
	github.com/mgechev/revive v1.0.3
	github.com/mitchellh/go-ps v1.0.0
	github.com/mitchellh/mapstructure v1.4.1
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/onsi/ginkgo v1.16.2
	github.com/onsi/gomega v1.12.0
	github.com/peterbourgon/ff/v3 v3.0.0
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/common v0.29.0
	github.com/pyroscope-io/dotnetdiag v1.2.1
	github.com/pyroscope-io/lfu-go v1.0.4
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/shirou/gopsutil v3.21.4+incompatible
	github.com/sirupsen/logrus v1.7.0
	github.com/slok/go-http-metrics v0.9.0
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.8.1
	github.com/tklauser/go-sysconf v0.3.6 // indirect
	github.com/twmb/murmur3 v1.1.5
	github.com/valyala/bytebufferpool v1.0.0
	github.com/wacul/ptr v1.0.0 // indirect
	golang.org/x/oauth2 v0.0.0-20210514164344-f6687ab2804c
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210616094352-59db8d763f22
	golang.org/x/tools v0.1.2
	google.golang.org/protobuf v1.26.0
	gopkg.in/yaml.v2 v2.4.0
	honnef.co/go/tools v0.0.1-2020.1.6
)

replace github.com/mgechev/revive v1.0.3 => github.com/pyroscope-io/revive v1.0.6-0.20210330033039-4a71146f9dc1
