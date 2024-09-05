package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCompareOptionForCertificate(t *testing.T) {
	type args struct {
		lhs elbv2types.Certificate
		rhs elbv2types.Certificate
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "two certificate equals exactly",
			args: args{
				lhs: elbv2types.Certificate{
					CertificateArn: awssdk.String("cert-A"),
				},
				rhs: elbv2types.Certificate{
					CertificateArn: awssdk.String("cert-A"),
				},
			},
			want: true,
		},
		{
			name: "two certificate are not equals if certARN mismatch",
			args: args{
				lhs: elbv2types.Certificate{
					CertificateArn: awssdk.String("cert-A"),
				},
				rhs: elbv2types.Certificate{
					CertificateArn: awssdk.String("cert-B"),
				},
			},
			want: false,
		},
		{
			name: "two certificate equals exactly irrelevant of their isDefault",
			args: args{
				lhs: elbv2types.Certificate{
					CertificateArn: awssdk.String("cert-A"),
				},
				rhs: elbv2types.Certificate{
					CertificateArn: awssdk.String("cert-A"),
					IsDefault:      awssdk.Bool(true),
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cmp.Equal(tt.args.lhs, tt.args.rhs, CompareOptionForCertificate())
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompareOptionForCertificates(t *testing.T) {
	type args struct {
		lhs []*elbv2types.Certificate
		rhs []*elbv2types.Certificate
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "two certificates slice are equal exactly",
			args: args{
				lhs: []*elbv2types.Certificate{
					{
						CertificateArn: awssdk.String("cert-A"),
					},
				},
				rhs: []*elbv2types.Certificate{
					{
						CertificateArn: awssdk.String("cert-A"),
					},
				},
			},
			want: true,
		},
		{
			name: "two certificates slice are not equal if certARN mismatches",
			args: args{
				lhs: []*elbv2types.Certificate{
					{
						CertificateArn: awssdk.String("cert-A"),
					},
				},
				rhs: []*elbv2types.Certificate{
					{
						CertificateArn: awssdk.String("cert-B"),
					},
				},
			},
			want: false,
		},
		{
			name: "two certificates slice are equal if they are equal after sorted",
			args: args{
				lhs: []*elbv2types.Certificate{
					{
						CertificateArn: awssdk.String("cert-A"),
					},
					{
						CertificateArn: awssdk.String("cert-B"),
					},
				},
				rhs: []*elbv2types.Certificate{
					{
						CertificateArn: awssdk.String("cert-B"),
					},
					{
						CertificateArn: awssdk.String("cert-A"),
					},
				},
			},
			want: true,
		},
		{
			name: "two certificates slice are equal for nil and empty slice",
			args: args{
				lhs: []*elbv2types.Certificate{},
				rhs: nil,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cmp.Equal(tt.args.lhs, tt.args.rhs, CompareOptionForCertificates())
			assert.Equal(t, tt.want, got)
		})
	}
}
