package annotations

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_suffixAnnotationParser_ParseStringAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		suffix      string
		annotations []map[string]string
		wantValue   string
		wantExists  bool
	}{
		{
			name:   "exists in annotation map",
			suffix: "target-type",
			annotations: []map[string]string{
				{
					"alb.ingress.kubernetes.io/target-type": "instance",
				},
			},
			wantValue:  "instance",
			wantExists: true,
		},
		{
			name:   "exists in both annotation map",
			suffix: "target-type",
			annotations: []map[string]string{
				{
					"alb.ingress.kubernetes.io/target-type": "instance",
				},
				{
					"alb.ingress.kubernetes.io/target-type": "ip",
				},
			},
			wantValue:  "instance",
			wantExists: true,
		},
		{
			name:   "exists in second annotation map",
			suffix: "target-type",
			annotations: []map[string]string{
				{
					"alb.ingress.kubernetes.io/security-groups": "sg-abc",
				},
				{
					"alb.ingress.kubernetes.io/target-type": "ip",
				},
			},
			wantValue:  "ip",
			wantExists: true,
		},
		{
			name:   "no-exists in any annotation map",
			suffix: "target-type",
			annotations: []map[string]string{
				{
					"alb.ingress.kubernetes.io/security-groups": "sg-abc",
				},
				{
					"alb.ingress.kubernetes.io/security-groups": "sg-abc",
				},
			},
			wantValue:  "",
			wantExists: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			var gotValue string
			gotExists := p.ParseStringAnnotation(tt.suffix, &gotValue, tt.annotations...)
			assert.Equal(t, tt.wantValue, gotValue)
			assert.Equal(t, tt.wantExists, gotExists)
		})
	}
}

func Test_suffixAnnotationParser_ParseInt64Annotation(t *testing.T) {
	tests := []struct {
		name        string
		suffix      string
		annotations []map[string]string
		wantValue   int64
		wantExists  bool
		wantErr     error
	}{
		{
			name:   "exists in annotation map",
			suffix: "healthcheck-interval-seconds",
			annotations: []map[string]string{
				{
					"alb.ingress.kubernetes.io/healthcheck-interval-seconds": "42",
				},
			},
			wantValue:  42,
			wantExists: true,
			wantErr:    nil,
		},
		{
			name:   "exists in both annotation map",
			suffix: "healthcheck-interval-seconds",
			annotations: []map[string]string{
				{
					"alb.ingress.kubernetes.io/healthcheck-interval-seconds": "42",
				},
				{
					"alb.ingress.kubernetes.io/healthcheck-interval-seconds": "10",
				},
			},
			wantValue:  42,
			wantExists: true,
			wantErr:    nil,
		},
		{
			name:   "exists in second annotation map",
			suffix: "healthcheck-interval-seconds",
			annotations: []map[string]string{
				{
					"alb.ingress.kubernetes.io/security-groups": "sg-abc",
				},
				{
					"alb.ingress.kubernetes.io/healthcheck-interval-seconds": "10",
				},
			},
			wantValue:  10,
			wantExists: true,
			wantErr:    nil,
		},
		{
			name:   "no-exists in any annotation map",
			suffix: "healthcheck-interval-seconds",
			annotations: []map[string]string{
				{
					"alb.ingress.kubernetes.io/security-groups": "sg-abc",
				},
				{
					"alb.ingress.kubernetes.io/security-groups": "sg-abc",
				},
			},
			wantValue:  0,
			wantExists: false,
			wantErr:    nil,
		},
		{
			name:   "value is not integer",
			suffix: "healthcheck-interval-seconds",
			annotations: []map[string]string{
				{
					"alb.ingress.kubernetes.io/healthcheck-interval-seconds": "im not integer",
				},
			},
			wantValue:  0,
			wantExists: true,
			wantErr:    errors.New(`failed to parse annotation, alb.ingress.kubernetes.io/healthcheck-interval-seconds: im not integer: strconv.ParseInt: parsing "im not integer": invalid syntax`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			var gotValue int64
			gotExists, gotErr := p.ParseInt64Annotation(tt.suffix, &gotValue, tt.annotations...)
			assert.Equal(t, tt.wantValue, gotValue)
			assert.Equal(t, tt.wantExists, gotExists)
			if tt.wantErr == nil {
				assert.NoError(t, gotErr)
			} else {
				assert.EqualError(t, gotErr, tt.wantErr.Error())
			}
		})
	}
}
