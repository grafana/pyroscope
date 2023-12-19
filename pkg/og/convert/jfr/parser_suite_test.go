package jfr

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/proto"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/og/storage/segment"
	"github.com/grafana/pyroscope/pkg/pprof"
)

func TestParser(t *testing.T) {
	RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Java JFR Parser suite")
}

func TestParseCompareExpectedData(t *testing.T) {
	testdata := []struct {
		jfr    string
		labels string
	}{
		{"testdata/cortex-dev-01__kafka-0__cpu__0.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu__1.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu__2.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu__3.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__0.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__1.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__2.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__3.jfr.gz", ""},
		{"testdata/cortex-dev-01__kafka-0__cpu_lock0_alloc0__0.jfr.gz", ""},
		{"testdata/dump1.jfr.gz", "testdata/dump1.labels.pb.gz"},
		{"testdata/dump2.jfr.gz", "testdata/dump2.labels.pb.gz"},
	}
	for _, td := range testdata {
		t.Run(td.jfr, func(t *testing.T) {
			jfr, err := bench.ReadGzipFile(td.jfr)
			require.NoError(t, err)
			//putter := &bench.MockPutter{Keep: true}
			k, err := segment.ParseKey("kafka.app")
			require.NoError(t, err)

			pi := &storage.PutInput{
				StartTime:  time.UnixMilli(1000),
				EndTime:    time.UnixMilli(2000),
				Key:        k,
				SpyName:    "java",
				SampleRate: 100,
			}
			var labels = new(LabelsSnapshot)
			if td.labels != "" {
				labelsBytes, err := bench.ReadGzipFile(td.labels)
				require.NoError(t, err)
				err = proto.Unmarshal(labelsBytes, labels)
				require.NoError(t, err)
			}
			req, err := ParseJFR(jfr, pi, labels)
			require.NoError(t, err)
			if len(req.Series) == 0 {
				t.Fatal(err)
			}
			//todo
			jsonFile := strings.TrimSuffix(td.jfr, ".jfr.gz") + ".json.gz"
			//err = putter.DumpJson(jsonFile)
			err = compareWithJson(t, req, jsonFile)
			require.NoError(t, err)

		})
	}
}

func compareWithJson(t *testing.T, req *distributormodel.PushRequest, file string) error {
	type flatProfileSeries struct {
		Labels  []*v1.LabelPair
		Profile *profilev1.Profile
	}

	var profiles []*flatProfileSeries
	for _, s := range req.Series {
		for _, sample := range s.Samples {
			sample.Profile.Normalize()
			iterateProfileSeries(sample.Profile.Profile.CloneVT(), s.Labels, func(p *profilev1.Profile, l phlaremodel.Labels) {
				profiles = append(profiles, &flatProfileSeries{
					Labels:  l,
					Profile: p,
				})
			})
		}
	}

	goldBS, err := bench.ReadGzipFile(file)
	if err != nil {
		return err
	}
	trees := make(map[string]string)
	err = json.Unmarshal(goldBS, &trees)
	if err != nil {
		return err
	}

	checkedSeries := make(map[string]struct{})
	for _, profile := range profiles {

		var keys []string
		scale := 1.0
		var valueIndices []int
		ls := phlaremodel.Labels(profile.Labels)
		metric := ls.Get(model.MetricNameLabel)
		service_name := ls.Get("service_name")
		typ := profile.Profile.StringTable[profile.Profile.SampleType[0].Type]
		event := ls.Get("jfr_event")
		keys = nil
		switch metric {
		case "process_cpu":
			keys = nil
			switch event {
			case "cpu", "wall":
				keys = []string{service_name + "." + "cpu"}
			case "itimer":
				keys = []string{service_name + "." + "itimer"}
			default:
				panic("unknown event: " + event) //todo wall
			}
			scale = 1.0 / (1e9 / 100.0)
			valueIndices = []int{0}
		case "memory":
			if typ == "live" {
				keys = []string{service_name + "." + "live"}
				valueIndices = []int{0}
			} else {
				if strings.Contains(typ, "alloc_in_new_tlab_objects") {
					keys = []string{
						service_name + "." + "alloc_in_new_tlab_objects",
						service_name + "." + "alloc_in_new_tlab_bytes",
					}
				} else {
					keys = []string{
						service_name + "." + "alloc_outside_tlab_objects",
						service_name + "." + "alloc_outside_tlab_bytes",
					}
				}
				valueIndices = []int{0, 1}
			}
		case "mutex":
			keys = []string{
				service_name + "." + "lock_count",
				service_name + "." + "lock_duration",
			}
			valueIndices = []int{0, 1}
		case "block":
			keys = []string{
				service_name + "." + "thread_park_count",
				service_name + "." + "thread_park_duration",
			}
			valueIndices = []int{0, 1}
		case "wall":
			keys = []string{service_name + "." + "wall"}
			valueIndices = []int{0}
			scale = 1.0 / (1e9 / 100.0)
		default:
			panic("unknown metric: " + metric + " " + service_name)
		}
		if len(keys) == 0 {
			return fmt.Errorf("no keys found for %s %s %s", metric, typ, service_name)
		}
		for i := range keys {
			key := keys[i]
			parseKey, err := segment.ParseKey(key)
			if err != nil {
				return err
			}
			for _, label := range profile.Labels {
				if strings.HasPrefix(label.Name, "__") || label.Name == phlaremodel.LabelNameServiceName || label.Name == "jfr_event" || label.Name == phlaremodel.LabelNamePyroscopeSpy {
					continue
				}
				parseKey.Add(label.Name, label.Value)
			}

			// We used to duplicate samples with profile ID.
			// Now we don't do it anymore, but have it in fixtures.
			if _, ok := parseKey.ProfileID(); ok {
				k := parseKey.Clone()
				k.Add("profile_id", "")
				delete(trees, k.Normalized())
			}

			key = parseKey.Normalized()
			expectedTree := trees[key]
			if expectedTree == "" {
				return fmt.Errorf("no tree found for %s", key)
			}
			checkedSeries[key] = struct{}{}
			expectedLines := strings.Split(expectedTree, "\n")
			slices.Sort(expectedLines)
			expectedTree = strings.Join(expectedLines, "\n")
			expectedTree = strings.Trim(expectedTree, "\n")

			pp := pprof.Profile{Profile: profile.Profile}
			pp.Normalize()

			collapseLines := bench.StackCollapseProto(pp.Profile, valueIndices[i], scale)
			slices.Sort(collapseLines)
			collapsedStr := strings.Join(collapseLines, "\n")
			collapsedStr = strings.Trim(collapsedStr, "\n")

			if expectedTree != collapsedStr {
				os.WriteFile(file+"_"+metric+"_"+typ+"_expected.txt", []byte(expectedTree), 0644)
				os.WriteFile(file+"_"+metric+"_"+typ+"_actual.txt", []byte(collapsedStr), 0644)
				return fmt.Errorf("expected tree:\n%s\ngot:\n%s", expectedTree, collapsedStr)
			}
			fmt.Printf("ok %s %d\n", key, len(collapsedStr))
		}
	}
	for k, v := range trees {
		_ = v
		if _, ok := checkedSeries[k]; !ok {
			assert.Failf(t, "no profile found for ", "key=%s", k)
		}
	}
	for k, v := range checkedSeries {
		_ = v
		if _, ok := trees[k]; !ok {
			assert.Failf(t, "no tree found for ", "key=%s", k)
		}
	}
	return nil
}

func BenchmarkParser(b *testing.B) {
	tests := []string{
		"testdata/cortex-dev-01__kafka-0__cpu__0.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu__1.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu__2.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu__3.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__0.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__1.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__2.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu_lock_alloc__3.jfr.gz",
		"testdata/cortex-dev-01__kafka-0__cpu_lock0_alloc0__0.jfr.gz",
	}

	for _, testdata := range tests {
		f := testdata
		b.Run(testdata, func(b *testing.B) {
			jfr, err := bench.ReadGzipFile(f)
			require.NoError(b, err)
			k, err := segment.ParseKey("kafka.app")
			require.NoError(b, err)
			pi := &storage.PutInput{
				StartTime:  time.UnixMilli(1000),
				EndTime:    time.UnixMilli(2000),
				Key:        k,
				SpyName:    "java",
				SampleRate: 100,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				profiles, err := ParseJFR(jfr, pi, nil)
				if err != nil {
					b.Fatal(err)
				}
				if len(profiles.Series) == 0 {
					b.Fatal()
				}
			}
		})
	}
}

func iterateProfileSeries(p *profilev1.Profile, seriesLabels phlaremodel.Labels, fn func(*profilev1.Profile, phlaremodel.Labels)) {
	for _, x := range p.Sample {
		sort.Sort(pprof.LabelsByKeyValue(x.Label))
	}
	sort.Sort(pprof.SamplesByLabels(p.Sample))
	groups := pprof.GroupSamplesByLabels(p)
	e := pprof.NewSampleExporter(p)
	for _, g := range groups {
		ls := mergeSeriesAndSampleLabels(p, seriesLabels, g.Labels)
		ps := e.ExportSamples(new(profilev1.Profile), g.Samples)
		fn(ps, ls)
	}
}

func mergeSeriesAndSampleLabels(p *profilev1.Profile, sl []*v1.LabelPair, pl []*profilev1.Label) []*v1.LabelPair {
	m := phlaremodel.Labels(sl).Clone()
	for _, l := range pl {
		m = append(m, &v1.LabelPair{
			Name:  p.StringTable[l.Key],
			Value: p.StringTable[l.Str],
		})
	}
	sort.Stable(m)
	return m.Unique()
}
