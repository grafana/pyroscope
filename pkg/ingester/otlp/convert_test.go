package otlp

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    v1 "go.opentelemetry.io/proto/otlp/common/v1"
    v1experimental "go.opentelemetry.io/proto/otlp/profiles/v1development"
)

func TestConvertLocationBackKernelFrameType(t *testing.T) {
    tests := []struct {
        name             string
        frameType        string
        mappingFilename  string
        expectedFilename string
    }{
        {
            name:             "kernel frame type adds prefix",
            frameType:        "kernel",
            mappingFilename:  "/lib/modules/kernel.ko",
            expectedFilename: "[kernel] /lib/modules/kernel.ko",
        },
        {
            name:             "kernel prefix not duplicated",
            frameType:        "kernel",
            mappingFilename:  "[kernel] /lib/modules/kernel.ko",
            expectedFilename: "[kernel] /lib/modules/kernel.ko",
        },
        {
            name:             "non-kernel frame type unchanged",
            frameType:        "native",
            mappingFilename:  "/usr/bin/app",
            expectedFilename: "/usr/bin/app",
        },
        {
            name:             "no frame type unchanged",
            frameType:        "",
            mappingFilename:  "/usr/bin/app",
            expectedFilename: "/usr/bin/app",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            stringTable := []string{
                "",
                tt.mappingFilename,
                "profile.frame.type",
                tt.frameType,
                "samples",
                "count",
            }

            dictionary := &v1experimental.ProfilesDictionary{
                StringTable: stringTable,
                AttributeTable: []*v1experimental.KeyValueAndUnit{
                    {KeyStrindex: 0},
                    {
                        KeyStrindex: 2,
                        Value: &v1.AnyValue{
                            Value: &v1.AnyValue_StringValue{
                                StringValue: tt.frameType,
                            },
                        },
                    },
                },
                MappingTable: []*v1experimental.Mapping{
                    {FilenameStrindex: 0},
                    {FilenameStrindex: 1},
                },
                LocationTable: []*v1experimental.Location{
                    {},
                    {
                        MappingIndex:     1,
                        AttributeIndices: []int32{1},
                    },
                },
                FunctionTable: []*v1experimental.Function{},
                StackTable:    []*v1experimental.Stack{},
            }

            src := &v1experimental.Profile{
                SampleType: &v1experimental.ValueType{
                    TypeStrindex: 4,
                    UnitStrindex: 5,
                },
            }

            p, err := newProfileBuilder(src, dictionary)
            require.NoError(t, err)

            om := dictionary.MappingTable[1]
            ol := dictionary.LocationTable[1]
            _, err = p.convertMappingBack([]*v1experimental.Location{ol}, om, dictionary)
            require.NoError(t, err)

            _, err = p.convertLocationBack(ol, dictionary)
            require.NoError(t, err)

            require.Len(t, p.dst.Mapping, 1)
            actualFilename := p.dst.StringTable[p.dst.Mapping[0].Filename]
            assert.Equal(t, tt.expectedFilename, actualFilename)
        })
    }
}