package ls

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"
)

type listCertificatesCall struct {
	input  *acm.ListCertificatesInput
	output []*acm.CertificateSummary
	err    error
}

type describeCertificateCall struct {
	certArn string
	output  *acm.CertificateDetail
	err     error
}

func Test_CertDiscovery_Discover(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		hosts                    []string
		listCertificateCall      *listCertificatesCall
		describeCertificateCalls []describeCertificateCall
		expectedCerts            []string
		expectedErr              string
	}{
		{
			name:  "when ACM has exact match with TLS host",
			hosts: []string{"foo.example.com"},
			listCertificateCall: &listCertificatesCall{
				input:  &acm.ListCertificatesInput{CertificateStatuses: aws.StringSlice([]string{acm.CertificateStatusIssued})},
				output: []*acm.CertificateSummary{{CertificateArn: aws.String("arn:aws:acm:us-west-2:xxx:certificate/yyy")}},
			},
			describeCertificateCalls: []describeCertificateCall{
				{
					certArn: "arn:aws:acm:us-west-2:xxx:certificate/yyy",
					output: &acm.CertificateDetail{
						SubjectAlternativeNames: aws.StringSlice([]string{"foo.example.com"}),
					},
				},
			},
			expectedCerts: []string{"arn:aws:acm:us-west-2:xxx:certificate/yyy"},
		},
		{
			name:  "when ACM has wildcard match with TLS host",
			hosts: []string{"foo.example.com"},
			listCertificateCall: &listCertificatesCall{
				input:  &acm.ListCertificatesInput{CertificateStatuses: aws.StringSlice([]string{acm.CertificateStatusIssued})},
				output: []*acm.CertificateSummary{{CertificateArn: aws.String("arn:aws:acm:us-west-2:xxx:certificate/yyy")}},
			},
			describeCertificateCalls: []describeCertificateCall{
				{
					certArn: "arn:aws:acm:us-west-2:xxx:certificate/yyy",
					output: &acm.CertificateDetail{
						SubjectAlternativeNames: aws.StringSlice([]string{"*.example.com"}),
					},
				},
			},
			expectedCerts: []string{"arn:aws:acm:us-west-2:xxx:certificate/yyy"},
		},
		{
			name:  "when ACM has SAN domain match with TLS host",
			hosts: []string{"foo.example.com"},
			listCertificateCall: &listCertificatesCall{
				input:  &acm.ListCertificatesInput{CertificateStatuses: aws.StringSlice([]string{acm.CertificateStatusIssued})},
				output: []*acm.CertificateSummary{{CertificateArn: aws.String("arn:aws:acm:us-west-2:xxx:certificate/yyy")}},
			},
			describeCertificateCalls: []describeCertificateCall{
				{
					certArn: "arn:aws:acm:us-west-2:xxx:certificate/yyy",
					output: &acm.CertificateDetail{
						SubjectAlternativeNames: aws.StringSlice([]string{"bar.example.com", "foo.example.com"}),
					},
				},
			},
			expectedCerts: []string{"arn:aws:acm:us-west-2:xxx:certificate/yyy"},
		},
		{
			name:  "when ACM has exact match with multiple TLS host",
			hosts: []string{"foo.example.com", "bar.example.com"},
			listCertificateCall: &listCertificatesCall{
				input: &acm.ListCertificatesInput{CertificateStatuses: aws.StringSlice([]string{acm.CertificateStatusIssued})},
				output: []*acm.CertificateSummary{
					{CertificateArn: aws.String("arn:aws:acm:us-west-2:xxx:certificate/yyy")},
					{CertificateArn: aws.String("arn:aws:acm:us-west-2:xxx:certificate/zzz")},
				},
			},
			describeCertificateCalls: []describeCertificateCall{
				{
					certArn: "arn:aws:acm:us-west-2:xxx:certificate/yyy",
					output: &acm.CertificateDetail{
						SubjectAlternativeNames: aws.StringSlice([]string{"foo.example.com"}),
					},
				},
				{
					certArn: "arn:aws:acm:us-west-2:xxx:certificate/zzz",
					output: &acm.CertificateDetail{
						SubjectAlternativeNames: aws.StringSlice([]string{"bar.example.com"}),
					},
				},
			},
			expectedCerts: []string{"arn:aws:acm:us-west-2:xxx:certificate/yyy", "arn:aws:acm:us-west-2:xxx:certificate/zzz"},
		},
		{
			name:  "when ACM has multiple match with TLS host",
			hosts: []string{"foo.example.com"},
			listCertificateCall: &listCertificatesCall{
				input: &acm.ListCertificatesInput{CertificateStatuses: aws.StringSlice([]string{acm.CertificateStatusIssued})},
				output: []*acm.CertificateSummary{
					{CertificateArn: aws.String("arn:aws:acm:us-west-2:xxx:certificate/yyy")},
					{CertificateArn: aws.String("arn:aws:acm:us-west-2:xxx:certificate/zzz")},
				},
			},
			describeCertificateCalls: []describeCertificateCall{
				{
					certArn: "arn:aws:acm:us-west-2:xxx:certificate/yyy",
					output: &acm.CertificateDetail{
						SubjectAlternativeNames: aws.StringSlice([]string{"foo.example.com"}),
					},
				},
				{
					certArn: "arn:aws:acm:us-west-2:xxx:certificate/zzz",
					output: &acm.CertificateDetail{
						SubjectAlternativeNames: aws.StringSlice([]string{"foo.example.com"}),
					},
				},
			},
			expectedCerts: nil,
			expectedErr:   "multiple certificate found for host: foo.example.com, certARNs: [arn:aws:acm:us-west-2:xxx:certificate/yyy arn:aws:acm:us-west-2:xxx:certificate/zzz]",
		},
		{
			name:  "when ACM has no match with TLS host",
			hosts: []string{"foo.example.com"},
			listCertificateCall: &listCertificatesCall{
				input: &acm.ListCertificatesInput{CertificateStatuses: aws.StringSlice([]string{acm.CertificateStatusIssued})},
				output: []*acm.CertificateSummary{
					{CertificateArn: aws.String("arn:aws:acm:us-west-2:xxx:certificate/yyy")},
				},
			},
			describeCertificateCalls: []describeCertificateCall{
				{
					certArn: "arn:aws:acm:us-west-2:xxx:certificate/yyy",
					output: &acm.CertificateDetail{
						SubjectAlternativeNames: aws.StringSlice([]string{"bar.example.com"}),
					},
				},
			},
			expectedCerts: nil,
			expectedErr:   "none certificate found for host: foo.example.com",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mockedCloud := &mocks.CloudAPI{}
			if tc.listCertificateCall != nil {
				mockedCloud.On("ListCertificates", ctx, tc.listCertificateCall.input).Return(tc.listCertificateCall.output, tc.listCertificateCall.err)
			}
			for _, call := range tc.describeCertificateCalls {
				mockedCloud.On("DescribeCertificate", ctx, call.certArn).Return(call.output, call.err)
			}

			certDiscovery := NewACMCertDiscovery(mockedCloud)
			certArns, err := certDiscovery.Discover(ctx, sets.NewString(tc.hosts...))
			if tc.expectedErr != "" {
				assert.EqualError(t, err, tc.expectedErr)
			} else {
				assert.Nil(t, err)
			}
			assert.ElementsMatch(t, certArns, tc.expectedCerts)
		})
	}
}

func Test_domainMatchesHost(t *testing.T) {
	var tests = []struct {
		domain string
		host   string
		want   bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "exampl0.com", false},

		// wildcards
		{"*.example.com", "foo.example.com", true},
		{"*.example.com", "example.com", false},
		{"*.exampl0.com", "foo.example.com", false},

		// invalid hosts, not sure these are possible
		{"*.*.example.com", "foo.bar.example.com", false},
		{"foo.*.example.com", "foo.bar.example.com", false},
	}

	for _, test := range tests {
		var msg = "should"
		if !test.want {
			msg = "should not"
		}

		d := &acmCertDiscovery{}
		t.Run(fmt.Sprintf("%s %s match %s", test.domain, msg, test.host), func(t *testing.T) {
			have := d.domainMatchesHost(test.domain, test.host)
			if test.want != have {
				t.Fail()
			}
		})
	}
}
