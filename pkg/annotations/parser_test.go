package annotations

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_annotationParser_ParseStringAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		opts        []ParseOption
		suffix      string
		annotations map[string]string
		wantExist   bool
		wantValue   string
	}{
		{
			name:        "Empty annotations",
			prefix:      "pfx.io",
			opts:        []ParseOption{},
			suffix:      "random_suffix",
			annotations: map[string]string{},
			wantExist:   false,
			wantValue:   "",
		},
		{
			name:   "Exact match",
			prefix: "pfx.io",
			opts:   []ParseOption{WithExact()},
			suffix: "k8s.io/exact_suffix",
			annotations: map[string]string{
				"pfx.io/exact_suffix": "wprefix",
				"k8s.io/exact_suffix": "exact_value",
				"exact_suffix":        "nopfx_value",
			},
			wantExist: true,
			wantValue: "exact_value",
		},
		{
			name:   "Single annotation",
			prefix: "t.co",
			suffix: "aws-load-balancer-type",
			annotations: map[string]string{
				"t.co/aws-load-balancer-type": "nlb-ip",
			},
			wantExist: true,
			wantValue: "nlb-ip",
		},
		{
			name:   "Muiltiple annotations",
			prefix: "b.k.io",
			suffix: "t2",
			annotations: map[string]string{
				"b.k.io/test-1":  "value-1",
				"b.k.io/t2":      "value-2",
				"b.k.io/t3":      "value-3",
				"k.io/t2":        "ignore-this",
				"something else": "random",
				"abc.cde":        "d",
			},
			wantExist: true,
			wantValue: "value-2",
		},
		{
			name:   "With alternatives",
			opts:   []ParseOption{WithAlternativePrefixes("2s.io", "alt")},
			prefix: "k8s.io",
			suffix: "t2",
			annotations: map[string]string{
				"k8s.io/test-1": "value-1",
				"alt/t2":        "value-2",
				"k8s/t3":        "value-3",
				"random":        "ignore",
			},
			wantExist: true,
			wantValue: "value-2",
		},
		{
			name:   "no-prefix",
			suffix: "t2",
			annotations: map[string]string{
				"k.io/test-1": "value-1",
				"n.io/t2":     "value-2",
				"k.io/t3":     "value-3",
				"random":      "ignore",
			},
			wantExist: false,
			wantValue: "",
		},
		{
			name:   "multi option exact match",
			suffix: "test-1",
			opts:   []ParseOption{WithAlternativePrefixes("ab"), WithAlternativePrefixes("cd"), WithExact(), WithAlternativePrefixes("kk")},
			annotations: map[string]string{
				"test-1":    "value-1",
				"ab/test-1": "value-2",
				"cd/test-1": "value-3",
				"random":    "ignore",
			},
			wantExist: true,
			wantValue: "value-1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewSuffixAnnotationParser(tt.prefix)
			value := ""
			exists := parser.ParseStringAnnotation(tt.suffix, &value, tt.annotations, tt.opts...)
			assert.Equal(t, tt.wantExist, exists)
			if tt.wantExist {
				assert.Equal(t, tt.wantValue, value)
			}
		})
	}
}

func Test_serviceAnnotationParser_ParseInt64Annotation(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		opts        []ParseOption
		suffix      string
		annotations map[string]string
		wantExist   bool
		wantValue   int64
		wantError   bool
	}{
		{
			name:        "no annotation",
			prefix:      "",
			suffix:      "some-suffix",
			annotations: nil,
			wantExist:   false,
			wantError:   false,
		},
		{
			name:   "single annotation",
			prefix: "with spaces",
			suffix: "destination",
			annotations: map[string]string{
				"with spaces/destination": "101",
			},
			wantExist: true,
			wantValue: 101,
			wantError: false,
		},
		{
			name:   "errors",
			prefix: "prefix",
			suffix: "invalid",
			opts:   []ParseOption{WithAlternativePrefixes("prefix/test")},
			annotations: map[string]string{
				"prefix/test/invalid": "22d",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewSuffixAnnotationParser(tt.prefix)
			value := int64(0)
			exists, err := parser.ParseInt64Annotation(tt.suffix, &value, tt.annotations, tt.opts...)
			if tt.wantError {
				assert.True(t, err != nil)
			} else {
				assert.Equal(t, nil, err)
				assert.Equal(t, tt.wantExist, exists)
				if tt.wantExist {
					assert.Equal(t, tt.wantValue, value)
				}
			}
		})
	}
}

func Test_serviceAnnotationParser_ParseStringSliceAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		opts        []ParseOption
		suffix      string
		annotations map[string]string
		wantExist   bool
		wantValue   []string
	}{
		{
			name:        "Nonexistent",
			prefix:      "a.co",
			suffix:      "try",
			annotations: nil,
			wantExist:   false,
			wantValue:   nil,
		},
		{
			name:   "empty value",
			prefix: "a.co",
			suffix: "b.co/val",
			opts:   []ParseOption{WithExact()},
			annotations: map[string]string{
				"b.co/val": "\t,  ,,,,,",
			},
			wantExist: true,
			wantValue: []string{},
		},
		{
			name:   "single value",
			prefix: "a.co",
			suffix: "val",
			annotations: map[string]string{
				"b.co/val": "abc,",
			},
			opts:      []ParseOption{WithAlternativePrefixes("de"), WithAlternativePrefixes("b.co")},
			wantExist: true,
			wantValue: []string{"abc"},
		},
		{
			name:   "multiple values",
			prefix: "a.co",
			suffix: "b.co/val",
			annotations: map[string]string{
				"b.co/val": "ab  c, de  \t, test, ,\t\t\t123   \t\t, \"ooo, 1,,,, 3",
				"val":      "abc, def, a d ",
				"a.co/y":   "e",
			},
			opts:      []ParseOption{WithAlternativePrefixes("de"), WithExact(), WithAlternativePrefixes("co")},
			wantExist: true,
			wantValue: []string{"ab  c", "de", "test", "123", "\"ooo", "1", "3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewSuffixAnnotationParser(tt.suffix)
			value := []string{}
			exists := parser.ParseStringSliceAnnotation(tt.suffix, &value, tt.annotations, tt.opts...)
			assert.Equal(t, tt.wantExist, exists)
			if tt.wantExist {
				assert.Equal(t, tt.wantValue, value)
			}
		})
	}
}

func Test_serviceAnnotationParser_ParseStringMapAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		opts        []ParseOption
		suffix      string
		annotations map[string]string
		wantExist   bool
		wantValue   map[string]string
	}{
		{
			name:        "empty",
			prefix:      "",
			suffix:      "",
			annotations: nil,
			wantExist:   false,
			wantValue:   nil,
		},
		{
			name:   "multiple keys",
			prefix: "p.co",
			suffix: "sfx",
			annotations: map[string]string{
				"first-value": "1",
				"p.co/sfx":    "key= val,   c, d=, lb-type=ip, sec    \t\t=  internal,,   ,c=override,alone, =value",
			},
			wantExist: true,
			wantValue: map[string]string{
				"key":     "val",
				"c":       "override",
				"d":       "",
				"lb-type": "ip",
				"sec":     "internal",
				"alone":   "",
				"":        "value",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewSuffixAnnotationParser(tt.prefix)
			value := map[string]string{}
			exists := parser.ParseStringMapAnnotation(tt.suffix, &value, tt.annotations, tt.opts...)
			assert.Equal(t, tt.wantExist, exists)
			if tt.wantExist {
				assert.Equal(t, tt.wantValue, value)
			}
		})
	}
}

func Test_serviceAnnotationParser_ParseJSONAnnotation(t *testing.T) {
	type objStruct struct {
		Name   string
		Value  string
		Weight int64
	}
	tests := []struct {
		name        string
		prefix      string
		opts        []ParseOption
		suffix      string
		annotations map[string]string
		wantExist   bool
		wantValue   interface{}
		wantError   bool
	}{
		{
			name:        "Nonexistent",
			prefix:      "a.bc",
			suffix:      "empty-j",
			annotations: nil,
			wantExist:   false,
			wantValue:   nil,
		},
		{
			name:   "Valid type",
			prefix: "a.bc",
			suffix: "json-annotation",
			annotations: map[string]string{
				"a.bc/json-annotation": "{\"Name\": \"Test\", \"Value\": \"ABC\", \"Weight\": 123}",
			},
			wantExist: true,
			wantValue: objStruct{"Test", "ABC", 123},
		},
		{
			name:   "Invalid",
			prefix: "a.bc",
			suffix: "json-annotation",
			annotations: map[string]string{
				"a.bc/json-annotation": "{\"Name\": \"Test\", \"Value\": \"ABC\", \"Weight\": ",
			},
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewSuffixAnnotationParser(tt.prefix)
			value := objStruct{}
			exists, err := parser.ParseJSONAnnotation(tt.suffix, &value, tt.annotations, tt.opts...)
			if tt.wantError {
				assert.True(t, err != nil)
			} else {
				assert.Equal(t, nil, err)
				assert.Equal(t, tt.wantExist, exists)
				if tt.wantExist {
					assert.Equal(t, tt.wantValue, value)
				}
			}
		})
	}
}
