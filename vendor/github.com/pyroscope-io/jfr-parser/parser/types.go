package parser

import (
	"errors"
	"fmt"

	"github.com/pyroscope-io/jfr-parser/reader"
)

var parseBaseTypeAndDrops = map[string]func(reader.Reader) error{
	"boolean": func(r reader.Reader) (err error) {
		_, err = toBoolean(r)
		return
	},
	"byte": func(r reader.Reader) (err error) {
		_, err = toByte(r)
		return
	},
	"double": func(r reader.Reader) (err error) {
		_, err = toDouble(r)
		return
	},
	"float": func(r reader.Reader) (err error) {
		_, err = toFloat(r)
		return
	},
	"int": func(r reader.Reader) (err error) {
		_, err = toInt(r)
		return
	},
	"long": func(r reader.Reader) (err error) {
		_, err = toLong(r)
		return
	},
	"short": func(r reader.Reader) (err error) {
		_, err = toShort(r)
		return
	},
	"java.lang.String": func(r reader.Reader) (err error) {
		_, err = toString(r)
		return
	},
}

var types = map[string]func() ParseResolvable{
	"java.lang.Class":                func() ParseResolvable { return new(Class) },
	"java.lang.Thread":               func() ParseResolvable { return new(Thread) },
	"jdk.types.ClassLoader":          func() ParseResolvable { return new(ClassLoader) },
	"jdk.types.CodeBlobType":         func() ParseResolvable { return new(CodeBlobType) },
	"jdk.types.FlagValueOrigin":      func() ParseResolvable { return new(FlagValueOrigin) },
	"jdk.types.FrameType":            func() ParseResolvable { return new(FrameType) },
	"jdk.types.G1YCType":             func() ParseResolvable { return new(G1YCType) },
	"jdk.types.GCName":               func() ParseResolvable { return new(GCName) },
	"jdk.types.Method":               func() ParseResolvable { return new(Method) },
	"jdk.types.Module":               func() ParseResolvable { return new(Module) },
	"jdk.types.NarrowOopMode":        func() ParseResolvable { return new(NarrowOopMode) },
	"jdk.types.NetworkInterfaceName": func() ParseResolvable { return new(NetworkInterfaceName) },
	"jdk.types.Package":              func() ParseResolvable { return new(Package) },
	"jdk.types.StackTrace":           func() ParseResolvable { return new(StackTrace) },
	"jdk.types.Symbol":               func() ParseResolvable { return new(Symbol) },
	"jdk.types.ThreadState":          func() ParseResolvable { return new(ThreadState) },
}

func ParseClass(r reader.Reader, classes ClassMap, cpools PoolMap, classID int64) (ParseResolvable, error) {
	class, ok := classes[int(classID)]
	if !ok {
		return nil, fmt.Errorf("unexpected class %d", classID)
	}
	v := class.typeFn()
	if err := v.Parse(r, classes, cpools, class); err != nil {
		return nil, err
	}
	return v, nil
}

type Parseable interface {
	Parse(reader.Reader, ClassMap, PoolMap, *ClassMetadata) error
}

type Resolvable interface {
	Resolve(ClassMap, PoolMap) error
}

type ParseResolvable interface {
	Parseable
	Resolvable
}

type constant struct {
	classID int64
	field   string
	index   int64
}

func appendConstant(r reader.Reader, constants *[]constant, name string, class int64) error {
	i, err := r.VarLong()
	if err != nil {
		return fmt.Errorf("unable to read constant index")
	}
	*constants = append(*constants, constant{field: name, index: i, classID: class})
	return nil
}

func parseFields(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata, constants *[]constant, resolved bool, cb func(reader.Reader, string, ParseResolvable) error) error {
	if constants != nil && cap(*constants) == 0 && class.numConstants != 0 && !resolved {
		*constants = make([]constant, 0, class.numConstants)
	}
	for i := range class.Fields {
		f := &class.Fields[i]
		if f.ConstantPool {
			if constants != nil && !resolved {
				if err := appendConstant(r, constants, f.Name, f.Class); err != nil {
					return fmt.Errorf("failed to parse %s: unable to append constant: %w", class.Name, err)
				}
			} else {
				cpool, ok := cpools[int(f.Class)]
				if !ok {
					return fmt.Errorf("unknown constant pool class %d", f.Class)
				}
				i, err := r.VarLong()
				if err != nil {
					return fmt.Errorf("unable to read constant index")
				}
				p, ok := cpool.Pool[int(i)]
				if !ok {
					continue
				}
				if err := cb(r, f.Name, p); err != nil {
					return fmt.Errorf("unable to parse constant field %s: %w", f.Name, err)
				}
			}
		} else if f.Dimension == 1 {
			offset := r.Offset()
			err := cb(r, "len", nil)
			if err != nil {
				return fmt.Errorf("failed to parse %s: unable to read array length: %w", class.Name, err)
			}
			_, _ = r.SeekStart(int64(offset))
			n, _ := r.VarInt() // this must successfult
			// TODO: assert n is small enough
			for i := 0; i < int(n); i++ {
				var p ParseResolvable
				oldOffset := r.Offset()
				if !f.isBaseType {
					p, err = ParseClass(r, classes, cpools, f.Class)
					if err != nil {
						return fmt.Errorf("failed to parse %s: unable to read an array element: %w", class.Name, err)
					}
				}
				err = cb(r, f.Name, p)
				newOffset := r.Offset()
				if newOffset == oldOffset {
					err = f.parseBaseTypeAndDrop(r)
				}
				if err != nil {
					return fmt.Errorf("failed to parse %s: unable to parse an array element: %w", class.Name, err)
				}
			}
		} else {
			var (
				p   ParseResolvable
				err error
			)
			oldOffset := r.Offset()
			if !f.isBaseType {
				p, err = ParseClass(r, classes, cpools, f.Class)
				if err != nil {
					return fmt.Errorf("failed to parse %s: unable to read an array element: %w", class.Name, err)
				}
			}
			err = cb(r, f.Name, p)
			newOffset := r.Offset()
			if newOffset == oldOffset {
				err = f.parseBaseTypeAndDrop(r)
			}
			if err != nil {
				return fmt.Errorf("failed to parse %s: unable to parse a field: %w", class.Name, err)
			}
		}
	}
	return nil
}

func resolveConstants(classes ClassMap, cpools PoolMap, constants *[]constant, resolved *bool, cb func(reader.Reader, string, ParseResolvable) error) error {
	if *resolved {
		return nil
	}
	*resolved = true
	for _, c := range *constants {
		if err := ResolveConstants(classes, cpools, int(c.classID)); err != nil {
			return fmt.Errorf("unable to resolve contants: %w", err)
		}
		p, ok := cpools[int(c.classID)]
		if !ok {
			// Non-existent constant pool references seem to be used to mark no value
			continue
		}
		it, ok := p.Pool[int(c.index)]
		if !ok {
			// Non-existent constant pool references seem to be used to mark no value
			continue
		}
		if err := it.Resolve(classes, cpools); err != nil {
			return err
		}
		if err := cb(nil, c.field, it); err != nil {
			return fmt.Errorf("unable to resolve constants for field %s: %w", c.field, err)
		}
	}
	*constants = nil
	return nil
}

func toBoolean(r reader.Reader) (bool, error) {
	return r.Boolean()
}

func toByte(r reader.Reader) (int8, error) {
	return r.Byte()
}

func toDouble(r reader.Reader) (float64, error) {
	return r.Double()
}

func toFloat(r reader.Reader) (float32, error) {
	return r.Float()
}

func toInt(r reader.Reader) (int32, error) {
	return r.VarInt()
}

func toLong(r reader.Reader) (int64, error) {
	return r.VarLong()
}

func toShort(r reader.Reader) (int16, error) {
	return r.VarShort()
}

func toString(r reader.Reader) (string, error) {
	return r.String()
}

type Class struct {
	ClassLoader *ClassLoader
	Name        *Symbol
	Package     *Package
	Modifiers   int64
	constants   []constant
	resolved    bool
}

func (c *Class) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "classLoader":
		c.ClassLoader, err = toClassLoader(p)
	case "name":
		c.Name, err = toSymbol(p)
	case "package":
		c.Package, err = toPackage(p)
	case "modifers":
		c.Modifiers, err = toLong(r)
	}
	return err
}

func (c *Class) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &c.constants, c.resolved, c.parseField)
}

func (c *Class) Resolve(classes ClassMap, cpools PoolMap) error {
	if err := resolveConstants(classes, cpools, &c.constants, &c.resolved, c.parseField); err != nil {
		return err
	}
	if c.ClassLoader != nil {
		if err := c.ClassLoader.Resolve(classes, cpools); err != nil {
			return err
		}
	}
	if c.Package != nil {
		return c.Package.Resolve(classes, cpools)
	}
	return nil
}

func toClass(p ParseResolvable) (*Class, error) {
	c, ok := p.(*Class)
	if !ok {
		// TODO
		return nil, errors.New("")
	}
	return c, nil
}

type Thread struct {
	OsName       string
	OsThreadID   int64
	JavaName     string
	JavaThreadID int64
	constants    []constant
	resolved     bool
}

func (t *Thread) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "osName":
		t.OsName, err = toString(r)
	case "osThreadId":
		t.OsThreadID, err = toLong(r)
	case "javaName":
		t.JavaName, err = toString(r)
	case "javaThreadId":
		t.JavaThreadID, err = toLong(r)
	}
	return err
}

func (t *Thread) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &t.constants, t.resolved, t.parseField)
}

func (t *Thread) Resolve(classes ClassMap, cpools PoolMap) error {
	return resolveConstants(classes, cpools, &t.constants, &t.resolved, t.parseField)
}

func toThread(p ParseResolvable) (*Thread, error) {
	t, ok := p.(*Thread)
	if !ok {
		return nil, errors.New("not a Thread")
	}
	return t, nil
}

type ClassLoader struct {
	Type      *Class
	Name      *Symbol
	constants []constant
	resolved  bool
}

func (cl *ClassLoader) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "type":
		cl.Type, err = toClass(p)
	case "name":
		cl.Name, err = toSymbol(p)
	}
	return err
}

func (cl *ClassLoader) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &cl.constants, cl.resolved, cl.parseField)
}

func (cl *ClassLoader) Resolve(classes ClassMap, cpools PoolMap) error {
	if err := resolveConstants(classes, cpools, &cl.constants, &cl.resolved, cl.parseField); err != nil {
		return err
	}
	if cl.Type != nil {
		if err := cl.Type.Resolve(classes, cpools); err != nil {
			return err
		}
	}
	return nil
}

func toClassLoader(p ParseResolvable) (*ClassLoader, error) {
	c, ok := p.(*ClassLoader)
	if !ok {
		// TODO
		return nil, errors.New("")
	}
	return c, nil
}

type CodeBlobType struct {
	String    string
	constants []constant
	resolved  bool
}

func (cbt *CodeBlobType) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "string":
		cbt.String, err = toString(r)
	}
	return err
}

func (cbt *CodeBlobType) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &cbt.constants, cbt.resolved, cbt.parseField)
}

func (cbt *CodeBlobType) Resolve(classes ClassMap, cpools PoolMap) error {
	return nil
}

func toCodeBlobType(p ParseResolvable) (*CodeBlobType, error) {
	cbt, ok := p.(*CodeBlobType)
	if !ok {
		return nil, errors.New("not a CodeBlobType")
	}
	return cbt, nil
}

type FlagValueOrigin struct {
	String    string
	constants []constant
	resolved  bool
}

func (fvo *FlagValueOrigin) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "description":
		fvo.String, err = toString(r)
	}
	return err
}

func (fvo *FlagValueOrigin) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &fvo.constants, fvo.resolved, fvo.parseField)
}

func (fvo *FlagValueOrigin) Resolve(classes ClassMap, cpools PoolMap) error {
	return resolveConstants(classes, cpools, &fvo.constants, &fvo.resolved, fvo.parseField)
}

func toFlagValueOrigin(p Parseable) (*FlagValueOrigin, error) {
	fvo, ok := p.(*FlagValueOrigin)
	if !ok {
		return nil, errors.New("not a FlagValueOrigin")
	}
	return fvo, nil
}

type FrameType struct {
	Description string
	constants   []constant
	resolved    bool
}

func (ft *FrameType) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "description":
		ft.Description, err = toString(r)
	}
	return err
}

func (ft *FrameType) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &ft.constants, ft.resolved, ft.parseField)
}

func (ft *FrameType) Resolve(classes ClassMap, cpools PoolMap) error {
	return nil
}

func toFrameType(p Parseable) (*FrameType, error) {
	ft, ok := p.(*FrameType)
	if !ok {
		return nil, errors.New("not a FrameType")
	}
	return ft, nil
}

type G1YCType struct {
	String    string
	constants []constant
	resolved  bool
}

func (gyt *G1YCType) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "string":
		gyt.String, err = toString(r)
	}
	return err
}

func (gyt *G1YCType) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &gyt.constants, gyt.resolved, gyt.parseField)
}

func (gyt *G1YCType) Resolve(classes ClassMap, cpools PoolMap) error {
	return nil
}

func toG1YCType(p Parseable) (*G1YCType, error) {
	gyt, ok := p.(*G1YCType)
	if !ok {
		return nil, errors.New("not a G1YCType")
	}
	return gyt, nil
}

type GCName struct {
	String    string
	constants []constant
	resolved  bool
}

func (gn *GCName) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "string":
		gn.String, err = toString(r)
	}
	return err
}

func (gn *GCName) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &gn.constants, gn.resolved, gn.parseField)
}

func (gn *GCName) Resolve(classes ClassMap, cpools PoolMap) error {
	return nil
}

func toGCName(p Parseable) (*GCName, error) {
	gn, ok := p.(*GCName)
	if !ok {
		return nil, errors.New("not a GCName")
	}
	return gn, nil
}

type Method struct {
	Type       *Class
	Name       *Symbol
	Descriptor *Symbol
	Modifiers  int32
	Hidden     bool
	constants  []constant
	resolved   bool
}

func (m *Method) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "type":
		m.Type, err = toClass(p)
	case "name":
		m.Name, err = toSymbol(p)
	case "descriptor":
		m.Descriptor, err = toSymbol(p)
	case "modifiers":
		m.Modifiers, err = toInt(r)
	case "hidden":
		m.Hidden, err = toBoolean(r)
	}
	return err
}

func (m *Method) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &m.constants, m.resolved, m.parseField)
}

func (m *Method) Resolve(classes ClassMap, cpools PoolMap) error {
	if err := resolveConstants(classes, cpools, &m.constants, &m.resolved, m.parseField); err != nil {
		return err
	}
	if m.Type != nil {
		if err := m.Type.Resolve(classes, cpools); err != nil {
			return err
		}
	}
	return nil
}

func toMethod(p ParseResolvable) (*Method, error) {
	m, ok := p.(*Method)
	if !ok {
		return nil, errors.New("not a Method")
	}
	return m, nil
}

type Module struct {
	Name        *Symbol
	Version     *Symbol
	Location    *Symbol
	ClassLoader *ClassLoader
	constants   []constant
	resolved    bool
}

func (m *Module) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "name":
		m.Name, err = toSymbol(p)
	case "version":
		m.Version, err = toSymbol(p)
	case "location":
		m.Location, err = toSymbol(p)
	case "classLoader":
		m.ClassLoader, err = toClassLoader(p)
	}
	return err
}

func (m *Module) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &m.constants, m.resolved, m.parseField)
}

func (m *Module) Resolve(classes ClassMap, cpools PoolMap) error {
	if err := resolveConstants(classes, cpools, &m.constants, &m.resolved, m.parseField); err != nil {
		return err
	}
	if m.ClassLoader != nil {
		return m.ClassLoader.Resolve(classes, cpools)
	}
	return nil
}

func toModule(p ParseResolvable) (*Module, error) {
	m, ok := p.(*Module)
	if !ok {
		return nil, errors.New("not a Module")
	}
	return m, nil
}

type NarrowOopMode struct {
	String    string
	constants []constant
	resolved  bool
}

func (nom *NarrowOopMode) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "string":
		nom.String, err = toString(r)
	}
	return err
}

func (nom *NarrowOopMode) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &nom.constants, nom.resolved, nom.parseField)
}

func (nom *NarrowOopMode) Resolve(classes ClassMap, cpools PoolMap) error {
	return resolveConstants(classes, cpools, &nom.constants, &nom.resolved, nom.parseField)
}

func toNarrowOopMode(p Parseable) (*NarrowOopMode, error) {
	nom, ok := p.(*NarrowOopMode)
	if !ok {
		return nil, errors.New("not a NarrowOopMode")
	}
	return nom, nil
}

type NetworkInterfaceName struct {
	NetworkInterface string
	constants        []constant
	resolved         bool
}

func (nim *NetworkInterfaceName) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "networkInterface":
		nim.NetworkInterface, err = toString(r)
	}
	return err
}

func (nim *NetworkInterfaceName) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &nim.constants, nim.resolved, nim.parseField)
}

func (nim *NetworkInterfaceName) Resolve(classes ClassMap, cpools PoolMap) error {
	return resolveConstants(classes, cpools, &nim.constants, &nim.resolved, nim.parseField)
}

func toNetworkInterfaceName(p Parseable) (*NetworkInterfaceName, error) {
	nim, ok := p.(*NetworkInterfaceName)
	if !ok {
		return nil, errors.New("not a NetworkInterfaceName")
	}
	return nim, nil
}

type Package struct {
	Name      *Symbol
	constants []constant
	resolved  bool
}

func (pkg *Package) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "name":
		pkg.Name, err = toSymbol(p)
	}
	return err
}

func (p *Package) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &p.constants, p.resolved, p.parseField)
}

func (p *Package) Resolve(classes ClassMap, cpools PoolMap) error {
	if err := resolveConstants(classes, cpools, &p.constants, &p.resolved, p.parseField); err != nil {
		return err
	}
	if p.Name != nil {
		return p.Name.Resolve(classes, cpools)
	}
	return nil
}

func toPackage(p ParseResolvable) (*Package, error) {
	pkg, ok := p.(*Package)
	if !ok {
		// TODO
		return nil, errors.New("")
	}
	return pkg, nil
}

type StackFrame struct {
	Method        *Method
	LineNumber    int32
	ByteCodeIndex int32
	Type          *FrameType
	constants     []constant
	resolved      bool
}

func (sf *StackFrame) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "method":
		sf.Method, err = toMethod(p)
	case "lineNumber":
		sf.LineNumber, err = toInt(r)
	case "byteCodeIndex":
		sf.ByteCodeIndex, err = toInt(r)
	case "type":
		sf.Type, err = toFrameType(p)
	}
	return err
}

func (sf *StackFrame) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &sf.constants, sf.resolved, sf.parseField)
}

func (sf *StackFrame) Resolve(classes ClassMap, cpools PoolMap) error {
	if err := resolveConstants(classes, cpools, &sf.constants, &sf.resolved, sf.parseField); err != nil {
		return err
	}
	if sf.Method != nil {
		if err := sf.Method.Resolve(classes, cpools); err != nil {
			return err
		}
	}
	return nil
}

func toStackFrame(p ParseResolvable) (*StackFrame, error) {
	sf, ok := p.(*StackFrame)
	if !ok {
		return nil, errors.New("not a StackFrame")
	}
	return sf, nil
}

type StackTrace struct {
	Truncated               bool
	Frames                  []*StackFrame
	constants               []constant
	resolved                bool
	getPointerToStackFrames func(int) []*StackFrame
}

func (st *StackTrace) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "truncated":
		st.Truncated, err = toBoolean(r)
	case "frames":
		var sf *StackFrame
		sf, err := toStackFrame(p)
		if err != nil {
			return err
		}
		st.Frames = append(st.Frames, sf)
	case "len":
		frameLen, err := r.VarInt()
		if err != nil {
			return err
		}
		st.Frames = st.getPointerToStackFrames(int(frameLen))
	}
	return err
}

func (st *StackTrace) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &st.constants, st.resolved, st.parseField)
}

func (st *StackTrace) Resolve(classes ClassMap, cpools PoolMap) error {
	if err := resolveConstants(classes, cpools, &st.constants, &st.resolved, st.parseField); err != nil {
		return err
	}
	for _, f := range st.Frames {
		if err := f.Resolve(classes, cpools); err != nil {
			return err
		}
	}
	return nil
}

func toStackTrace(p ParseResolvable) (*StackTrace, error) {
	st, ok := p.(*StackTrace)
	if !ok {
		return nil, errors.New("not a StackTrace")
	}
	return st, nil
}

type Symbol struct {
	String    string
	constants []constant
	resolved  bool
}

func (s *Symbol) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "string":
		s.String, err = toString(r)
	}
	return err
}

func (s *Symbol) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &s.constants, s.resolved, s.parseField)
}

func (s *Symbol) Resolve(classes ClassMap, cpools PoolMap) error {
	return nil
}

func toSymbol(p ParseResolvable) (*Symbol, error) {
	s, ok := p.(*Symbol)
	if !ok {
		// TODO
		return nil, errors.New("")
	}
	return s, nil
}

type ThreadState struct {
	Name      string
	constants []constant
	resolved  bool
}

func (ts *ThreadState) parseField(r reader.Reader, name string, p ParseResolvable) (err error) {
	switch name {
	case "name":
		ts.Name, err = toString(r)
	}
	return err
}

func (ts *ThreadState) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &ts.constants, ts.resolved, ts.parseField)
}

func (ts *ThreadState) Resolve(classes ClassMap, cpools PoolMap) error {
	return nil
}

func toThreadState(p ParseResolvable) (*ThreadState, error) {
	ts, ok := p.(*ThreadState)
	if !ok {
		return nil, errors.New("not a ThreadState")
	}
	return ts, nil
}

// UnsupportedType represents any type that is not supported by the parser.
// This will allow to still read the unsupported type instead of returning an error.
type UnsupportedType struct {
	constants []constant
	resolved  bool
}

func (ut *UnsupportedType) parseField(r reader.Reader, name string, p ParseResolvable) error {
	return nil
}

func (ut *UnsupportedType) Parse(r reader.Reader, classes ClassMap, cpools PoolMap, class *ClassMetadata) error {
	return parseFields(r, classes, cpools, class, &ut.constants, ut.resolved, ut.parseField)
}

func (ut *UnsupportedType) Resolve(classes ClassMap, cpools PoolMap) error {
	return resolveConstants(classes, cpools, &ut.constants, &ut.resolved, ut.parseField)
}
