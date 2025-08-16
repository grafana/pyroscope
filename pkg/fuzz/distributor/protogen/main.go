package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"time"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/validation"
)

//todo remove this garbage
func main() {
	var buf bytes.Buffer
	var conversionBuf bytes.Buffer

	buf.WriteString("syntax = \"proto3\";\n\n")
	buf.WriteString("package fuzz;\n\n")
	buf.WriteString("import \"google/v1/profile.proto\";\n")
	buf.WriteString("import \"types/v1/types.proto\";\n")
	buf.WriteString("import \"settings/v1/recording_rules.proto\";\n")
	buf.WriteString("import \"settings/v1/setting.proto\";\n\n")

	todo := make(map[reflect.Type]struct{})
	processed := make(map[reflect.Type]struct{})
	todo[reflect.TypeOf(validation.Limits{})] = struct{}{}

	// Initialize conversion file
	conversionBuf.WriteString("package common\n\n")
	conversionBuf.WriteString("import (\n")
	conversionBuf.WriteString("\"time\"\n")
	conversionBuf.WriteString("\n")
	conversionBuf.WriteString("\"github.com/grafana/pyroscope/api/gen/proto/go/fuzz\"\n")
	conversionBuf.WriteString("\"github.com/grafana/pyroscope/pkg/distributor/ingestlimits\"\n")
	conversionBuf.WriteString("\"github.com/grafana/pyroscope/pkg/distributor/sampling\"\n")
	conversionBuf.WriteString("\"github.com/grafana/pyroscope/pkg/validation\"\n")
	conversionBuf.WriteString("\"github.com/prometheus/common/model\"\n")
	conversionBuf.WriteString("\"github.com/prometheus/prometheus/model/relabel\"\n")
	conversionBuf.WriteString(")\n\n")

	// Skip types that are already defined in imported protos
	skipTypes := map[reflect.Type]struct{}{
		reflect.TypeOf(&settingsv1.RecordingRule{}):                {},
		reflect.TypeOf(settingsv1.RecordingRule{}):                 {},
		reflect.TypeOf(&settingsv1.StacktraceFilter{}):             {},
		reflect.TypeOf(settingsv1.StacktraceFilter{}):              {},
		reflect.TypeOf(&settingsv1.StacktraceFilterFunctionName{}): {},
		reflect.TypeOf(settingsv1.StacktraceFilterFunctionName{}):  {},
		reflect.TypeOf(&typesv1.LabelPair{}):                       {},
		reflect.TypeOf(typesv1.LabelPair{}):                        {},
	}

	// Store messages to sort them later
	type message struct {
		name    string
		content string
		goType  reflect.Type
	}
	var messages []message

	for len(todo) > 0 {
		var first reflect.Type
		for k, _ := range todo {
			first = k
			break
		}
		delete(todo, first)

		// Skip if already processed
		if _, ok := processed[first]; ok {
			continue
		}
		// Skip types that are already defined in imported protos
		if _, ok := skipTypes[first]; ok {
			processed[first] = struct{}{}
			continue
		}
		processed[first] = struct{}{}

		var msgBuf bytes.Buffer
		messageName := type2ProtoType(first)
		msgBuf.WriteString(fmt.Sprintf("message %s {\n", messageName))

		// Special case for UsageGroupConfig - make it a simple map<string, string>
		if strings.HasSuffix(messageName, "UsageGroupConfig") {
			msgBuf.WriteString("  map<string, string> config = 1;\n")
		} else {
			fields := reflect.VisibleFields(first)
			fieldNo := new(int)
			*fieldNo = 1

			for _, f := range fields {
				pfield(&msgBuf, "", f, fieldNo, todo)
			}
		}
		msgBuf.WriteString("}\n")

		messages = append(messages, message{name: messageName, content: msgBuf.String(), goType: first})
	}

	// Sort messages by name
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].name < messages[j].name
	})

	// Write sorted messages
	for _, msg := range messages {
		buf.WriteString(msg.content)
		buf.WriteString("\n")
	}

	output := buf.String()

	fmt.Print(output)

	err := os.WriteFile("/Users/grafana/pyroscope/frontend-vibe-coding/api/fuzz/types.proto", []byte(output), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing to proto file: %v\n", err)
		os.Exit(1)
	}

	// Generate conversion functions
	for _, msg := range messages {
		generateConversionFunction(&conversionBuf, msg.name, msg.goType)
	}

	// Write conversion functions to file
	err = os.WriteFile("/Users/grafana/pyroscope/frontend-vibe-coding/pkg/fuzz/common/common.go", []byte(conversionBuf.String()), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing to conversion file: %v\n", err)
		os.Exit(1)
	}

	// Run go fmt on the generated file
	cmd := exec.Command("go", "fmt", "/Users/grafana/pyroscope/frontend-vibe-coding/pkg/fuzz/common/common.go")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting generated file: %v\n", err)
	}
}

func shouldSkipType(t reflect.Type) bool {
	// Check if this is one of the types we're importing
	if t.PkgPath() == "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1" ||
		t.PkgPath() == "github.com/grafana/pyroscope/api/gen/proto/go/types/v1" {
		return true
	}
	return false
}

func pfield(buf *bytes.Buffer, prefix string, f reflect.StructField, fieldNo *int, todo map[reflect.Type]struct{}) {
	//buf.WriteString(fmt.Sprintf("  //  %s %s \n", f.Name, f.Type.String()))
	if !f.IsExported() {
		buf.WriteString(fmt.Sprintf("  // not exported %s %s\n", f.Name, f.Type.String()))
		return
	}

	// Special case for time.Time - treat as uint64
	if f.Type == reflect.TypeOf(time.Time{}) {
		fieldName := prefix + f.Name
		buf.WriteString(fmt.Sprintf("  uint64 %s = %d;\n", fieldName, *fieldNo))
		*fieldNo++
		return
	}

	// Special case for relabel.Regexp - treat as string
	if f.Type.String() == "relabel.Regexp" {
		fieldName := prefix + f.Name
		buf.WriteString(fmt.Sprintf("  string %s = %d;\n", fieldName, *fieldNo))
		*fieldNo++
		return
	}

	// Special case for UsageGroupConfig types - treat as map<string, string>
	if strings.HasSuffix(f.Type.String(), "UsageGroupConfig") {
		fieldName := prefix + f.Name
		buf.WriteString(fmt.Sprintf("  map<string, string> %s = %d;\n", fieldName, *fieldNo))
		*fieldNo++
		return
	}

	kind := f.Type.Kind()

	switch kind {
	case reflect.Struct:
		nestedFields := reflect.VisibleFields(f.Type)
		fieldPrefix := prefix + type2ProtoType(f.Type) + "__"
		for _, nestedField := range nestedFields {
			pfield(buf, fieldPrefix, nestedField, fieldNo, todo)
		}
	case reflect.Map:
		keyType := f.Type.Key()
		valueType := f.Type.Elem()
		fieldName := prefix + f.Name
		protoKeyType := type2ProtoType(keyType)
		protoValueType := type2ProtoType(valueType)
		if valueType.Kind() == reflect.Pointer {
			if !shouldSkipType(valueType.Elem()) {
				todo[valueType.Elem()] = struct{}{}
			}
		} else if valueType.Kind() == reflect.Struct {
			if !shouldSkipType(valueType) {
				todo[valueType] = struct{}{}
			}
		}
		buf.WriteString(fmt.Sprintf("  map<%s, %s> %s = %d;\n", protoKeyType, protoValueType, fieldName, *fieldNo))
		*fieldNo++
	case reflect.Pointer, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.String, reflect.Slice:
		repeated := ""
		if kind == reflect.Slice {
			repeated = "repeated "
			// If this is a slice, add the element type to todo
			elemType := f.Type.Elem()
			if elemType.Kind() == reflect.Pointer {
				if !shouldSkipType(elemType.Elem()) {
					todo[elemType.Elem()] = struct{}{}
				}
			} else if elemType.Kind() == reflect.Struct {
				if !shouldSkipType(elemType) {
					todo[elemType] = struct{}{}
				}
			}
		}
		if kind == reflect.Pointer {
			if !shouldSkipType(f.Type.Elem()) {
				todo[f.Type.Elem()] = struct{}{}
			}
		}
		fieldName := prefix + f.Name
		buf.WriteString(fmt.Sprintf("  %s%s %s = %d;\n", repeated, type2ProtoType(f.Type), fieldName, *fieldNo))
		*fieldNo++
	default:
		buf.WriteString(fmt.Sprintf("  // unhandled kind %s\n", kind.String()))
	}
}

func type2ProtoType(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Special case for time.Time
	if t == reflect.TypeOf(time.Time{}) {
		return "uint64"
	}

	// Special case for relabel.Regexp - treat as string
	if t.String() == "relabel.Regexp" {
		return "string"
	}

	// Check for types that we're importing
	if t == reflect.TypeOf(settingsv1.RecordingRule{}) {
		return "settings.v1.RecordingRule"
	}
	if t == reflect.TypeOf(settingsv1.StacktraceFilter{}) {
		return "settings.v1.StacktraceFilter"
	}
	if t == reflect.TypeOf(settingsv1.StacktraceFilterFunctionName{}) {
		return "settings.v1.StacktraceFilterFunctionName"
	}
	if t == reflect.TypeOf(typesv1.LabelPair{}) {
		return "types.v1.LabelPair"
	}

	// Check if this is a slice type alias (like RelabelRules which is []*relabel.Config)
	if t.Kind() == reflect.Slice && t.Name() != "" {
		// Get the element type
		elemType := t.Elem()
		return type2ProtoType(elemType)
	}

	// Check if this is a type alias by looking at the underlying kind
	// If the kind is a basic type (string, int, etc.) and it has a name,
	// it's likely a type alias and we should use the underlying type
	if isBasicKind(t.Kind()) && t.Name() != "" {
		// Return the proto type for the underlying kind
		return kindToProtoType(t.Kind())
	}

	n := t.Name()
	pkgpath := strings.Split(t.PkgPath(), "/")
	pkg := pkgpath[len(pkgpath)-1]

	switch n {
	case "int":
		return "int64"
	case "uint64":
		return "uint64"
	case "string":
		return "string"
	case "Duration":
		return "int64"
	case "float64":
		return "float"
	default:
		if len(pkg) > 0 {
			return strings.ToUpper(pkg[:1]) + pkg[1:] + n
		} else {
			return n
		}
	}

}

func isBasicKind(k reflect.Kind) bool {
	switch k {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.String:
		return true
	default:
		return false
	}
}

func kindToProtoType(k reflect.Kind) string {
	switch k {
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int64"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "uint64"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.String:
		return "string"
	default:
		return ""
	}
}

func hasSetMethod(t reflect.Type) bool {
	// Check if the type has a Set method with signature: func(*T) Set(string) error
	ptrType := reflect.PtrTo(t)
	method, found := ptrType.MethodByName("Set")
	if !found {
		return false
	}

	// Check method signature: should take a string and return an error
	methodType := method.Type
	// Method has receiver as first argument, so we check from index 1
	if methodType.NumIn() != 2 || methodType.NumOut() != 1 {
		return false
	}

	// Check that it takes a string as argument
	if methodType.In(1).Kind() != reflect.String {
		return false
	}

	// Check that it returns an error
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	if !methodType.Out(0).Implements(errorType) {
		return false
	}

	return true
}

func hasValidateMethod(t reflect.Type) bool {
	// Check if the type has a Validate method with signature: func(T) Validate() error
	// Also check on pointer receiver
	method, found := t.MethodByName("Validate")
	if !found {
		// Try pointer receiver
		ptrType := reflect.PtrTo(t)
		method, found = ptrType.MethodByName("Validate")
		if !found {
			return false
		}
		// For pointer receiver, check signature
		methodType := method.Type
		if methodType.NumIn() != 1 || methodType.NumOut() != 1 {
			return false
		}
		errorType := reflect.TypeOf((*error)(nil)).Elem()
		if !methodType.Out(0).Implements(errorType) {
			return false
		}
		return true
	}

	// Check method signature: should take no arguments (besides receiver) and return an error
	methodType := method.Type
	// Method has receiver as first argument, so we check from index 1
	if methodType.NumIn() != 1 || methodType.NumOut() != 1 {
		return false
	}

	// Check that it returns an error
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	if !methodType.Out(0).Implements(errorType) {
		return false
	}

	return true
}

func generateConversionFunction(buf *bytes.Buffer, messageName string, goType reflect.Type) {
	// Skip if it's a type we're importing
	if shouldSkipType(goType) {
		return
	}

	// Special case for UsageGroupConfig - it's stored as map<string, string>
	if strings.HasSuffix(messageName, "UsageGroupConfig") {
		return
	}

	functionName := "Convert" + messageName
	goTypeName := getGoTypeName(goType)

	buf.WriteString(fmt.Sprintf("\nfunc %s(msg *fuzz.%s) (%s, error) {\n", functionName, messageName, goTypeName))
	buf.WriteString(fmt.Sprintf("if msg == nil {\n"))
	buf.WriteString(fmt.Sprintf("return %s{}, nil\n", goTypeName))
	buf.WriteString(fmt.Sprintf("}\n"))
	buf.WriteString(fmt.Sprintf("result := %s{}\n", goTypeName))
	buf.WriteString(fmt.Sprintf("var err error\n"))
	buf.WriteString(fmt.Sprintf("_ = err\n"))

	// Generate field conversions
	fields := reflect.VisibleFields(goType)
	for _, field := range fields {
		if !field.IsExported() {
			continue
		}
		generateFieldConversion(buf, field, messageName)
	}

	// Check if the type has a Validate method and call it
	if hasValidateMethod(goType) {
		buf.WriteString("err = result.Validate()\n")
		buf.WriteString("if err != nil {\n")
		buf.WriteString("return result, err\n")
		buf.WriteString("}\n")
	}

	buf.WriteString("return result, nil\n")
	buf.WriteString("}\n")
}

func getGoTypeName(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	pkgPath := t.PkgPath()
	name := t.Name()

	if pkgPath == "" {
		return name
	}

	// Map package paths to their import aliases
	pkgMap := map[string]string{
		"github.com/grafana/pyroscope/pkg/validation":                                                   "validation.",
		"github.com/grafana/pyroscope/pkg/distributor/ingestlimits":                                     "ingestlimits.",
		"github.com/prometheus/prometheus/model/relabel":                                                "relabel.",
		"github.com/grafana/pyroscope/pkg/distributor/sampling":                                         "sampling.",
		"github.com/grafana/pyroscope/pkg/segmentwriter/client/distributor/placement/adaptiveplacement": "adaptiveplacement.",
		"github.com/grafana/pyroscope/pkg/metastore/index/cleaner/retention":                            "retention.",
		"github.com/grafana/pyroscope/api/gen/proto/go/settings/v1":                                     "settingsv1.",
	}

	for path, prefix := range pkgMap {
		if pkgPath == path {
			return prefix + name
		}
	}

	return name
}

func generateFieldConversion(buf *bytes.Buffer, field reflect.StructField, messageName string) {
	fieldName := field.Name
	fieldType := field.Type

	// Handle time.Time special case
	if fieldType == reflect.TypeOf(time.Time{}) {
		buf.WriteString(fmt.Sprintf("result.%s = time.Unix(0, int64(msg.%s))\n", fieldName, fieldName))
		return
	}

	// Handle time.Duration special case
	if fieldType.String() == "time.Duration" {
		buf.WriteString(fmt.Sprintf("result.%s = time.Duration(msg.%s)\n", fieldName, fieldName))
		return
	}

	// Handle model.LabelNames special case ([]string -> []LabelName)
	if fieldType.String() == "model.LabelNames" {
		buf.WriteString(fmt.Sprintf("if msg.%s != nil {\n", fieldName))
		buf.WriteString(fmt.Sprintf("result.%s = make(model.LabelNames, len(msg.%s))\n", fieldName, fieldName))
		buf.WriteString(fmt.Sprintf("for i, v := range msg.%s {\n", fieldName))
		buf.WriteString(fmt.Sprintf("result.%s[i] = model.LabelName(v)\n", fieldName))
		buf.WriteString(fmt.Sprintf("}\n"))
		buf.WriteString(fmt.Sprintf("}\n"))
		return
	}

	// Handle relabel.Regexp special case
	if fieldType.String() == "relabel.Regexp" {
		buf.WriteString(fmt.Sprintf("if msg.%s != \"\" {\n", fieldName))
		buf.WriteString(fmt.Sprintf("regexp, err := relabel.NewRegexp(msg.%s)\n", fieldName))
		buf.WriteString(fmt.Sprintf("if err == nil {\n"))
		buf.WriteString(fmt.Sprintf("result.%s = regexp\n", fieldName))
		buf.WriteString(fmt.Sprintf("}\n"))
		buf.WriteString(fmt.Sprintf("}\n"))
		return
	}

	// Handle UsageGroupConfig special case (map<string, string>)
	if strings.HasSuffix(fieldType.String(), "UsageGroupConfig") {
		buf.WriteString(fmt.Sprintf("if msg.%s != nil {\n", fieldName))
		buf.WriteString(fmt.Sprintf("result.%s = &validation.UsageGroupConfig{}\n", fieldName))
		buf.WriteString(fmt.Sprintf("err = result.%s.UnmarshalMap(msg.%s)\n", fieldName, fieldName))
		buf.WriteString(fmt.Sprintf("if err != nil {\n"))
		buf.WriteString(fmt.Sprintf("return result, err\n"))
		buf.WriteString(fmt.Sprintf("}\n"))
		buf.WriteString(fmt.Sprintf("}\n"))
		return
	}

	// Check if this is a type alias (named type with underlying basic type)
	if fieldType.Name() != "" && fieldType.PkgPath() != "" {
		underlyingKind := fieldType.Kind()
		if isBasicKind(underlyingKind) {
			// Check if the type has a Set method and underlying type is string
			if underlyingKind == reflect.String && hasSetMethod(fieldType) {
				// Use the Set method for conversion
				buf.WriteString(fmt.Sprintf("err = result.%s.Set(msg.%s)\n", fieldName, fieldName))
				buf.WriteString(fmt.Sprintf("if err != nil {\n"))
				buf.WriteString(fmt.Sprintf("return result, err\n"))
				buf.WriteString(fmt.Sprintf("}\n"))
			} else {
				// This is a type alias, generate a type cast
				buf.WriteString(fmt.Sprintf("result.%s = %s(msg.%s)\n", fieldName, fieldType.String(), fieldName))
			}
			return
		}
	}

	switch fieldType.Kind() {
	case reflect.Struct:
		// Handle nested struct fields with flattened naming
		prefix := type2ProtoType(fieldType) + "__"
		nestedFields := reflect.VisibleFields(fieldType)
		for _, nestedField := range nestedFields {
			if !nestedField.IsExported() {
				continue
			}
			generateNestedFieldConversion(buf, nestedField, prefix, "result."+fieldName)
		}

	case reflect.Ptr:
		elemType := fieldType.Elem()
		protoType := type2ProtoType(elemType)

		// Check if this is an imported type
		if shouldSkipType(elemType) {
			buf.WriteString(fmt.Sprintf("result.%s = msg.%s\n", fieldName, fieldName))
		} else {
			buf.WriteString(fmt.Sprintf("if msg.%s != nil {\n", fieldName))
			buf.WriteString(fmt.Sprintf("converted, err := Convert%s(msg.%s)\n", protoType, fieldName))
			buf.WriteString(fmt.Sprintf("if err == nil {\n"))
			// Check if the converted type has a Validate method
			if hasValidateMethod(elemType) {
				buf.WriteString(fmt.Sprintf("err = converted.Validate()\n"))
				buf.WriteString(fmt.Sprintf("if err == nil {\n"))
				buf.WriteString(fmt.Sprintf("result.%s = &converted\n", fieldName))
				buf.WriteString(fmt.Sprintf("}\n"))
				// If validation fails, leave the field nil
			} else {
				buf.WriteString(fmt.Sprintf("result.%s = &converted\n", fieldName))
			}
			buf.WriteString(fmt.Sprintf("}\n"))
			// If conversion fails, leave the field nil
			buf.WriteString(fmt.Sprintf("}\n"))
		}

	case reflect.Slice:
		elemType := fieldType.Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
			protoType := type2ProtoType(elemType)
			if shouldSkipType(elemType) {
				buf.WriteString(fmt.Sprintf("result.%s = msg.%s\n", fieldName, fieldName))
			} else {
				buf.WriteString(fmt.Sprintf("if msg.%s != nil {\n", fieldName))
				buf.WriteString(fmt.Sprintf("result.%s = make([]*%s, 0, len(msg.%s))\n", fieldName, getGoTypeName(elemType), fieldName))
				buf.WriteString(fmt.Sprintf("for _, item := range msg.%s {\n", fieldName))
				buf.WriteString(fmt.Sprintf("converted, err := Convert%s(item)\n", protoType))
				buf.WriteString(fmt.Sprintf("if err == nil {\n"))
				// Check if the converted type has a Validate method
				if hasValidateMethod(elemType) {
					buf.WriteString(fmt.Sprintf("err = converted.Validate()\n"))
					buf.WriteString(fmt.Sprintf("if err == nil {\n"))
					buf.WriteString(fmt.Sprintf("result.%s = append(result.%s, &converted)\n", fieldName, fieldName))
					buf.WriteString(fmt.Sprintf("}\n"))
					// If validation fails, skip this item
				} else {
					buf.WriteString(fmt.Sprintf("result.%s = append(result.%s, &converted)\n", fieldName, fieldName))
				}
				buf.WriteString(fmt.Sprintf("}\n"))
				// If conversion fails, skip this item
				buf.WriteString(fmt.Sprintf("}\n"))
				buf.WriteString(fmt.Sprintf("}\n"))
			}
		} else {
			buf.WriteString(fmt.Sprintf("result.%s = msg.%s\n", fieldName, fieldName))
		}

	case reflect.Map:
		valueType := fieldType.Elem()
		if valueType.Kind() == reflect.Ptr {
			valueType = valueType.Elem()
			protoType := type2ProtoType(valueType)
			buf.WriteString(fmt.Sprintf("if msg.%s != nil {\n", fieldName))
			buf.WriteString(fmt.Sprintf("result.%s = make(map[string]*%s)\n", fieldName, getGoTypeName(valueType)))
			buf.WriteString(fmt.Sprintf("for k, v := range msg.%s {\n", fieldName))
			buf.WriteString(fmt.Sprintf("if v != nil {\n"))
			buf.WriteString(fmt.Sprintf("converted, err := Convert%s(v)\n", protoType))
			buf.WriteString(fmt.Sprintf("if err == nil {\n"))
			// Check if the converted type has a Validate method
			if hasValidateMethod(valueType) {
				buf.WriteString(fmt.Sprintf("err = converted.Validate()\n"))
				buf.WriteString(fmt.Sprintf("if err == nil {\n"))
				buf.WriteString(fmt.Sprintf("result.%s[k] = &converted\n", fieldName))
				buf.WriteString(fmt.Sprintf("}\n"))
				// If validation fails, skip this entry
			} else {
				buf.WriteString(fmt.Sprintf("result.%s[k] = &converted\n", fieldName))
			}
			buf.WriteString(fmt.Sprintf("}\n"))
			// If conversion fails, skip this entry
			buf.WriteString(fmt.Sprintf("}\n"))
			buf.WriteString(fmt.Sprintf("}\n"))
			buf.WriteString(fmt.Sprintf("}\n"))
		} else if valueType.Kind() == reflect.Struct {
			protoType := type2ProtoType(valueType)
			buf.WriteString(fmt.Sprintf("if msg.%s != nil {\n", fieldName))
			buf.WriteString(fmt.Sprintf("result.%s = make(map[string]%s)\n", fieldName, getGoTypeName(valueType)))
			buf.WriteString(fmt.Sprintf("for k, v := range msg.%s {\n", fieldName))
			buf.WriteString(fmt.Sprintf("if v != nil {\n"))
			buf.WriteString(fmt.Sprintf("converted, err := Convert%s(v)\n", protoType))
			buf.WriteString(fmt.Sprintf("if err == nil {\n"))
			// Check if the converted type has a Validate method
			if hasValidateMethod(valueType) {
				buf.WriteString(fmt.Sprintf("err = converted.Validate()\n"))
				buf.WriteString(fmt.Sprintf("if err == nil {\n"))
				buf.WriteString(fmt.Sprintf("result.%s[k] = converted\n", fieldName))
				buf.WriteString(fmt.Sprintf("}\n"))
				// If validation fails, skip this entry
			} else {
				buf.WriteString(fmt.Sprintf("result.%s[k] = converted\n", fieldName))
			}
			buf.WriteString(fmt.Sprintf("}\n"))
			// If conversion fails, skip this entry  
			buf.WriteString(fmt.Sprintf("}\n"))
			buf.WriteString(fmt.Sprintf("}\n"))
			buf.WriteString(fmt.Sprintf("}\n"))
		} else {
			buf.WriteString(fmt.Sprintf("result.%s = msg.%s\n", fieldName, fieldName))
		}

	case reflect.String, reflect.Bool:
		buf.WriteString(fmt.Sprintf("result.%s = msg.%s\n", fieldName, fieldName))

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		buf.WriteString(fmt.Sprintf("result.%s = int(msg.%s)\n", fieldName, fieldName))

	case reflect.Int64:
		// msg field is already int64 in proto, no need to cast
		buf.WriteString(fmt.Sprintf("result.%s = msg.%s\n", fieldName, fieldName))

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		buf.WriteString(fmt.Sprintf("result.%s = uint64(msg.%s)\n", fieldName, fieldName))

	case reflect.Uint64:
		// No typecast needed for uint64 -> uint64
		buf.WriteString(fmt.Sprintf("result.%s = msg.%s\n", fieldName, fieldName))

	case reflect.Float32:
		buf.WriteString(fmt.Sprintf("result.%s = float32(msg.%s)\n", fieldName, fieldName))

	case reflect.Float64:
		buf.WriteString(fmt.Sprintf("result.%s = float64(msg.%s)\n", fieldName, fieldName))
	}
}

func generateNestedFieldConversion(buf *bytes.Buffer, field reflect.StructField, prefix string, targetPrefix string) {
	fieldName := prefix + field.Name
	fieldType := field.Type

	// Handle time.Time special case
	if fieldType == reflect.TypeOf(time.Time{}) {
		buf.WriteString(fmt.Sprintf("%s.%s = time.Unix(0, int64(msg.%s))\n", targetPrefix, field.Name, fieldName))
		return
	}

	// Handle time.Duration special case
	if fieldType.String() == "time.Duration" {
		buf.WriteString(fmt.Sprintf("%s.%s = time.Duration(msg.%s)\n", targetPrefix, field.Name, fieldName))
		return
	}

	// Check if this is a type alias (named type with underlying basic type)
	if fieldType.Name() != "" && fieldType.PkgPath() != "" {
		underlyingKind := fieldType.Kind()
		if isBasicKind(underlyingKind) {
			// Check if the type has a Set method and underlying type is string
			if underlyingKind == reflect.String && hasSetMethod(fieldType) {
				// Use the Set method for conversion
				buf.WriteString(fmt.Sprintf("err = %s.%s.Set(msg.%s)\n", targetPrefix, field.Name, fieldName))
				buf.WriteString(fmt.Sprintf("if err != nil {\n"))
				buf.WriteString(fmt.Sprintf("return result, err\n"))
				buf.WriteString(fmt.Sprintf("}\n"))
			} else {
				// This is a type alias, generate a type cast
				buf.WriteString(fmt.Sprintf("%s.%s = %s(msg.%s)\n", targetPrefix, field.Name, fieldType.String(), fieldName))
			}
			return
		}
	}

	switch fieldType.Kind() {
	case reflect.String, reflect.Bool:
		buf.WriteString(fmt.Sprintf("%s.%s = msg.%s\n", targetPrefix, field.Name, fieldName))

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		buf.WriteString(fmt.Sprintf("%s.%s = int(msg.%s)\n", targetPrefix, field.Name, fieldName))

	case reflect.Int64:
		// msg field is already int64 in proto, no need to cast
		buf.WriteString(fmt.Sprintf("%s.%s = msg.%s\n", targetPrefix, field.Name, fieldName))

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		buf.WriteString(fmt.Sprintf("%s.%s = uint64(msg.%s)\n", targetPrefix, field.Name, fieldName))

	case reflect.Uint64:
		// No typecast needed for uint64 -> uint64
		buf.WriteString(fmt.Sprintf("%s.%s = msg.%s\n", targetPrefix, field.Name, fieldName))

	case reflect.Float32:
		buf.WriteString(fmt.Sprintf("%s.%s = float32(msg.%s)\n", targetPrefix, field.Name, fieldName))

	case reflect.Float64:
		buf.WriteString(fmt.Sprintf("%s.%s = float64(msg.%s)\n", targetPrefix, field.Name, fieldName))
	}
}
