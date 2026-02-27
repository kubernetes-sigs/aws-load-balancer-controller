package certs

import (
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmTypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

func Test_acmCertDiscovery_loadAllCertificateARNs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockACM := services.NewMockACM(ctrl)

	type cacheItem struct {
		key                string
		value              []string
		checkExistenceOnly bool
	}

	tests := []struct {
		name                        string
		setupExpectations           func()
		filterTags                  map[string]string
		enableCertificateManagement bool
		want                        []string
		wantErr                     bool
	}{
		{
			name: "load cert arns regularly",
			setupExpectations: func() {
				mockACM.EXPECT().ListCertificatesAsList(gomock.Any(), gomock.Eq(&acm.ListCertificatesInput{
					CertificateStatuses: []acmtypes.CertificateStatus{acmtypes.CertificateStatusIssued},
					Includes: &acmTypes.Filters{
						KeyTypes: acmTypes.KeyAlgorithm.Values(""),
					},
				})).
					Return([]acmtypes.CertificateSummary{{
						CertificateArn: awssdk.String("arn:aws:acm:eu-central-1:134051052098:certificate/0983b834-dc36-4253-8f8c-2e21525d1185"),
					}}, nil)
			},
			want: []string{"arn:aws:acm:eu-central-1:134051052098:certificate/0983b834-dc36-4253-8f8c-2e21525d1185"},
		},
		{
			name: "load cert arns with filterTags filter",
			setupExpectations: func() {
				mockACM.EXPECT().ListCertificatesAsList(gomock.Any(), gomock.Eq(&acm.ListCertificatesInput{
					CertificateStatuses: []acmtypes.CertificateStatus{acmtypes.CertificateStatusIssued},
					Includes: &acmTypes.Filters{
						KeyTypes: acmTypes.KeyAlgorithm.Values(""),
					},
				})).
					Return([]acmtypes.CertificateSummary{
						{
							CertificateArn: awssdk.String("arn:aws:acm:eu-central-1:134051052098:certificate/0983b834-dc36-4253-8f8c-2e21525d1185"),
						},
					}, nil)

				mockACM.EXPECT().ListTagsForCertificate(gomock.Any(), gomock.Eq(&acm.ListTagsForCertificateInput{CertificateArn: awssdk.String("arn:aws:acm:eu-central-1:134051052098:certificate/0983b834-dc36-4253-8f8c-2e21525d1185")})).Return(&acm.ListTagsForCertificateOutput{Tags: []acmTypes.Tag{{Key: awssdk.String("foo"), Value: awssdk.String("bar")}}}, nil)
			},
			enableCertificateManagement: true,
			filterTags:                  map[string]string{"foo": "bar"},
			want:                        []string(nil),
		},
		{
			name: "load cert arns with filterTags filter, partial matches",
			setupExpectations: func() {
				mockACM.EXPECT().ListCertificatesAsList(gomock.Any(), gomock.Eq(&acm.ListCertificatesInput{
					CertificateStatuses: []acmtypes.CertificateStatus{acmtypes.CertificateStatusIssued},
					Includes: &acmTypes.Filters{
						KeyTypes: acmTypes.KeyAlgorithm.Values(""),
					},
				})).
					Return([]acmtypes.CertificateSummary{
						{CertificateArn: awssdk.String("arn:aws:acm:eu-central-1:134051052098:certificate/0983b834-dc36-4253-8f8c-2e21525d1185")},
						{CertificateArn: awssdk.String("arn:aws:acm:eu-central-1:134051052098:certificate/2d5aa6f7-7d3a-4e81-8202-c6d87c36140b")},
						{CertificateArn: awssdk.String("arn:aws:acm:eu-central-1:134051052098:certificate/d2c0c245-776c-4177-ad8c-9c892826fcb7")},
					}, nil)

				mockACM.EXPECT().ListTagsForCertificate(gomock.Any(), gomock.Eq(&acm.ListTagsForCertificateInput{CertificateArn: awssdk.String("arn:aws:acm:eu-central-1:134051052098:certificate/0983b834-dc36-4253-8f8c-2e21525d1185")})).Return(&acm.ListTagsForCertificateOutput{Tags: []acmTypes.Tag{{Key: awssdk.String("foo"), Value: awssdk.String("bar")}}}, nil)
				mockACM.EXPECT().ListTagsForCertificate(gomock.Any(), gomock.Eq(&acm.ListTagsForCertificateInput{CertificateArn: awssdk.String("arn:aws:acm:eu-central-1:134051052098:certificate/2d5aa6f7-7d3a-4e81-8202-c6d87c36140b")})).Return(&acm.ListTagsForCertificateOutput{Tags: []acmTypes.Tag(nil)}, nil)
				mockACM.EXPECT().ListTagsForCertificate(gomock.Any(), gomock.Eq(&acm.ListTagsForCertificateInput{CertificateArn: awssdk.String("arn:aws:acm:eu-central-1:134051052098:certificate/d2c0c245-776c-4177-ad8c-9c892826fcb7")})).Return(&acm.ListTagsForCertificateOutput{Tags: []acmTypes.Tag{{Key: awssdk.String("foo"), Value: awssdk.String("somethingelse")}}}, nil)
			},
			enableCertificateManagement: true,
			filterTags:                  map[string]string{"foo": "bar"},
			want:                        []string{"arn:aws:acm:eu-central-1:134051052098:certificate/2d5aa6f7-7d3a-4e81-8202-c6d87c36140b", "arn:aws:acm:eu-central-1:134051052098:certificate/d2c0c245-776c-4177-ad8c-9c892826fcb7"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup expectations
			tt.setupExpectations()

			d := &acmCertDiscovery{
				acmClient:                   mockACM,
				certARNsCache:               cache.NewExpiring(),
				certARNsCacheTTL:            defaultCertARNsCacheTTL,
				enableCertificateManagement: tt.enableCertificateManagement,
			}

			got, err := d.loadAllCertificateARNs(t.Context(), tt.filterTags)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
				got, ok := d.certARNsCache.Get(certARNsCacheKey)
				assert.True(t, ok)
				assert.Equal(t, tt.want, got.([]string))
			}
		})
	}
}

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
