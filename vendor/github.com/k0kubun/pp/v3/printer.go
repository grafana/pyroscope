// printer.go: The actual pretty print implementation. Everything in this file should be private.
package pp

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

const (
	indentWidth = 2
)

func (pp *PrettyPrinter) format(object interface{}) string {
	return newPrinter(object, &pp.currentScheme, pp.maxDepth, pp.coloringEnabled, pp.decimalUint, pp.exportedOnly, pp.thousandsSeparator).String()
}

func newPrinter(object interface{}, currentScheme *ColorScheme, maxDepth int, coloringEnabled bool, decimalUint bool, exportedOnly bool, thousandsSeparator bool) *printer {
	buffer := bytes.NewBufferString("")
	tw := new(tabwriter.Writer)
	tw.Init(buffer, indentWidth, 0, 1, ' ', 0)

	printer := &printer{
		Buffer:             buffer,
		tw:                 tw,
		depth:              0,
		maxDepth:           maxDepth,
		value:              reflect.ValueOf(object),
		visited:            map[uintptr]bool{},
		currentScheme:      currentScheme,
		coloringEnabled:    coloringEnabled,
		decimalUint:        decimalUint,
		exportedOnly:       exportedOnly,
		thousandsSeparator: thousandsSeparator,
	}

	if thousandsSeparator {
		printer.localizedPrinter = message.NewPrinter(language.English)
	}

	return printer
}

type printer struct {
	*bytes.Buffer
	tw                 *tabwriter.Writer
	depth              int
	maxDepth           int
	value              reflect.Value
	visited            map[uintptr]bool
	currentScheme      *ColorScheme
	coloringEnabled    bool
	decimalUint        bool
	exportedOnly       bool
	thousandsSeparator bool
	localizedPrinter   *message.Printer
}

func (p *printer) String() string {
	switch p.value.Kind() {
	case reflect.Bool:
		p.colorPrint(p.raw(), p.currentScheme.Bool)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Complex64, reflect.Complex128:
		p.colorPrint(p.raw(), p.currentScheme.Integer)
	case reflect.Float32, reflect.Float64:
		p.colorPrint(p.raw(), p.currentScheme.Float)
	case reflect.String:
		p.printString()
	case reflect.Map:
		p.printMap()
	case reflect.Struct:
		p.printStruct()
	case reflect.Array, reflect.Slice:
		p.printSlice()
	case reflect.Chan:
		p.printf("(%s)(%s)", p.typeString(), p.pointerAddr())
	case reflect.Interface:
		p.printInterface()
	case reflect.Ptr:
		p.printPtr()
	case reflect.Func:
		p.printf("%s {...}", p.typeString())
	case reflect.UnsafePointer:
		p.printf("%s(%s)", p.typeString(), p.pointerAddr())
	case reflect.Invalid:
		p.print(p.nil())
	default:
		p.print(p.raw())
	}

	p.tw.Flush()
	return p.Buffer.String()
}

func (p *printer) print(text string) {
	fmt.Fprint(p.tw, text)
}

func (p *printer) printf(format string, args ...interface{}) {
	text := fmt.Sprintf(format, args...)
	p.print(text)
}

func (p *printer) println(text string) {
	p.print(text + "\n")
}

func (p *printer) indentPrint(text string) {
	p.print(p.indent() + text)
}

func (p *printer) indentPrintf(format string, args ...interface{}) {
	text := fmt.Sprintf(format, args...)
	p.indentPrint(text)
}

func (p *printer) colorPrint(text string, color uint16) {
	p.print(p.colorize(text, color))
}

func (p *printer) printString() {
	quoted := strconv.Quote(p.value.String())
	quoted = quoted[1 : len(quoted)-1]

	p.colorPrint(`"`, p.currentScheme.StringQuotation)
	for len(quoted) > 0 {
		pos := strings.IndexByte(quoted, '\\')
		if pos == -1 {
			p.colorPrint(quoted, p.currentScheme.String)
			break
		}
		if pos != 0 {
			p.colorPrint(quoted[0:pos], p.currentScheme.String)
		}

		n := 1
		switch quoted[pos+1] {
		case 'x': // "\x00"
			n = 3
		case 'u': // "\u0000"
			n = 5
		case 'U': // "\U00000000"
			n = 9
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9': // "\000"
			n = 3
		}
		p.colorPrint(quoted[pos:pos+n+1], p.currentScheme.EscapedChar)
		quoted = quoted[pos+n+1:]
	}
	p.colorPrint(`"`, p.currentScheme.StringQuotation)
}

func (p *printer) printMap() {
	if p.value.Len() == 0 {
		p.printf("%s{}", p.typeString())
		return
	}

	if p.visited[p.value.Pointer()] {
		p.printf("%s{...}", p.typeString())
		return
	}
	p.visited[p.value.Pointer()] = true

	if PrintMapTypes {
		p.printf("%s{\n", p.typeString())
	} else {
		p.println("{")
	}
	p.indented(func() {
		value := sortMap(p.value)
		for i := 0; i < value.Len(); i++ {
			p.indentPrintf("%s:\t%s,\n", p.format(value.keys[i]), p.format(value.values[i]))
		}
	})
	p.indentPrint("}")
}

func (p *printer) printStruct() {
	if p.value.CanInterface() {
		if p.value.Type().String() == "time.Time" && p.value.Type().PkgPath() == "time" {
			p.printTime()
			return
		} else if p.value.Type().String() == "big.Int" {
			bigInt := p.value.Interface().(big.Int)
			p.print(p.colorize(bigInt.String(), p.currentScheme.Integer))
			return
		} else if p.value.Type().String() == "big.Float" {
			bigFloat := p.value.Interface().(big.Float)
			p.print(p.colorize(bigFloat.String(), p.currentScheme.Float))
			return
		}
	}

	var fields []int
	for i := 0; i < p.value.NumField(); i++ {
		field := p.value.Type().Field(i)
		value := p.value.Field(i)
		// ignore unexported if needed
		if p.exportedOnly && field.PkgPath != "" {
			continue
		}
		// ignore fields if zero value, or explicitly set
		if tag := field.Tag.Get("pp"); tag != "" {
			parts := strings.Split(tag, ",")
			if len(parts) == 2 && parts[1] == "omitempty" && valueIsZero(value) {
				continue
			}
			if parts[0] == "-" {
				continue
			}
		}
		fields = append(fields, i)
	}

	if len(fields) == 0 {
		p.print(p.typeString() + "{}")
		return
	}

	p.println(p.typeString() + "{")
	p.indented(func() {
		for _, i := range fields {
			field := p.value.Type().Field(i)
			value := p.value.Field(i)

			fieldName := field.Name
			if tag := field.Tag.Get("pp"); tag != "" {
				tagName := strings.Split(tag, ",")
				if tagName[0] != "" {
					fieldName = tagName[0]
				}
			}

			colorizedFieldName := p.colorize(fieldName, p.currentScheme.FieldName)
			p.indentPrintf("%s:\t%s,\n", colorizedFieldName, p.format(value))
		}
	})
	p.indentPrint("}")
}

func (p *printer) printTime() {
	tm := p.value.Interface().(time.Time)
	p.printf(
		"%s-%s-%s %s:%s:%s %s",
		p.colorize(strconv.Itoa(tm.Year()), p.currentScheme.Time),
		p.colorize(fmt.Sprintf("%02d", tm.Month()), p.currentScheme.Time),
		p.colorize(fmt.Sprintf("%02d", tm.Day()), p.currentScheme.Time),
		p.colorize(fmt.Sprintf("%02d", tm.Hour()), p.currentScheme.Time),
		p.colorize(fmt.Sprintf("%02d", tm.Minute()), p.currentScheme.Time),
		p.colorize(fmt.Sprintf("%02d", tm.Second()), p.currentScheme.Time),
		p.colorize(tm.Location().String(), p.currentScheme.Time),
	)
}

func (p *printer) printSlice() {
	if p.value.Kind() == reflect.Slice && p.value.IsNil() {
		p.printf("%s(%s)", p.typeString(), p.nil())
		return
	}
	if p.value.Len() == 0 {
		p.printf("%s{}", p.typeString())
		return
	}

	if p.value.Kind() == reflect.Slice {
		if p.visited[p.value.Pointer()] {
			// Stop travarsing cyclic reference
			p.printf("%s{...}", p.typeString())
			return
		}
		p.visited[p.value.Pointer()] = true
	}

	// Fold a large buffer
	if p.value.Len() > BufferFoldThreshold {
		p.printf("%s{...}", p.typeString())
		return
	}

	p.println(p.typeString() + "{")
	p.indented(func() {
		groupsize := 0
		switch p.value.Type().Elem().Kind() {
		case reflect.Uint8:
			groupsize = 16
		case reflect.Uint16:
			groupsize = 8
		case reflect.Uint32:
			groupsize = 8
		case reflect.Uint64:
			groupsize = 4
		}

		if groupsize > 0 {
			for i := 0; i < p.value.Len(); i++ {
				// indent for new group
				if i%groupsize == 0 {
					p.print(p.indent())
				}
				// slice element
				p.printf("%s,", p.format(p.value.Index(i)))
				// space or newline
				if (i+1)%groupsize == 0 || i+1 == p.value.Len() {
					p.print("\n")
				} else {
					p.print(" ")
				}
			}
		} else {
			for i := 0; i < p.value.Len(); i++ {
				p.indentPrintf("%s,\n", p.format(p.value.Index(i)))
			}
		}
	})
	p.indentPrint("}")
}

func (p *printer) printInterface() {
	e := p.value.Elem()
	if e.Kind() == reflect.Invalid {
		p.print(p.nil())
	} else if e.IsValid() {
		p.print(p.format(e))
	} else {
		p.printf("%s(%s)", p.typeString(), p.nil())
	}
}

func (p *printer) printPtr() {
	if p.visited[p.value.Pointer()] {
		p.printf("&%s{...}", p.elemTypeString())
		return
	}
	if p.value.Pointer() != 0 {
		p.visited[p.value.Pointer()] = true
	}

	if p.value.Elem().IsValid() {
		p.printf("&%s", p.format(p.value.Elem()))
	} else {
		p.printf("(%s)(%s)", p.typeString(), p.nil())
	}
}

func (p *printer) pointerAddr() string {
	return p.colorize(fmt.Sprintf("%#v", p.value.Pointer()), p.currentScheme.PointerAdress)
}

func (p *printer) typeString() string {
	return p.colorizeType(p.value.Type().String())
}

func (p *printer) elemTypeString() string {
	return p.colorizeType(p.value.Elem().Type().String())
}

func (p *printer) colorizeType(t string) string {
	prefix := ""

	if p.matchRegexp(t, `^\[\].+$`) {
		prefix = "[]"
		t = t[2:]
	}

	if p.matchRegexp(t, `^\[\d+\].+$`) {
		num := regexp.MustCompile(`\d+`).FindString(t)
		prefix = fmt.Sprintf("[%s]", p.colorize(num, p.currentScheme.ObjectLength))
		t = t[2+len(num):]
	}

	if p.matchRegexp(t, `^[^\.]+\.[^\.]+$`) {
		ts := strings.Split(t, ".")
		t = fmt.Sprintf("%s.%s", ts[0], p.colorize(ts[1], p.currentScheme.StructName))
	} else {
		t = p.colorize(t, p.currentScheme.StructName)
	}
	return prefix + t
}

func (p *printer) matchRegexp(text, exp string) bool {
	return regexp.MustCompile(exp).MatchString(text)
}

func (p *printer) indented(proc func()) {
	p.depth++
	if p.maxDepth == -1 || p.depth <= p.maxDepth {
		proc()
	}
	p.depth--
}

func (p *printer) fmtOrLocalizedSprintf(format string, a ...interface{}) string {
	if p.localizedPrinter == nil {
		return fmt.Sprintf(format, a...)
	}

	return p.localizedPrinter.Sprintf(format, a...)
}

func (p *printer) raw() string {
	// Some value causes panic when Interface() is called.
	switch p.value.Kind() {
	case reflect.Bool:
		return fmt.Sprintf("%#v", p.value.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return p.fmtOrLocalizedSprintf("%v", p.value.Int())
	case reflect.Uint, reflect.Uintptr:
		if p.decimalUint {
			return p.fmtOrLocalizedSprintf("%d", p.value.Uint())
		} else {
			return fmt.Sprintf("%#v", p.value.Uint())
		}
	case reflect.Uint8:
		if p.decimalUint {
			return fmt.Sprintf("%d", p.value.Uint())
		} else {
			return fmt.Sprintf("0x%02x", p.value.Uint())
		}
	case reflect.Uint16:
		if p.decimalUint {
			return p.fmtOrLocalizedSprintf("%d", p.value.Uint())
		} else {
			return fmt.Sprintf("0x%04x", p.value.Uint())
		}
	case reflect.Uint32:
		if p.decimalUint {
			return p.fmtOrLocalizedSprintf("%d", p.value.Uint())
		} else {
			return fmt.Sprintf("0x%08x", p.value.Uint())
		}
	case reflect.Uint64:
		if p.decimalUint {
			return p.fmtOrLocalizedSprintf("%d", p.value.Uint())
		} else {
			return fmt.Sprintf("0x%016x", p.value.Uint())
		}
	case reflect.Float32, reflect.Float64:
		return p.fmtOrLocalizedSprintf("%f", p.value.Float())
	case reflect.Complex64, reflect.Complex128:
		return fmt.Sprintf("%#v", p.value.Complex())
	default:
		return fmt.Sprintf("%#v", p.value.Interface())
	}
}

func (p *printer) nil() string {
	return p.colorize("nil", p.currentScheme.Nil)
}

func (p *printer) colorize(text string, color uint16) string {
	if ColoringEnabled && p.coloringEnabled {
		return colorizeText(text, color)
	} else {
		return text
	}
}

func (p *printer) format(object interface{}) string {
	pp := newPrinter(object, p.currentScheme, p.maxDepth, p.coloringEnabled, p.decimalUint, p.exportedOnly, p.thousandsSeparator)
	pp.depth = p.depth
	pp.visited = p.visited
	if value, ok := object.(reflect.Value); ok {
		pp.value = value
	}
	return pp.String()
}

func (p *printer) indent() string {
	return strings.Repeat("\t", p.depth)
}

// valueIsZero reports whether v is the zero value for its type.
// It returns false if the argument is invalid.
// This is a copy paste of reflect#IsZero from go1.15. It is not present before go1.13 (source: https://golang.org/doc/go1.13#library)
// source: https://golang.org/src/reflect/value.go?s=34297:34325#L1090
// This will need to be updated for new types or the decision should be made to drop support for Go version pre go1.13
func valueIsZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return math.Float64bits(v.Float()) == 0
	case reflect.Complex64, reflect.Complex128:
		c := v.Complex()
		return math.Float64bits(real(c)) == 0 && math.Float64bits(imag(c)) == 0
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !valueIsZero(v.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return v.IsNil()
	case reflect.String:
		return v.Len() == 0
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !valueIsZero(v.Field(i)) {
				return false
			}
		}
		return true
	default:
		// this is the only difference between stdlib reflect#IsZero and this function. We're not going to
		// panic on the default cause, even
		return false
	}
}
