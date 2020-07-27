package k8s

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_serviceAnnotationParser_ParseStringAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		prefixes    []string
		suffix      string
		annotations map[string]string
		wantExist   bool
		wantValue   string
	}{
		{
			name:        "Empty annotations",
			prefixes:    []string{"service.kubernetes.io", "service.beta.kubernetes.io"},
			suffix:      "random_suffix",
			annotations: map[string]string{},
			wantExist:   false,
			wantValue:   "",
		},
		{
			name:     "Single annotation",
			prefixes: []string{"service.kubernetes.io", "service.beta.kubernetes.io"},
			suffix:   ServiceAnnotationLoadBalancerType,
			annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-type": "nlb-ip",
			},
			wantExist: true,
			wantValue: "nlb-ip",
		},
		{
			name:     "Muiltiple annotations",
			prefixes: []string{"service.kubernetes.io", "service.beta.kubernetes.io"},
			suffix:   "t2",
			annotations: map[string]string{
				"service.beta.kubernetes.io/test-1": "value-1",
				"service.beta.kubernetes.io/t2":     "value-2",
				"service.beta.kubernetes.io/t3":     "value-3",
				"something else":                    "random",
				"abc.cde":                           "d",
			},
			wantExist: true,
			wantValue: "value-2",
		},
		{
			name:     "override",
			prefixes: []string{"service.kubernetes.io", "service.beta.kubernetes.io"},
			suffix:   "t2",
			annotations: map[string]string{
				"service.beta.kubernetes.io/test-1": "value-1",
				"service.kubernetes.io/t2":          "value-2",
				"service.beta.kubernetes.io/t3":     "value-3",
				"random":                            "ignore",
			},
			wantExist: true,
			wantValue: "value-2",
		},
		{
			name:     "no-prefix",
			prefixes: nil,
			suffix:   "t2",
			annotations: map[string]string{
				"service.beta.kubernetes.io/test-1": "value-1",
				"service.kubernetes.io/t2":          "value-2",
				"service.beta.kubernetes.io/t3":     "value-3",
				"random":                            "ignore",
			},
			wantExist: false,
			wantValue: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewServiceAnnotationParser(tt.prefixes...)
			value := ""
			exists := parser.ParseStringAnnotation(tt.suffix, &value, tt.annotations)
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
		prefixes    []string
		suffix      string
		annotations map[string]string
		wantExist   bool
		wantValue   int64
		wantError   bool
	}{
		{
			name:        "no annotation",
			prefixes:    nil,
			suffix:      "some-suffix",
			annotations: nil,
			wantExist:   false,
			wantValue:   0,
			wantError:   false,
		},
		{
			name:     "single annotation",
			prefixes: []string{"with spaces"},
			suffix:   "destination",
			annotations: map[string]string{
				"with spaces/destination": "101",
			},
			wantExist: true,
			wantValue: 101,
			wantError: false,
		},
		{
			name:     "errors",
			prefixes: []string{"prefix/test"},
			suffix:   "invalid",
			annotations: map[string]string{
				"prefix/test/invalid": "22d",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewServiceAnnotationParser(tt.prefixes...)
			value := int64(0)
			exists, err := parser.ParseInt64Annotation(tt.suffix, &value, tt.annotations)
			if tt.wantError {
				assert.True(t, err != nil)
			} else {
				assert.Equal(t, err, nil)
				assert.Equal(t, tt.wantExist, exists)
				if tt.wantExist {
					assert.Equal(t, tt.wantValue, value)
				}
			}
		})

	}
}
