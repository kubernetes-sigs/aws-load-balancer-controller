package acm

import (
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/stretchr/testify/assert"
	acmModel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/acm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

func Test_matchResAndSDKCertificates(t *testing.T) {
	stack := core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "name"})

	createdAtTimeNow := time.Now()
	createdAtTimeBefore := time.Now().Add(-reissueWaitTime)
	type args struct {
		resCerts         []*acmModel.Certificate
		sdkCerts         []CertificateWithTags
		resourceIDTagKey string
	}
	tests := []struct {
		name    string
		args    args
		want    []resAndSDKCertificatePair
		want1   []*acmModel.Certificate
		want2   []CertificateWithTags
		wantErr error
	}{
		{
			name: "all certificates have match",
			args: args{
				resCerts: []*acmModel.Certificate{
					{
						ResourceMeta: core.NewResourceMeta(stack, "AWS::ACM::Certificate", "id-1"),
						Spec: acmModel.CertificateSpec{
							Type:                    acmtypes.CertificateTypeAmazonIssued,
							DomainName:              "example.com",
							SubjectAlternativeNames: []string{"example.com"},
							ValidationMethod:        acmtypes.ValidationMethodDns,
						},
					},
				},
				sdkCerts: []CertificateWithTags{
					{
						Certificate: &acmtypes.CertificateSummary{
							CertificateArn:                  awssdk.String("arn-1"),
							DomainName:                      awssdk.String("example.com"),
							SubjectAlternativeNameSummaries: []string{"example.com"},
							Status:                          acmtypes.CertificateStatusIssued,
							Type:                            acmtypes.CertificateTypeAmazonIssued,
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: []resAndSDKCertificatePair{
				{
					resCert: &acmModel.Certificate{
						ResourceMeta: core.NewResourceMeta(stack, "AWS::ACM::Certificate", "id-1"),
						Spec: acmModel.CertificateSpec{
							Type:                    acmtypes.CertificateTypeAmazonIssued,
							DomainName:              "example.com",
							SubjectAlternativeNames: []string{"example.com"},
							ValidationMethod:        acmtypes.ValidationMethodDns,
						},
					},
					sdkCert: CertificateWithTags{
						Certificate: &acmtypes.CertificateSummary{
							CertificateArn:                  awssdk.String("arn-1"),
							DomainName:                      awssdk.String("example.com"),
							SubjectAlternativeNameSummaries: []string{"example.com"},
							Status:                          acmtypes.CertificateStatusIssued,
							Type:                            acmtypes.CertificateTypeAmazonIssued,
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
			},
		},
		{
			name: "certificate with missing SANs",
			args: args{
				resCerts: []*acmModel.Certificate{
					{
						ResourceMeta: core.NewResourceMeta(stack, "AWS::ACM::Certificate", "id-2"),
						Spec: acmModel.CertificateSpec{
							Type:                    acmtypes.CertificateTypeAmazonIssued,
							DomainName:              "example.com",
							SubjectAlternativeNames: []string{"example.com", "otherexample.com"},
							ValidationMethod:        acmtypes.ValidationMethodDns,
						},
					},
				},
				sdkCerts: []CertificateWithTags{
					{
						Certificate: &acmtypes.CertificateSummary{
							CertificateArn:                  awssdk.String("arn-2"),
							DomainName:                      awssdk.String("example.com"),
							SubjectAlternativeNameSummaries: []string{"example.com"},
							Status:                          acmtypes.CertificateStatusIssued,
							Type:                            acmtypes.CertificateTypeAmazonIssued,
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want1: []*acmModel.Certificate{
				{
					ResourceMeta: core.NewResourceMeta(stack, "AWS::ACM::Certificate", "id-2"),
					Spec: acmModel.CertificateSpec{
						Type:                    acmtypes.CertificateTypeAmazonIssued,
						DomainName:              "example.com",
						SubjectAlternativeNames: []string{"example.com", "otherexample.com"},
						ValidationMethod:        acmtypes.ValidationMethodDns,
					},
				},
			},
			want2: []CertificateWithTags{
				{
					Certificate: &acmtypes.CertificateSummary{
						CertificateArn:                  awssdk.String("arn-2"),
						DomainName:                      awssdk.String("example.com"),
						SubjectAlternativeNameSummaries: []string{"example.com"},
						Status:                          acmtypes.CertificateStatusIssued,
						Type:                            acmtypes.CertificateTypeAmazonIssued,
					},
					Tags: map[string]string{
						"ingress.k8s.aws/resource": "id-2",
					},
				},
			},
		},
		{
			name: "certificate with pending validation but age smaller than reissueWaitTime",
			args: args{
				resCerts: []*acmModel.Certificate{
					{
						ResourceMeta: core.NewResourceMeta(stack, "AWS::ACM::Certificate", "id-3"),
						Spec: acmModel.CertificateSpec{
							Type:                    acmtypes.CertificateTypeAmazonIssued,
							DomainName:              "example.com",
							SubjectAlternativeNames: []string{"example.com"},
							ValidationMethod:        acmtypes.ValidationMethodDns,
						},
					},
				},
				sdkCerts: []CertificateWithTags{
					{
						Certificate: &acmtypes.CertificateSummary{
							CertificateArn:                  awssdk.String("arn-3"),
							DomainName:                      awssdk.String("example.com"),
							SubjectAlternativeNameSummaries: []string{"example.com"},
							Type:                            acmtypes.CertificateTypeAmazonIssued,
							Status:                          acmtypes.CertificateStatusPendingValidation,
							CreatedAt:                       awssdk.Time(createdAtTimeNow), // at the time the test runs, this cert should be yong enough to add it as match
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-3",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: []resAndSDKCertificatePair{
				{
					resCert: &acmModel.Certificate{
						ResourceMeta: core.NewResourceMeta(stack, "AWS::ACM::Certificate", "id-3"),
						Spec: acmModel.CertificateSpec{
							Type:                    acmtypes.CertificateTypeAmazonIssued,
							DomainName:              "example.com",
							SubjectAlternativeNames: []string{"example.com"},
							ValidationMethod:        acmtypes.ValidationMethodDns,
						},
					},
					sdkCert: CertificateWithTags{
						Certificate: &acmtypes.CertificateSummary{
							CertificateArn:                  awssdk.String("arn-3"),
							DomainName:                      awssdk.String("example.com"),
							SubjectAlternativeNameSummaries: []string{"example.com"},
							Type:                            acmtypes.CertificateTypeAmazonIssued,
							Status:                          acmtypes.CertificateStatusPendingValidation,
							CreatedAt:                       awssdk.Time(createdAtTimeNow),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-3",
						},
					},
				},
			},
		},

		{
			name: "certificate with pending validation and age older than reissueWaitTime",
			args: args{
				resCerts: []*acmModel.Certificate{
					{
						ResourceMeta: core.NewResourceMeta(stack, "AWS::ACM::Certificate", "id-4"),
						Spec: acmModel.CertificateSpec{
							Type:                    acmtypes.CertificateTypeAmazonIssued,
							DomainName:              "example.com",
							SubjectAlternativeNames: []string{"example.com"},
							ValidationMethod:        acmtypes.ValidationMethodDns,
						},
					},
				},
				sdkCerts: []CertificateWithTags{
					{
						Certificate: &acmtypes.CertificateSummary{
							CertificateArn:                  awssdk.String("arn-4"),
							DomainName:                      awssdk.String("example.com"),
							SubjectAlternativeNameSummaries: []string{"example.com"},
							Type:                            acmtypes.CertificateTypeAmazonIssued,
							Status:                          acmtypes.CertificateStatusPendingValidation,
							CreatedAt:                       awssdk.Time(createdAtTimeBefore), // older than reissueWaitTime and still pending -> trigger recreate
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-4",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want1: []*acmModel.Certificate{
				{
					ResourceMeta: core.NewResourceMeta(stack, "AWS::ACM::Certificate", "id-4"),
					Spec: acmModel.CertificateSpec{
						Type:                    acmtypes.CertificateTypeAmazonIssued,
						DomainName:              "example.com",
						SubjectAlternativeNames: []string{"example.com"},
						ValidationMethod:        acmtypes.ValidationMethodDns,
					},
				},
			},
			want2: []CertificateWithTags{
				{
					Certificate: &acmtypes.CertificateSummary{
						CertificateArn:                  awssdk.String("arn-4"),
						DomainName:                      awssdk.String("example.com"),
						SubjectAlternativeNameSummaries: []string{"example.com"},
						Type:                            acmtypes.CertificateTypeAmazonIssued,
						Status:                          acmtypes.CertificateStatusPendingValidation,
						CreatedAt:                       awssdk.Time(createdAtTimeBefore),
					},
					Tags: map[string]string{
						"ingress.k8s.aws/resource": "id-4",
					},
				},
			},
		},
		{
			name: "certificate amazon issued but want private one",
			args: args{
				resCerts: []*acmModel.Certificate{
					{
						ResourceMeta: core.NewResourceMeta(stack, "AWS::ACM::Certificate", "private/example.com"),
						Spec: acmModel.CertificateSpec{
							Type:                    acmtypes.CertificateTypePrivate,
							CertificateAuthorityARN: "arn:aws:acm-pca:::::my-ca",
							DomainName:              "example.com",
							SubjectAlternativeNames: []string{"example.com"},
						},
					},
				},
				sdkCerts: []CertificateWithTags{
					{
						Certificate: &acmtypes.CertificateSummary{
							CertificateArn:                  awssdk.String("arn-5"),
							DomainName:                      awssdk.String("example.com"),
							SubjectAlternativeNameSummaries: []string{"example.com"},
							Type:                            acmtypes.CertificateTypeAmazonIssued,
							Status:                          acmtypes.CertificateStatusIssued,
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "amazon_issued/example.com",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want1: []*acmModel.Certificate{
				{
					ResourceMeta: core.NewResourceMeta(stack, "AWS::ACM::Certificate", "private/example.com"),
					Spec: acmModel.CertificateSpec{
						Type:                    acmtypes.CertificateTypePrivate,
						CertificateAuthorityARN: "arn:aws:acm-pca:::::my-ca",
						DomainName:              "example.com",
						SubjectAlternativeNames: []string{"example.com"},
					},
				},
			},
			want2: []CertificateWithTags{
				{
					Certificate: &acmtypes.CertificateSummary{
						CertificateArn:                  awssdk.String("arn-5"),
						DomainName:                      awssdk.String("example.com"),
						SubjectAlternativeNameSummaries: []string{"example.com"},
						Type:                            acmtypes.CertificateTypeAmazonIssued,
						Status:                          acmtypes.CertificateStatusIssued,
					},
					Tags: map[string]string{
						"ingress.k8s.aws/resource": "amazon_issued/example.com",
					},
				},
			},
		},

		{
			name: "certificate domainname changes",
			args: args{
				resCerts: []*acmModel.Certificate{
					{
						ResourceMeta: core.NewResourceMeta(stack, "AWS::ACM::Certificate", "private/otherexample.com"),
						Spec: acmModel.CertificateSpec{
							Type:                    acmtypes.CertificateTypePrivate,
							CertificateAuthorityARN: "arn:aws:acm-pca:::::my-ca",
							DomainName:              "otherexample.com",
							SubjectAlternativeNames: []string{"otherexample.com"},
						},
					},
				},
				sdkCerts: []CertificateWithTags{
					{
						Certificate: &acmtypes.CertificateSummary{
							CertificateArn:                  awssdk.String("arn-6"),
							DomainName:                      awssdk.String("example.com"),
							SubjectAlternativeNameSummaries: []string{"example.com"},
							Type:                            acmtypes.CertificateTypePrivate,
							Status:                          acmtypes.CertificateStatusIssued,
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "private/example.com",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want1: []*acmModel.Certificate{
				{
					ResourceMeta: core.NewResourceMeta(stack, "AWS::ACM::Certificate", "private/otherexample.com"),
					Spec: acmModel.CertificateSpec{
						Type:                    acmtypes.CertificateTypePrivate,
						CertificateAuthorityARN: "arn:aws:acm-pca:::::my-ca",
						DomainName:              "otherexample.com",
						SubjectAlternativeNames: []string{"otherexample.com"},
					},
				},
			},
			want2: []CertificateWithTags{
				{
					Certificate: &acmtypes.CertificateSummary{
						CertificateArn:                  awssdk.String("arn-6"),
						DomainName:                      awssdk.String("example.com"),
						SubjectAlternativeNameSummaries: []string{"example.com"},
						Type:                            acmtypes.CertificateTypePrivate,
						Status:                          acmtypes.CertificateStatusIssued,
					},
					Tags: map[string]string{
						"ingress.k8s.aws/resource": "private/example.com",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, got2, err := matchResAndSDKCertificates(tt.args.resCerts, tt.args.sdkCerts, tt.args.resourceIDTagKey)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
				assert.Equal(t, tt.want1, got1)
				assert.Equal(t, tt.want2, got2)
			}
		})
	}
}
