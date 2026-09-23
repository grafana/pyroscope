package profiledump

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodedMetadataSizeParityRepresentationsAndIdentity(t *testing.T) {
	multipart := Representation{ContentType: `multipart/form-data; boundary="a\"b\\c"`, Encoding: "identity", Syntax: "multipart"}
	for _, tc := range []struct {
		name   string
		change func(*Metadata)
	}{
		{"stored multipart escaping", func(m *Metadata) { m.Stored = multipart }},
		{"incoming multipart escaping", func(m *Metadata) { m.Incoming = &multipart }},
		{"tenant escaping", func(m *Metadata) { m.TenantID = "tenant\"\\<&>日本語\u2028\u2029" }},
		{"distributor escaping", func(m *Metadata) { m.DistributorID = "distributor\"\\<&>日本語\u2028\u2029" }},
		{"maximum escaped tenant", func(m *Metadata) { m.TenantID = strings.Repeat("&", MaxTenantBytes) }},
		{"maximum escaped distributor", func(m *Metadata) { m.DistributorID = strings.Repeat("<", MaxTextBytes) }},
		{"empty label map", func(m *Metadata) { m.Labels = map[string]string{} }},
		{"multiple escaped labels", func(m *Metadata) {
			m.Labels = map[string]string{`quoted"name`: `back\slash`, "html&": "<>日本語\u2028\u2029"}
		}},
		{"maximum label count", func(m *Metadata) {
			m.Labels = make(map[string]string, MaxLabels)
			for i := 0; i < MaxLabels; i++ {
				m.Labels[fmt.Sprintf(`label"%d`, i)] = `<value\>`
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testMetadata(123456789)
			tc.change(&m)
			require.NoError(t, m.Validate())
			encoded, err := json.Marshal(m)
			require.NoError(t, err)
			require.Equal(t, int64(len(encoded)), encodedMetadataSize(m))
		})
	}
}

// The size counter intentionally omits JSON control-character/invalid-UTF-8
// replacement escapes: validation must reject these strings before counting.
func TestMetadataSizingRejectsUnsupportedStrings(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*Metadata, string)
	}{
		{"tenant", func(m *Metadata, s string) { m.TenantID = s }},
		{"distributor", func(m *Metadata, s string) { m.DistributorID = s }},
		{"profile ID", func(m *Metadata, s string) { m.OriginalProfileID = s }},
		{"stored content type", func(m *Metadata, s string) { m.Stored.ContentType = "text/plain; x=\"" + s + "\"" }},
		{"incoming content type", func(m *Metadata, s string) { m.Incoming.ContentType = "text/plain; x=\"" + s + "\"" }},
		{"label name", func(m *Metadata, s string) { m.Labels = map[string]string{s: "value"} }},
		{"label value", func(m *Metadata, s string) { m.Labels = map[string]string{"name": s} }},
	} {
		for _, value := range []string{"a\nb", "a\rb", "a\tb", "a\x00b", "a\x7fb", "a\xffb"} {
			t.Run(fmt.Sprintf("%s/%q", tc.name, value), func(t *testing.T) {
				m := testMetadata(1)
				tc.set(&m, value)
				require.Error(t, m.Validate())
			})
		}
	}
}
