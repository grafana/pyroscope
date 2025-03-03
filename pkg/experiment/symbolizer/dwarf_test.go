package symbolizer

import (
	"context"
	"debug/dwarf"
	"testing"

	pprof "github.com/google/pprof/profile"
	"github.com/stretchr/testify/assert"
)

func TestResolveAddress(t *testing.T) {
	tests := []struct {
		name    string
		addr    uint64
		setup   func(*DWARFInfo)
		want    []SymbolLocation
		wantErr bool
	}{
		{
			name: "address found in optimized maps",
			addr: 0x1000,
			setup: func(d *DWARFInfo) {
				d.addressMap[0x1000] = &FunctionInfo{
					Ranges: []FunctionRange{{
						StartAddr: 0x1000,
						EndAddr:   0x2000,
					}},
					Locations: []SymbolLocation{{
						Function: &pprof.Function{
							Name:     "main",
							Filename: "main.go",
						},
						Line: 10,
					}},
				}
			},
			want: []SymbolLocation{{
				Function: &pprof.Function{
					Name:     "main",
					Filename: "main.go",
				},
				Line: 10,
			}},
		},
		{
			name: "address found in range but not exact match",
			addr: 0x1500,
			setup: func(d *DWARFInfo) {
				d.addressMap[0x1000] = &FunctionInfo{
					Ranges: []FunctionRange{{
						StartAddr: 0x1000,
						EndAddr:   0x2000,
					}},
					Locations: []SymbolLocation{{
						Function: &pprof.Function{
							Name:     "main",
							Filename: "main.go",
						},
						Line: 10,
					}},
				}
			},
			want: []SymbolLocation{{
				Function: &pprof.Function{
					Name:     "main",
					Filename: "main.go",
				},
				Line: 10,
			}},
		},
		{
			name: "address with inlined functions",
			addr: 0x1000,
			setup: func(d *DWARFInfo) {
				d.addressMap[0x1000] = &FunctionInfo{
					Ranges: []FunctionRange{{
						StartAddr: 0x1000,
						EndAddr:   0x2000,
					}},
					Locations: []SymbolLocation{
						{
							Function: &pprof.Function{
								Name:     "memcpy",
								Filename: "string.h",
							},
							Line: 42,
						},
						{
							Function: &pprof.Function{
								Name:     "printf",
								Filename: "stdio.h",
							},
							Line: 100,
						},
					},
				}
			},
			want: []SymbolLocation{
				{
					Function: &pprof.Function{
						Name:     "memcpy",
						Filename: "string.h",
					},
					Line: 42,
				},
				{
					Function: &pprof.Function{
						Name:     "printf",
						Filename: "stdio.h",
					},
					Line: 100,
				},
			},
		},
		{
			name: "address not found",
			addr: 0x3000,
			setup: func(d *DWARFInfo) {
				d.addressMap[0x1000] = &FunctionInfo{
					Ranges: []FunctionRange{{
						StartAddr: 0x1000,
						EndAddr:   0x2000,
					}},
					Locations: []SymbolLocation{{
						Function: &pprof.Function{
							Name:     "main",
							Filename: "main.go",
						},
						Line: 10,
					}},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDWARFInfo(&dwarf.Data{})
			if tt.setup != nil {
				tt.setup(d)
			}

			got, err := d.ResolveAddress(context.Background(), tt.addr)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindLineInfo(t *testing.T) {
	tests := []struct {
		name     string
		entries  []dwarf.LineEntry
		ranges   [][2]uint64
		wantFile string
		wantLine int64
	}{
		{
			name: "exact match found",
			entries: []dwarf.LineEntry{{
				Address: 0x1000,
				File:    &dwarf.LineFile{Name: "test.go"},
				Line:    42,
				IsStmt:  true,
			}},
			ranges:   [][2]uint64{{0x1000, 0x2000}},
			wantFile: "test.go",
			wantLine: 42,
		},
		{
			name: "closest match before target",
			entries: []dwarf.LineEntry{
				{
					Address: 0x500,
					File:    &dwarf.LineFile{Name: "before.go"},
					Line:    10,
					IsStmt:  true,
				},
				{
					Address: 0x2500,
					File:    &dwarf.LineFile{Name: "after.go"},
					Line:    20,
					IsStmt:  true,
				},
			},
			ranges:   [][2]uint64{{0x1000, 0x2000}},
			wantFile: "before.go",
			wantLine: 10,
		},
		{
			name:     "no entries",
			entries:  []dwarf.LineEntry{},
			ranges:   [][2]uint64{{0x1000, 0x2000}},
			wantFile: "?",
			wantLine: 0,
		},
		{
			name: "multiple entries in range",
			entries: []dwarf.LineEntry{
				{
					Address: 0x1000,
					File:    &dwarf.LineFile{Name: "first.go"},
					Line:    10,
					IsStmt:  true,
				},
				{
					Address: 0x1500,
					File:    &dwarf.LineFile{Name: "second.go"},
					Line:    20,
					IsStmt:  true,
				},
			},
			ranges:   [][2]uint64{{0x1000, 0x2000}},
			wantFile: "first.go",
			wantLine: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDWARFInfo(&dwarf.Data{})
			gotFile, gotLine := d.findLineInfo(tt.entries, tt.ranges)
			assert.Equal(t, tt.wantFile, gotFile)
			assert.Equal(t, tt.wantLine, gotLine)
		})
	}
}
