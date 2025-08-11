package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/grafana/pyroscope/api/gen/proto/go/fuzz/distributor"
	"os"
	"path/filepath"
	"strings"

	//"github.com/grafana/pyroscope/api/gen/proto/go/fuzz/distributor"
	"github.com/grafana/pyroscope/pkg/pprof"
	//"github.com/grafana/pyroscope/pkg/validation"
)

var testdata = flag.String("testdata", "", "path to testdata directory")
var corpus = flag.String("corpus", "", "path to corpus dir")

func main() {
	flag.Parse()
	if *corpus == "" {
		panic("corpus path is required")
	}
	if *testdata == "" {
		panic("testdata path is required")
	}
	profiles := loadProfilesFromTestdata()
	_ = profiles
	for _, p := range profiles {

		m := &distributor.FuzzDistributor{
			Requests: []*distributor.Request{
				{
					Series: []*distributor.Sample{
						{
							Labels:  nil,
							Profile: p.Profile,
							ID:      "",
						},
					},
					Tenant: "",
				},
			},
			IngesterErrors: nil,
			Limits:         nil,
		}
		data, err := m.MarshalVT()
		if err != nil {
			panic(err)
		}
		checksum := sha256.Sum256(data)
		filename := hex.EncodeToString(checksum[:])
		err = os.WriteFile(filepath.Join(*corpus, filename), data, 0644)
		if err != nil {
			panic(err)
		}
	}

}

func loadProfilesFromTestdata() []*pprof.Profile {
	var profiles []*pprof.Profile
	files, err := os.ReadDir(*testdata)
	if err != nil {
		panic(err)
	}
	for _, file := range files {
		if file.IsDir() || strings.HasSuffix(file.Name(), ".txt") {
			continue
		}
		var data []byte
		var p *pprof.Profile
		pp := filepath.Join(*testdata, file.Name())
		data, err = os.ReadFile(pp)
		if err != nil {
			panic(err)
		}

		p, err = pprof.RawFromBytes(data)

		if err != nil {
			panic(fmt.Errorf("could not parse %s: %w", pp, err))
		}
		fmt.Printf("loaded pprof from %s\n", pp)
		profiles = append(profiles, p)
	}
	return profiles
}
