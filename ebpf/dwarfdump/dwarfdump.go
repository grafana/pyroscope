package dwarfdump

import (
	"debug/dwarf"
	"debug/elf"
	"fmt"
	"reflect"
	"strings"
)

// based on https://github.com/grafana/beyla/blob/6b46732da73f2f2cb84e41efdc74789509a7fa2b/pkg/internal/goexec/structmembers.go

type Field struct {
	Name   string
	Offset uint64

	//attrs map[dwarf.Attr]any
}
type Typedef struct {
	Name        string
	TypeOffsets []dwarf.Offset
}
type Typ struct {
	Name   string
	Fields []Field
	Size   int64
}

func (t Typ) GetField(name string) *Field {
	for i, field := range t.Fields {
		if field.Name == name {
			return &t.Fields[i]
		}
	}
	return nil
}

type Index struct {
	offset2Type map[dwarf.Offset]*Typ
	typedefs    map[string]*Typedef
}

func (i *Index) GetTypeByName2(name string) *Typ {
	if name == "" {
		return nil
	}
	typedef := i.typedefs[name]
	if typedef == nil {
		return i.GetTypeByName(name)
	}
	var res []*Typ
	for _, offset := range typedef.TypeOffsets {
		typ := i.offset2Type[offset]
		if typ == nil {
			panic(fmt.Sprintf("%s %d not found", name, offset))
		}
		res = append(res, typ)
		if len(res) > 0 {
			prev := res[len(res)-1]
			if !reflect.DeepEqual(prev, typ) {
				panic(fmt.Sprintf("not eq %v prev %v", typ, prev))
			}
		}
	}
	if len(res) == 0 {
		return nil
	}
	return res[0]
}
func (i *Index) GetTypeByName(name string) *Typ {
	var res []*Typ
	for _, typ := range i.offset2Type {
		if typ.Name == name {
			res = append(res, typ)
			if len(res) > 0 {
				prev := res[len(res)-1]
				if !reflect.DeepEqual(prev, typ) {
					panic(fmt.Sprintf("not eq %v prev %v", typ, prev))
				}
			}
		}
	}
	if len(res) == 0 {
		return nil
		//panic(fmt.Sprintf("%s not found", name))
	}
	return res[0]
}

// structMemberOffsetsFromDwarf reads the executable dwarf information to get
// the offsets specified in the structMembers map
func structMemberOffsetsFromDwarf(data *dwarf.Data) (Index, error) {

	reader := data.Reader()
	res := Index{
		//name2Type : map[string]*Typ{},
		offset2Type: map[dwarf.Offset]*Typ{},
		typedefs:    map[string]*Typedef{},
	}

	for {
		entry, err := reader.Next()
		if err != nil {
			return res, err
		}
		if entry == nil { // END of dwarf data
			return res, nil
		}
		attrs := getAttrs(entry)

		if entry.Tag != dwarf.TagStructType && entry.Tag != dwarf.TagTypedef {
			continue
		}
		if entry.Tag == dwarf.TagTypedef {
			typeName, _ := attrs[dwarf.AttrName].(string)
			if typeName != "" {
				typedef := res.typedefs[typeName]
				if typedef == nil {
					typedef = &Typedef{Name: typeName}
					res.typedefs[typeName] = typedef
				}
				tt := attrs[dwarf.AttrType]
				if tt != nil {
					typedef.TypeOffsets = append(typedef.TypeOffsets, tt.(dwarf.Offset))
				} else {
					//fmt.Println("hek")
				}
			}
			continue
		}
		typeName, _ := attrs[dwarf.AttrName].(string)

		sz, _ := attrs[dwarf.AttrByteSize].(int64)
		if sz == 0 {
			reader.SkipChildren()
			continue
		}

		offsets, err := readMembers(reader)
		if err != nil {
			return res, err
		}
		nt := &Typ{
			Name:   typeName,
			Size:   sz,
			Fields: offsets,
		}
		res.offset2Type[(entry.Offset)] = nt

	}
}

func readMembers(
	reader *dwarf.Reader,
) ([]Field, error) {
	var res []Field
	for {
		entry, err := reader.Next()
		if err != nil {
			return res, fmt.Errorf("can't read DWARF data: %w", err)
		}
		if entry == nil { // END of dwarf data
			return res, nil
		}
		// Nil tag: end of the members list
		if entry.Tag == 0 {
			return res, nil
		}
		attrs := getAttrs(entry)
		name, nok := attrs[dwarf.AttrName].(string)
		value, vok := attrs[dwarf.AttrDataMemberLoc]
		//fmt.Printf("    %s %d\n", name, value)
		if nok && vok {

			res = append(res, Field{
				name,
				uint64(value.(int64)),
			})
		}
	}
}
func getAttrs(entry *dwarf.Entry) map[dwarf.Attr]any {
	attrs := map[dwarf.Attr]any{}
	for f := range entry.Field {
		attrs[entry.Field[f].Attr] = entry.Field[f].Val
	}
	return attrs
}

type FieldDump struct {
	Name   string
	Offset int
}

func Dump(elfPath string, fields []Need) []FieldDump {
	var err error

	f, err := elf.Open(elfPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	d, err := f.DWARF()
	if err != nil {
		panic(err)

	}

	types, err := structMemberOffsetsFromDwarf(d)
	if err != nil {
		panic(err)
	}

	var e []FieldDump
	for _, need := range fields {
		typ := types.GetTypeByName2(need.Name)
		if typ == nil {
			typ = types.GetTypeByName2(need.PrettyName)
		}
		//if typ == nil {
		//	panic(fmt.Sprintf("typ %s not found", need.Name))
		//}

		for _, needField := range need.Fields {
			o := -1
			if typ != nil {
				f := typ.GetField(needField.Name)
				if f != nil {
					o = int(f.Offset)
				}
			}
			pname := needField.PrintName
			if pname == "" {
				pname = fmt.Sprintf("%s%s", typeName(need), fieldName(needField.Name))
			}
			e = append(e, FieldDump{pname, o})
		}
		if need.Size {
			szName := typeName(need) + "Size"
			if typ == nil {
				e = append(e, FieldDump{szName, -1})
			} else {
				e = append(e, FieldDump{szName, int(typ.Size)})
			}
		}
	}
	return e

}

func typeName(need Need) string {
	n := need.Name
	if need.PrettyName != "" {
		n = need.PrettyName
	}
	n = strings.TrimSuffix(n, "_")
	n = strings.TrimSuffix(n, "__")
	n = strings.TrimPrefix(n, "__")
	n = strings.TrimPrefix(n, "_")
	parts := strings.Split(n, "_")
	for i := range parts {
		p1 := parts[i][:1]
		p2 := parts[i][1:]
		parts[i] = strings.ToUpper(p1) + p2
	}
	return strings.Join(parts, "")

}

func fieldName(field string) string {
	field = strings.TrimPrefix(field, "_")
	parts := strings.Split(field, "_")
	for i := range parts {
		p1 := parts[i][:1]
		p2 := parts[i][1:]
		parts[i] = strings.ToUpper(p1) + p2
	}
	return strings.Join(parts, "")
}

type Need struct {
	Name       string
	PrettyName string
	Fields     []NeedField
	Size       bool
}
type NeedField struct {
	Name      string
	PrintName string
}

type Version struct {
	Major, Minor, Patch int
}

func (p *Version) Compare(other Version) int {
	major := other.Major - p.Major
	if major != 0 {
		return major
	}

	minor := other.Minor - p.Minor
	if minor != 0 {
		return minor
	}
	return other.Patch - p.Patch
}

type Entry struct {
	SrcFile string
	Version Version
	Offsets []FieldDump
}
