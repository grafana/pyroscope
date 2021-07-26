package cli

import (
	"bytes"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

type FlagsStruct struct {
	Config   string
	Foo      string
	Foos     []string
	Bar      int
	Baz      time.Duration
	FooBar   string
	FooFoo   float64
	FooBytes bytesize.ByteSize
}

var _ = Describe("flags", func() {
	Context("PopulateFlagSet", func() {
		Context("without config file", func() {
			It("correctly sets all types of arguments", func() {
				exampleCommand := &cobra.Command{
					RunE: func(cmd *cobra.Command, args []string) error {
						return nil
					},
				}

				cfg := FlagsStruct{}
				PopulateFlagSet(&cfg, exampleCommand.Flags())

				b := bytes.NewBufferString("")
				exampleCommand.SetOut(b)
				exampleCommand.SetArgs([]string{
					fmt.Sprintf("--foo=%s", "test-val-1"),
					fmt.Sprintf("--foos=%s", "test-val-2"),
					fmt.Sprintf("--foos=%s", "test-val-3"),
					fmt.Sprintf("--bar=%s", "123"),
					fmt.Sprintf("--baz=%s", "10h"),
					fmt.Sprintf("--foo-bar=%s", "test-val-4"),
					fmt.Sprintf("--foo-foo=%s", "10.23"),
					fmt.Sprintf("--foo-bytes=%s", "100MB"),
				})

				err := exampleCommand.Execute()
				Expect(err).ToNot(HaveOccurred())
				Expect(cfg.Foo).To(Equal("test-val-1"))
				Expect(cfg.Foos).To(Equal([]string{"test-val-2", "test-val-3"}))
				Expect(cfg.Bar).To(Equal(123))
				Expect(cfg.Baz).To(Equal(10 * time.Hour))
				Expect(cfg.FooBar).To(Equal("test-val-4"))
				Expect(cfg.FooFoo).To(Equal(10.23))
				Expect(cfg.FooBytes).To(Equal(100 * bytesize.MB))
			})
		})
	})
})
