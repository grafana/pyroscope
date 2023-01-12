package bench

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof/streaming"
	"github.com/pyroscope-io/pyroscope/pkg/util/form"
	"io"
	"io/fs"
	"mime/multipart"
	"strings"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"io/ioutil"
	"log"
	"net/http"

	"testing"
	"time"
)

type testcase struct {
	profile, prev []byte
	config        map[string]*tree.SampleTypeConfig
	fname         string
	spyname       string
}

func readRawCorpus(dir string) []*testcase {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}
	var res []*testcase
	for _, file := range files {
		fname := dir + "/" + file.Name()
		bs, err := ioutil.ReadFile(fname)
		if err != nil {
			panic(err)
		}
		res = append(res, &testcase{profile: bs, config: nil, fname: fname, spyname: "rbspy"})
	}
	return res
}
func readCorpus(dir string) []*testcase {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}
	var res []*testcase
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".txt") {
			res = append(res, readCorpusItem(dir, file))
		}
	}
	return res
}
func readCorpusItemI(dir string, i int) *testcase {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}

	return readCorpusItem(dir, files[i])

}

func readCorpusItem(dir string, file fs.FileInfo) *testcase {
	fname := dir + "/" + file.Name()
	return readCorpusItemFile(fname)
}

func readCorpusItemFile(fname string) *testcase {
	bs, err := ioutil.ReadFile(fname)
	if err != nil {
		panic(err)
	}
	r, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(bs)))
	if err != nil {
		panic(err)
	}
	_ = r
	//fmt.Printf("%s %d\n", file.Name(), len(bs))
	contentType := r.Header.Get("Content-Type")

	rawData, _ := ioutil.ReadAll(r.Body)

	decompres := func(b []byte) []byte {
		if len(b) < 2 {
			return b
		}
		if b[0] == 0x1f && b[1] == 0x8b {
			gzipr, err := gzip.NewReader(bytes.NewReader(b))
			if err != nil {
				panic(err)
			}
			defer gzipr.Close()
			var buf bytes.Buffer

			//defer bytebufferpool.Put(buf)

			if _, err = io.Copy(&buf, gzipr); err != nil {
				panic(err)
			}
			return buf.Bytes()
		}
		return b
	}

	if contentType == "binary/octet-stream" {
		return &testcase{
			profile: decompres(rawData),
			config:  tree.DefaultSampleTypeMapping,
			fname:   fname,
		}
	}
	boundary, err := form.ParseBoundary(contentType)
	if err != nil {
		panic(err)
	}

	f, err := multipart.NewReader(bytes.NewReader(rawData), boundary).ReadForm(32 << 20)
	if err != nil {
		panic(err)
	}
	const (
		formFieldProfile          = "profile"
		formFieldPreviousProfile  = "prev_profile"
		formFieldSampleTypeConfig = "sample_type_config"
	)

	Profile, err := form.ReadField(f, formFieldProfile)
	if err != nil {
		panic(err)
	}
	PreviousProfile, err := form.ReadField(f, formFieldPreviousProfile)
	if err != nil {
		panic(err)
	}

	stBS, err := form.ReadField(f, formFieldSampleTypeConfig)
	if err != nil {
		panic(err)
	}
	var config map[string]*tree.SampleTypeConfig
	if stBS != nil {
		if err = json.Unmarshal(stBS, &config); err != nil {
			panic(err)
		}
	} else {
		config = tree.DefaultSampleTypeMapping
	}
	_ = Profile
	_ = PreviousProfile

	if true {
		Profile = decompres(Profile)
		PreviousProfile = decompres(PreviousProfile)
	}
	//fmt.Println(config)
	elem := &testcase{Profile, PreviousProfile, config, fname, "gospy"}
	return elem
}

var small_pprof = "../../../../../cloudstorage/pkg/pyroscope/pprof/testdata/2022-10-08T00:44:10Z-55903298-d296-4730-a28d-9dcc7c5e25d6.txt"

var big_pprof = "../../../../../cloudstorage/pkg/pyroscope/pprof/testdata/2022-10-08T00:07:00Z-911c824f-a086-430c-99d7-315a53b58095.txt"

func BenchmarkSingleSmallMolecule(b *testing.B) {
	t := readCorpusItemFile(small_pprof)
	now := time.Now()
	for i := 0; i < b.N; i++ {

		config := t.config
		profile := t.profile
		parser := streaming.NewStreamingParser(streaming.ParserConfig{SampleTypes: config, Putter: putter, ArenasEnabled: true})
		err := parser.ParsePprof(context.TODO(), now, now, profile, false)
		if err != nil {
			b.Fatal(err)
		}
		parser.FreeArena()
	}
}

func BenchmarkSingleSmallUnmarshal(b *testing.B) {
	t := readCorpusItemFile(small_pprof)
	now := time.Now()
	for i := 0; i < b.N; i++ {
		config := t.config
		profile := t.profile
		parser := pprof.NewParser(pprof.ParserConfig{SampleTypes: config, Putter: putter})
		err := parser.ParsePprof(context.TODO(), now, now, profile, false)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSingleBigMolecule(b *testing.B) {
	t := readCorpusItemFile(big_pprof)
	now := time.Now()
	for i := 0; i < b.N; i++ {
		config := t.config
		profile := t.profile
		parser := streaming.NewStreamingParser(streaming.ParserConfig{SampleTypes: config, Putter: putter, ArenasEnabled: true})
		err := parser.ParsePprof(context.TODO(), now, now, profile, false)
		if err != nil {
			b.Fatal(err)
		}
		parser.FreeArena()
	}
}

func BenchmarkSingleBigUnmarshal(b *testing.B) {
	t := readCorpusItemFile(big_pprof)
	now := time.Now()
	for i := 0; i < b.N; i++ {
		config := t.config
		profile := t.profile
		parser := pprof.NewParser(pprof.ParserConfig{SampleTypes: config, Putter: putter})
		err := parser.ParsePprof(context.TODO(), now, now, profile, false)
		if err != nil {
			b.Fatal(err)
		}
	}
}

//func BenchmarkCorpus(b *testing.B) {
//	corpus := readCorpus("/home/korniltsev/Downloads/testcompare_pprofs")
//	now := time.Now()
//	var p pprof.ParserInterface
//	useNew := true
//	n := 5
//	for i := 0; i < n; i++ {
//		j := i
//		b.Run(fmt.Sprintf("BenchmarkCorpus_%d", j), func(b *testing.B) {
//			t := corpus[j]
//			config := t.config
//			profile := t.profile
//			for k := 0; k < b.N; k++ {
//				var parser *streaming.VTStreamingParser
//				if useNew {
//					parser = streaming.VTStreamingParserFromPool(streaming.ParserConfig{SampleTypes: config, Putter: putter})
//					p = parser
//				} else {
//					p = pprof.NewParser(pprof.ParserConfig{SampleTypes: config, Putter: putter})
//				}
//				err := p.ParsePprof(context.TODO(), now, now, profile, false)
//				if err != nil {
//					b.Fatal(err)
//				}
//				if parser != nil {
//					parser.ReturnToPool()
//					//parser.FreeArena()
//				}
//			}
//		})
//	}
//}
