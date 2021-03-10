package ingress

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_defaultClassAnnotationMatcher_Matches(t *testing.T) {
	type fields struct {
		ingressClass string
	}
	type args struct {
		ingClassAnnotation string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name: "specified non-empty ingressClass and matches",
			fields: fields{
				ingressClass: "alb",
			},
			args: args{
				ingClassAnnotation: "alb",
			},
			want: true,
		},
		{
			name: "specified non-empty ingressClass and mismatches",
			fields: fields{
				ingressClass: "alb",
			},
			args: args{
				ingClassAnnotation: "nginx",
			},
			want: false,
		},
		{
			name: "specified empty ingressClass with empty ingressClassAnnotation",
			fields: fields{
				ingressClass: "",
			},
			args: args{
				ingClassAnnotation: "",
			},
			want: true,
		},
		{
			name: "specified empty ingressClass with alb ingressClassAnnotation",
			fields: fields{
				ingressClass: "",
			},
			args: args{
				ingClassAnnotation: "alb",
			},
			want: true,
		},
		{
			name: "specified empty ingressClass with non-empty and non-alb ingressClassAnnotation",
			fields: fields{
				ingressClass: "",
			},
			args: args{
				ingClassAnnotation: "nginx",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &defaultClassAnnotationMatcher{
				ingressClass: tt.fields.ingressClass,
			}
			got := m.Matches(tt.args.ingClassAnnotation)
			assert.Equal(t, tt.want, got)
		})
	}
}
