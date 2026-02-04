package acm

import (
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_defaultTaggingManager_ListCertificates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockACM := services.NewMockACM(ctrl)

	type cacheItem struct {
		key                string
		value              map[string]string
		checkExistenceOnly bool
	}

	tests := []struct {
		name              string
		arns              []string
		setupExpectations func()
		wantCacheItems    []cacheItem
		tagFilters        []tracking.TagFilter
		want              []CertificateWithTags
		wantErr           bool
	}{
		{
			name: "successfully retrieve tags from ACM",
			setupExpectations: func() {
				mockACM.EXPECT().ListCertificatesAsList(gomock.Any(), gomock.Eq(&acm.ListCertificatesInput{})).
					Return([]acmtypes.CertificateSummary{{
						CertificateArn: awssdk.String("arn:aws:acm:eu-central-1:134051052098:certificate/0983b834-dc36-4253-8f8c-2e21525d1185"),
					}}, nil)
				mockACM.EXPECT().
					ListTagsForCertificate(gomock.Any(), gomock.Eq(&acm.ListTagsForCertificateInput{
						CertificateArn: awssdk.String("arn:aws:acm:eu-central-1:134051052098:certificate/0983b834-dc36-4253-8f8c-2e21525d1185"),
					})).
					Return(&acm.ListTagsForCertificateOutput{
						Tags: []acmtypes.Tag{
							{
								Key:   awssdk.String("elbv2.k8s.aws/cluster"),
								Value: awssdk.String("example"),
							},
							{
								Key:   awssdk.String("elbv2.k8s.aws/stack"),
								Value: awssdk.String("default/name"),
							},
						},
					}, nil)
			},
			tagFilters: []tracking.TagFilter{{"elbv2.k8s.aws/cluster": []string{}}},
			want: []CertificateWithTags{
				{
					Certificate: &acmtypes.CertificateSummary{CertificateArn: awssdk.String("arn:aws:acm:eu-central-1:134051052098:certificate/0983b834-dc36-4253-8f8c-2e21525d1185")},
					Tags: map[string]string{
						"elbv2.k8s.aws/cluster": "example",
						"elbv2.k8s.aws/stack":   "default/name",
					},
				},
			},
			wantCacheItems: []cacheItem{
				{
					key: "arn:aws:acm:eu-central-1:134051052098:certificate/0983b834-dc36-4253-8f8c-2e21525d1185",
					value: map[string]string{
						"elbv2.k8s.aws/cluster": "example",
						"elbv2.k8s.aws/stack":   "default/name",
					},
				},
			},
		},
		{
			name: "list tags for certificate with wrong tagfilters",
			setupExpectations: func() {
				mockACM.EXPECT().ListCertificatesAsList(gomock.Any(), gomock.Eq(&acm.ListCertificatesInput{})).
					Return([]acmtypes.CertificateSummary{{
						CertificateArn: awssdk.String("arn:aws:acm:eu-central-1:134051052098:certificate/0983b834-dc36-4253-8f8c-2e21525d1185"),
					}}, nil)

				mockACM.EXPECT().
					ListTagsForCertificate(gomock.Any(), gomock.Eq(&acm.ListTagsForCertificateInput{
						CertificateArn: awssdk.String("arn:aws:acm:eu-central-1:134051052098:certificate/0983b834-dc36-4253-8f8c-2e21525d1185"),
					})).
					Return(&acm.ListTagsForCertificateOutput{
						Tags: []acmtypes.Tag{
							{
								Key:   awssdk.String("elbv2.k8s.aws/foo"),
								Value: awssdk.String("example"),
							},
							{
								Key:   awssdk.String("elbv2.k8s.aws/bar"),
								Value: awssdk.String("default/name"),
							},
						},
					}, nil)
			},
			tagFilters: []tracking.TagFilter{{"elbv2.k8s.aws/cluster": []string{}}},
			want:       []CertificateWithTags(nil),
		},
		{
			name: "empty certificates list",
			setupExpectations: func() {
				mockACM.EXPECT().ListCertificatesAsList(gomock.Any(), gomock.Eq(&acm.ListCertificatesInput{})).
					Return([]acmtypes.CertificateSummary{}, nil)
			},
			want: []CertificateWithTags(nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup expectations
			tt.setupExpectations()

			m := &defaultTaggingManager{
				acmClient:            mockACM,
				logger:               logr.New(&log.NullLogSink{}),
				resourceTagsCache:    cache.NewExpiring(),
				resourceTagsCacheTTL: defaultResourceTagsCacheTTL,
			}

			got, err := m.ListCertificates(t.Context(), tt.tagFilters...)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

			for _, item := range tt.wantCacheItems {
				got, ok := m.resourceTagsCache.Get(item.key)
				if item.checkExistenceOnly {
					assert.Equal(t, true, ok)
				} else {
					gotRaw := got.(map[string]string)
					assert.Equal(t, item.value, gotRaw)
				}
			}
		})
	}
}
