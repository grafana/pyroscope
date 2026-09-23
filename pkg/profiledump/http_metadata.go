package profiledump

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// HTTPMetadata preserves Content-Encoding values and whether gzip was decompressed.
// Values are borrowed until Capture returns and base64-encoded to preserve raw bytes.
type HTTPMetadata struct {
	ContentEncoding  []string
	GzipDecompressed bool
}

func httpMetadataSize(m HTTPMetadata) int64 {
	n := int64(len(`{"content_encoding_base64":[],"gzip_decompressed":false}`))
	if m.GzipDecompressed {
		n-- // true is one byte shorter than false.
	}
	for i, value := range m.ContentEncoding {
		if len(value) > MaxMetadataSize {
			return MaxMetadataSize + 1
		}
		n += int64(2 + base64.StdEncoding.EncodedLen(len(value)))
		if i > 0 {
			n++
		}
		if n > MaxMetadataSize {
			return MaxMetadataSize + 1
		}
	}
	return n
}

func (m HTTPMetadata) MarshalJSON() ([]byte, error) {
	n := httpMetadataSize(m)
	if n > MaxMetadataSize {
		return nil, fmt.Errorf("HTTP metadata exceeds byte limit")
	}
	b := make([]byte, 0, int(n))
	b = append(b, `{"content_encoding_base64":[`...)
	for i, value := range m.ContentEncoding {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '"')
		b = base64.StdEncoding.AppendEncode(b, []byte(value))
		b = append(b, '"')
	}
	b = append(b, `],"gzip_decompressed":`...)
	if m.GzipDecompressed {
		b = append(b, "true}"...)
	} else {
		b = append(b, "false}"...)
	}
	return b, nil
}

func (m *HTTPMetadata) UnmarshalJSON(b []byte) error {
	var wire struct {
		ContentEncoding  [][]byte `json:"content_encoding_base64"`
		GzipDecompressed bool     `json:"gzip_decompressed"`
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&wire); err != nil {
		return err
	}
	*m = HTTPMetadata{GzipDecompressed: wire.GzipDecompressed}
	for _, value := range wire.ContentEncoding {
		m.ContentEncoding = append(m.ContentEncoding, string(value))
	}
	return nil
}
