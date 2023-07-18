package health

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/grafana/pyroscope/pkg/og/util/bytesize"
	"github.com/grafana/pyroscope/pkg/og/util/disk"
)

var _ = Describe("DiskPressure", func() {
	It("does not fire if threshold is zero", func() {
		var d DiskPressure
		m, err := d.Probe()
		Expect(err).ToNot(HaveOccurred())
		Expect(m.Status).To(Equal(Healthy))
		Expect(m.Message).To(BeEmpty())
	})

	It("does not fire if available space is greater than the configured threshold", func() {
		d := DiskPressure{
			Threshold: 5,
		}
		m, err := d.makeProbe(disk.UsageStats{
			Total:     10 * bytesize.GB,
			Available: 1 * bytesize.GB,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(m.Status).To(Equal(Healthy))
		Expect(m.Message).To(BeEmpty())
	})

	It("fires if less than the configured threshold is available", func() {
		d := DiskPressure{
			Threshold: 5,
		}
		m, err := d.makeProbe(disk.UsageStats{
			Total:     100 * bytesize.GB,
			Available: 4 * bytesize.GB,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(m.Status).To(Equal(Critical))
		Expect(m.Message).To(Equal("Disk space is running low: 4.00 GB available (4.0%)"))
	})

	It("fires if less than 1GB is available", func() {
		d := DiskPressure{
			Threshold: 5,
		}
		m, err := d.makeProbe(disk.UsageStats{
			Total:     5 * bytesize.GB,
			Available: bytesize.GB - 1,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(m.Status).To(Equal(Critical))
		Expect(m.Message).To(Equal("Disk space is running low: 1024.00 MB available (20.0%)"))
	})

	It("fires if available is less than the configured threshold", func() {
		d := DiskPressure{
			Threshold: 5,
		}
		m, err := d.makeProbe(disk.UsageStats{
			Total:     1 * bytesize.GB,
			Available: 1 * bytesize.MB,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(m.Status).To(Equal(Critical))
		Expect(m.Message).To(Equal("Disk space is running low: 1.00 MB available (0.1%)"))
	})

	It("fires if no space available", func() {
		d := DiskPressure{
			Threshold: 5,
		}
		m, err := d.makeProbe(disk.UsageStats{
			Total:     100 * bytesize.MB,
			Available: 0,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(m.Status).To(Equal(Critical))
		Expect(m.Message).To(Equal("Disk space is running low: 0 bytes available (0.0%)"))
	})

	It("fails if Available > Total", func() {
		var d DiskPressure
		_, err := d.makeProbe(disk.UsageStats{
			Total:     1 * bytesize.GB,
			Available: 2 * bytesize.GB,
		})
		Expect(err).To(MatchError(errTotalLessThanAvailable))
	})

	It("fails if Total is zero", func() {
		var d DiskPressure
		_, err := d.makeProbe(disk.UsageStats{
			Total:     0,
			Available: 2 * bytesize.GB,
		})
		Expect(err).To(MatchError(errZeroTotalSize))
	})
})
