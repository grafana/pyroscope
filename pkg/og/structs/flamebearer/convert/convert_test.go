package convert

import (
	"io/ioutil"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer"
)

var _ = Describe("Server", func() {
	Describe("Detecting format", func() {
		Context("with a valid pprof type", func() {
			When("there's only type", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Type: "pprof",
					}
				})
				It("should return pprof as type is be enough to detect the type", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(PprofToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
			When("there's pprof type and json filename", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Name: "profile.json",
						Type: "pprof",
					}
				})
				It("should return pprof as type takes precedence over filename", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(PprofToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's pprof type and json profile contents", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Data: []byte(`{"flamebearer":""}`),
						Type: "pprof",
					}
				})
				It("should return pprof as type takes precedence over profile contents", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(PprofToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with no (valid) type and a valid pprof filename", func() {
			When("there's pprof filename and json profile contents", func() {
				var m ProfileFile
				BeforeEach(func() {
					m = ProfileFile{
						Name: "profile.pprof",
						Data: []byte(`{"flamebearer":""}`),
					}
				})

				It("should return pprof as filename takes precedence over profile contents", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(PprofToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
			When("there's pprof filename and an unsupported type", func() {
				var m ProfileFile
				BeforeEach(func() {
					m = ProfileFile{
						Name: "profile.pprof",
						Type: "unsupported",
					}
				})

				It("should return pprof as unsupported type is ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(PprofToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with no (valid) type and filename, a valid pprof profile", func() {
			When("there's a profile with uncompressed pprof content", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Data: []byte{0x0a, 0x04},
					}
				})

				It("should return pprof", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(PprofToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's a profile with compressed pprof content", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Data: []byte{0x1f, 0x8b},
					}
				})

				It("should return pprof", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(PprofToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's a profile with compressed pprof content and an unsupported type", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Data: []byte{0x1f, 0x8b},
						Type: "unsupported",
					}
				})

				It("should return pprof as unsupported types are ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(PprofToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's a profile with compressed pprof content and unsupported filename", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Name: "profile.unsupported",
						Data: []byte{0x1f, 0x8b},
					}
				})

				It("should return pprof as unsupported filenames are ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(PprofToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with a valid json type", func() {
			When("there's only type", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Type: "json",
					}
				})
				It("should return json as type is be enough to detect the type", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(JSONToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
			When("there's json type and pprof filename", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Name: "profile.pprof",
						Type: "json",
					}
				})
				It("should return json as type takes precedence over filename", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(JSONToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's json type and pprof profile contents", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Data: []byte{0x1f, 0x8b},
						Type: "json",
					}
				})
				It("should return json as type takes precedence over profile contents", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(JSONToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with no (valid) type and a valid json filename", func() {
			When("there's json filename and pprof profile contents", func() {
				var m ProfileFile
				BeforeEach(func() {
					m = ProfileFile{
						Name: "profile.json",
						Data: []byte{0x1f, 0x8b},
					}
				})

				It("should return json as filename takes precedence over profile contents", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(JSONToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
			When("there's json filename and an unsupported type", func() {
				var m ProfileFile
				BeforeEach(func() {
					m = ProfileFile{
						Name: "profile.json",
						Type: "unsupported",
					}
				})

				It("should return json as unsupported type is ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(JSONToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with no (valid) type and filename, a valid json profile", func() {
			When("there's a profile with json content", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Data: []byte(`{"flamebearer":""}`),
					}
				})

				It("should return json", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(JSONToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's a profile with json content and an unsupported type", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Data: []byte(`{"flamebearer":""}`),
						Type: "unsupported",
					}
				})

				It("should return json as unsupported types are ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(JSONToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's a profile with json content and unsupported filename", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Name: "profile.unsupported",
						Data: []byte(`{"flamebearer":""}`),
					}
				})

				It("should return json as unsupported filenames are ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(JSONToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with a valid collapsed type", func() {
			When("there's only type", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Type: "collapsed",
					}
				})
				It("should return collapsed as type is be enough to detect the type", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(CollapsedToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
			When("there's collapsed type and pprof filename", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Name: "profile.pprof",
						Type: "collapsed",
					}
				})
				It("should return collapsed as type takes precedence over filename", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(CollapsedToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's json type and pprof profile contents", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Data: []byte{0x1f, 0x8b},
						Type: "collapsed",
					}
				})
				It("should return collapsed as type takes precedence over profile contents", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(CollapsedToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with no (valid) type and a valid collapsed filename", func() {
			When("there's collapsed filename and pprof profile contents", func() {
				var m ProfileFile
				BeforeEach(func() {
					m = ProfileFile{
						Name: "profile.collapsed",
						Data: []byte{0x1f, 0x8b},
					}
				})

				It("should return collapsed as filename takes precedence over profile contents", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(CollapsedToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
			When("there's collapsed filename and an unsupported type", func() {
				var m ProfileFile
				BeforeEach(func() {
					m = ProfileFile{
						Name: "profile.collapsed",
						Type: "unsupported",
					}
				})

				It("should return collapsed as unsupported type is ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(CollapsedToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's collapsed text filename and an unsupported type", func() {
				var m ProfileFile
				BeforeEach(func() {
					m = ProfileFile{
						Name: "profile.collapsed.txt",
						Type: "unsupported",
					}
				})

				It("should return collapsed as unsupported type is ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(CollapsedToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with no (valid) type and filename, a valid collapsed profile", func() {
			When("there's a profile with collapsed content", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Data: []byte("fn1 1\nfn2 2"),
					}
				})

				It("should return collapsed", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(CollapsedToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's a profile with collapsed content and an unsupported type", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Data: []byte("fn1 1\nfn2 2"),
						Type: "unsupported",
					}
				})

				It("should return collapsed as unsupported types are ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(CollapsedToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's a profile with collapsed content and unsupported filename", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Name: "profile.unsupported",
						Data: []byte("fn1 1\nfn2 2"),
					}
				})

				It("should return collapsed as unsupported filenames are ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(CollapsedToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("perf script", func() {
			When("detect by content", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Data: []byte("java 12688 [002] 6544038.708352: cpu-clock:\n\n"),
					}
				})

				It("should return perf_script", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(PerfScriptToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
			When("detect by .txt extension and content", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Name: "foo.txt",
						Data: []byte("java 12688 [002] 6544038.708352: cpu-clock:\n\n"),
					}
				})

				It("should return perf_script", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(PerfScriptToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
			When("detect by .perf_script extension", func() {
				var m ProfileFile

				BeforeEach(func() {
					m = ProfileFile{
						Name: "foo.perf_script",
						Data: []byte("foo;bar 239"),
					}
				})

				It("should return perf_script", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(PerfScriptToProfile).Pointer()
					f, err := converter(m)
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with an empty ProfileFile", func() {
			var m ProfileFile
			It("should return an error", func() {
				_, err := converter(m)
				Expect(err).ToNot(Succeed())
			})
		})
	})

	Describe("Calling DiffV1", func() {
		Context("with v1 profiles", func() {
			var base, diff *flamebearer.FlamebearerProfile

			When("Diff is called with valid and equal base and diff profiles", func() {
				BeforeEach(func() {
					base = &flamebearer.FlamebearerProfile{
						Version: 1,
						FlamebearerProfileV1: flamebearer.FlamebearerProfileV1{
							Metadata: flamebearer.FlamebearerMetadataV1{
								Format: "single",
							},
							// Taken from flamebearer test
							Flamebearer: flamebearer.FlamebearerV1{
								Names: []string{"total", "a", "c", "b"},
								Levels: [][]int{
									{0, 3, 0, 0},
									{0, 3, 0, 1},
									{0, 1, 1, 3, 0, 2, 2, 2},
								},
								NumTicks: 3,
								MaxSelf:  2,
							},
						},
					}

					diff = &flamebearer.FlamebearerProfile{
						Version: 1,
						FlamebearerProfileV1: flamebearer.FlamebearerProfileV1{
							Metadata: flamebearer.FlamebearerMetadataV1{
								Format: "single",
							},
							// Taken from flamebearer test
							Flamebearer: flamebearer.FlamebearerV1{
								Names: []string{"total", "a", "c", "b"},
								Levels: [][]int{
									{0, 3, 0, 0},
									{0, 3, 0, 1},
									{0, 1, 1, 3, 0, 2, 2, 2},
								},
								NumTicks: 3,
								MaxSelf:  2,
							},
						},
					}
				})

				It("returns the diff profile", func() {
					fb, err := flamebearer.Diff("name", base, diff, 1024)
					Expect(err).To(Succeed())
					Expect(fb.Version).To(Equal(uint(1)))
					Expect(fb.Metadata.Name).To(Equal("name"))
					Expect(fb.Metadata.Format).To(Equal("double"))
					Expect(fb.Flamebearer.Names).To(Equal([]string{"total", "a", "c", "b"}))
					Expect(fb.Flamebearer.Levels).To(Equal([][]int{
						{0, 3, 0, 0, 3, 0, 0},
						{0, 3, 0, 0, 3, 0, 1},
						{0, 1, 1, 0, 1, 1, 3, 0, 2, 2, 0, 2, 2, 2},
					}))
					Expect(fb.Flamebearer.NumTicks).To(Equal(6))
					Expect(fb.Flamebearer.MaxSelf).To(Equal(2))
					Expect(fb.LeftTicks).To(Equal(uint64(3)))
					Expect(fb.RightTicks).To(Equal(uint64(3)))
				})
			})
		})
	})
})

var _ = Describe("Convert", func() {
	It("converts malformed pprof", func() {
		m := ProfileFile{
			Type: "pprof",
			Data: readFile("./testdata/cpu-unknown.pb.gz"),
		}

		f, err := converter(m)
		Expect(err).To(BeNil())
		Expect(f).ToNot(BeNil())

		b, err := f(m.Data, "appname", 1024)
		Expect(err).To(BeNil())
		Expect(b).ToNot(BeNil())
	})

	Describe("JSON", func() {
		It("prunes tree", func() {
			m := ProfileFile{
				Type: "json",
				Data: readFile("./testdata/profile.json"),
			}

			f, err := converter(m)
			Expect(err).To(BeNil())
			Expect(f).ToNot(BeNil())

			b, err := f(m.Data, "appname", 1)
			Expect(err).To(BeNil())
			Expect(b).ToNot(BeNil())

			// 1 + total
			Expect(len(b[0].FlamebearerProfileV1.Flamebearer.Levels)).To(Equal(2))
		})
	})
})

func readFile(path string) []byte {
	f, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return f
}
