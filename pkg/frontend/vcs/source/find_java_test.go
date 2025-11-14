package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_convertJavaFunctionNameToPath(t *testing.T) {
	tests := []struct {
		name         string
		functionName string
		expected     string
	}{
		{
			name:         "simple class name",
			functionName: "com/example/MyClass",
			expected:     "com/example/MyClass.java",
		},
		{
			name:         "class with method",
			functionName: "com/example/MyClass.myMethod",
			expected:     "com/example/MyClass.java",
		},
		{
			name:         "inner class with dollar sign",
			functionName: "com/example/OuterClass$InnerClass",
			expected:     "com/example/OuterClass.java",
		},
		{
			name:         "inner class with method",
			functionName: "com/example/OuterClass$InnerClass.method",
			expected:     "com/example/OuterClass.java",
		},
		{
			name:         "lambda or anonymous class",
			functionName: "com/example/MyClass$1",
			expected:     "com/example/MyClass.java",
		},
		{
			name:         "nested inner class",
			functionName: "com/example/Outer$Inner$DeepInner",
			expected:     "com/example/Outer.java",
		},
		{
			name:         "method with parameters in name",
			functionName: "com/example/MyClass.method.withDots",
			expected:     "com/example/MyClass.java",
		},
		{
			name:         "single segment with method",
			functionName: "MyClass.method",
			expected:     "MyClass.java",
		},
		{
			name:         "single segment with inner class",
			functionName: "MyClass$Inner",
			expected:     "MyClass.java",
		},
		{
			name:         "no package, just class",
			functionName: "MyClass",
			expected:     "MyClass.java",
		},
		{
			name:         "complex nested structure",
			functionName: "org/springframework/boot/Application$Config$Bean.initialize",
			expected:     "org/springframework/boot/Application.java",
		},
		{
			name:         "dollar sign before dot",
			functionName: "com/example/Class$Inner.method",
			expected:     "com/example/Class.java",
		},
		{
			name:         "dot before dollar sign",
			functionName: "com/example/Class.method$1",
			expected:     "com/example/Class.java",
		},
		{
			name:         "multiple dollar signs",
			functionName: "com/example/Outer$Middle$Inner",
			expected:     "com/example/Outer.java",
		},
		{
			name:         "multiple dots",
			functionName: "com/example/Class.method.subMethod",
			expected:     "com/example/Class.java",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertJavaFunctionNameToPath(tt.functionName)
			assert.Equal(t, tt.expected, result)
		})
	}
}
