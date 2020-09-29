package ingress

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_acmCertDiscovery_domainMatchesHost(t *testing.T) {
	type args struct {
		domainName string
		tlsHost    string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "exact domain matches",
			args: args{
				domainName: "example.com",
				tlsHost:    "example.com",
			},
			want: true,
		},
		{
			name: "exact domain didn't matches",
			args: args{
				domainName: "example.com",
				tlsHost:    "www.example.com",
			},
			want: false,
		},
		{
			name: "wildcard domain matches",
			args: args{
				domainName: "*.example.com",
				tlsHost:    "www.example.com",
			},
			want: true,
		},
		{
			name: "wildcard domain didn't matches - case 1",
			args: args{
				domainName: "*.example.com",
				tlsHost:    "example.com",
			},
			want: false,
		},
		{
			name: "wildcard domain didn't matches - case 2",
			args: args{
				domainName: "*.example.com",
				tlsHost:    "www.app.example.com",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &acmCertDiscovery{}
			got := d.domainMatchesHost(tt.args.domainName, tt.args.tlsHost)
			assert.Equal(t, tt.want, got)
		})
	}
}
