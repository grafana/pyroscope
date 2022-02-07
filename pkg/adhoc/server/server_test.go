package server_test

import (
	"reflect"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/adhoc/server"
)

var _ = Describe("Server", func() {
	Describe("Detecting format", func() {
		Context("with a valid pprof type", func() {
			When("there's only type", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Type: "pprof",
					}
				})
				It("should return pprof as type is be enough to detect the type", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.PprofToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
			When("there's pprof type and json filename", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Filename: "profile.json",
						Type:     "pprof",
					}
				})
				It("should return pprof as type takes precedence over filename", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.PprofToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's pprof type and json profile contents", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Profile: []byte(`{"flamebearer":""}`),
						Type:    "pprof",
					}
				})
				It("should return pprof as type takes precedence over profile contents", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.PprofToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with no (valid) type and a valid pprof filename", func() {
			When("there's pprof filename and json profile contents", func() {
				var m server.Model
				BeforeEach(func() {
					m = server.Model{
						Filename: "profile.pprof",
						Profile:  []byte(`{"flamebearer":""}`),
					}
				})

				It("should return pprof as filename takes precedence over profile contents", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.PprofToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
			When("there's pprof filename and an unsupported type", func() {
				var m server.Model
				BeforeEach(func() {
					m = server.Model{
						Filename: "profile.pprof",
						Type:     "unsupported",
					}
				})

				It("should return pprof as unsupported type is ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.PprofToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with no (valid) type and filename, a valid pprof profile", func() {
			When("there's a profile with uncompressed pprof content", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Profile: []byte{0x0a, 0x04},
					}
				})

				It("should return pprof", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.PprofToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's a profile with compressed pprof content", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Profile: []byte{0x1f, 0x8b},
					}
				})

				It("should return pprof", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.PprofToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's a profile with compressed pprof content and an unsupported type", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Profile: []byte{0x1f, 0x8b},
						Type:    "unsupported",
					}
				})

				It("should return pprof as unsupported types are ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.PprofToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's a profile with compressed pprof content and unsupported filename", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Filename: "profile.unsupported",
						Profile:  []byte{0x1f, 0x8b},
					}
				})

				It("should return pprof as unsupported filenames are ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.PprofToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with a valid json type", func() {
			When("there's only type", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Type: "json",
					}
				})
				It("should return json as type is be enough to detect the type", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.JSONToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
			When("there's json type and pprof filename", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Filename: "profile.pprof",
						Type:     "json",
					}
				})
				It("should return json as type takes precedence over filename", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.JSONToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's json type and pprof profile contents", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Profile: []byte{0x1f, 0x8b},
						Type:    "json",
					}
				})
				It("should return json as type takes precedence over profile contents", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.JSONToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with no (valid) type and a valid json filename", func() {
			When("there's json filename and pprof profile contents", func() {
				var m server.Model
				BeforeEach(func() {
					m = server.Model{
						Filename: "profile.json",
						Profile:  []byte{0x1f, 0x8b},
					}
				})

				It("should return json as filename takes precedence over profile contents", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.JSONToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
			When("there's json filename and an unsupported type", func() {
				var m server.Model
				BeforeEach(func() {
					m = server.Model{
						Filename: "profile.json",
						Type:     "unsupported",
					}
				})

				It("should return json as unsupported type is ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.JSONToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with no (valid) type and filename, a valid json profile", func() {
			When("there's a profile with json content", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Profile: []byte(`{"flamebearer":""}`),
					}
				})

				It("should return json", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.JSONToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's a profile with json content and an unsupported type", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Profile: []byte(`{"flamebearer":""}`),
						Type:    "unsupported",
					}
				})

				It("should return json as unsupported types are ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.JSONToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's a profile with json content and unsupported filename", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Filename: "profile.unsupported",
						Profile:  []byte(`{"flamebearer":""}`),
					}
				})

				It("should return json as unsupported filenames are ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.JSONToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with a valid collapsed type", func() {
			When("there's only type", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Type: "collapsed",
					}
				})
				It("should return collapsed as type is be enough to detect the type", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.CollapsedToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
			When("there's collapsed type and pprof filename", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Filename: "profile.pprof",
						Type:     "collapsed",
					}
				})
				It("should return collapsed as type takes precedence over filename", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.CollapsedToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's json type and pprof profile contents", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Profile: []byte{0x1f, 0x8b},
						Type:    "collapsed",
					}
				})
				It("should return collapsed as type takes precedence over profile contents", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.CollapsedToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with no (valid) type and a valid collapsed filename", func() {
			When("there's collapsed filename and pprof profile contents", func() {
				var m server.Model
				BeforeEach(func() {
					m = server.Model{
						Filename: "profile.collapsed",
						Profile:  []byte{0x1f, 0x8b},
					}
				})

				It("should return collapsed as filename takes precedence over profile contents", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.CollapsedToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
			When("there's collapsed filename and an unsupported type", func() {
				var m server.Model
				BeforeEach(func() {
					m = server.Model{
						Filename: "profile.collapsed",
						Type:     "unsupported",
					}
				})

				It("should return collapsed as unsupported type is ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.CollapsedToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's collapsed text filename and an unsupported type", func() {
				var m server.Model
				BeforeEach(func() {
					m = server.Model{
						Filename: "profile.collapsed.txt",
						Type:     "unsupported",
					}
				})

				It("should return collapsed as unsupported type is ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.CollapsedToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})

		Context("with no (valid) type and filename, a valid collapsed profile", func() {
			When("there's a profile with collapsed content", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Profile: []byte("fn1 1\nfn2 2"),
					}
				})

				It("should return collapsed", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.CollapsedToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's a profile with collapsed content and an unsupported type", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Profile: []byte("fn1 1\nfn2 2"),
						Type:    "unsupported",
					}
				})

				It("should return collapsed as unsupported types are ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.CollapsedToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})

			When("there's a profile with collapsed content and unsupported filename", func() {
				var m server.Model

				BeforeEach(func() {
					m = server.Model{
						Filename: "profile.unsupported",
						Profile:  []byte("fn1 1\nfn2 2"),
					}
				})

				It("should return collapsed as unsupported filenames are ignored", func() {
					// We want to compare functions, which is not ideal.
					expected := reflect.ValueOf(server.CollapsedToProfileV1).Pointer()
					f, err := m.Converter()
					Expect(err).To(BeNil())
					Expect(f).ToNot(BeNil())
					Expect(reflect.ValueOf(f).Pointer()).To(Equal(expected))
				})
			})
		})
	})

	Context("with an empty model", func() {
		var m server.Model
		It("should return an error", func() {
			_, err := m.Converter()
			Expect(err).ToNot(Succeed())
		})
	})

})
