package heapanalyzer

import (
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

type HeapDump struct {
	Id string `json:"id"`

	// unix millis
	CreatedAt int64           `json:"createdAt"`
	Labels    *typesv1.Labels `json:"labels"`
}

type ObjectTypeStats struct {
	Type      string `json:"type"`
	Count     int64  `json:"count"`
	TotalSize int64  `json:"totalSize"`
}

type Object struct {
	Id          string `json:"id"`
	Type        string `json:"type"`
	Address     string `json:"address"`
	DisplayName string `json:"displayName"`
	Size        int64  `json:"size"`
}

type Field struct {
	Name    string `json:"name,omitempty"`
	Type    string `json:"type"`
	Value   string `json:"value,omitempty"`
	Pointer string `json:"pointer,omitempty"`

	Fields []*Field `json:"fields,omitempty"`
}

type ObjectWithDetails struct {
	Object

	Fields     []*Field     `json:"fields"`
	References []*Reference `json:"references"`
}

type Reference struct {
	From string `json:"from"`
}
