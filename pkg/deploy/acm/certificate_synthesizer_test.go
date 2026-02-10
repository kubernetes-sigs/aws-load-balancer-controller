package acm

import (
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	acmModel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/acm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_Synthesizer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := logr.New(&log.NullLogSink{})

	tests := []struct {
		name                     string
		defaultCaArn             string
		setup                    func(core.Stack, *services.MockACM, *services.MockRoute53, *tracking.MockProvider)
		checkStack               func(s core.Stack)
		wantErr                  error
		wantToDeleteCertificates []CertificateWithTags
	}{
		{
			name: "happy path with simple amazon issued certificate",
			setup: func(s core.Stack, mockACM *services.MockACM, mockRoute53 *services.MockRoute53, mockTracking *tracking.MockProvider) {
				acmModel.NewCertificate(s, "amazon_issued/example.com", acmModel.CertificateSpec{
					Type:                    acmtypes.CertificateTypeAmazonIssued,
					DomainName:              "example.com",
					SubjectAlternativeNames: []string{"example.com"},
					ValidationMethod:        acmtypes.ValidationMethodDns,
					Tags:                    map[string]string{},
				})

				mockTracking.EXPECT().StackTags(gomock.Any()).Return(map[string]string{"foo": "bar"})

				mockTracking.EXPECT().StackTagsLegacy(gomock.Any()).Return(map[string]string(nil))

				mockACM.EXPECT().ListCertificatesAsList(gomock.Any(), gomock.Eq(&acm.ListCertificatesInput{})).
					Return([]acmtypes.CertificateSummary{}, nil)

				mockTracking.EXPECT().ResourceIDTagKey().Return("foo")

				mockTracking.EXPECT().ResourceTags(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(map[string]string{"foo": "bar"})

				mockACM.EXPECT().RequestCertificateWithContext(gomock.Any(), gomock.Eq(&acm.RequestCertificateInput{
					DomainName:              awssdk.String("example.com"),
					ValidationMethod:        acmtypes.ValidationMethodDns,
					SubjectAlternativeNames: []string{"example.com"},
					Tags:                    []acmtypes.Tag{{Key: awssdk.String("foo"), Value: awssdk.String("bar")}},
				})).Return(&acm.RequestCertificateOutput{CertificateArn: awssdk.String("arn-1")}, nil)

				mockACM.EXPECT().DescribeCertificateWithContext(gomock.Any(), gomock.Eq(&acm.DescribeCertificateInput{
					CertificateArn: awssdk.String("arn-1"),
				})).Return(&acm.DescribeCertificateOutput{
					Certificate: &acmtypes.CertificateDetail{
						DomainValidationOptions: []acmtypes.DomainValidation{
							{
								ValidationMethod: acmtypes.ValidationMethodDns,
								DomainName:       awssdk.String("example.com"),
								ResourceRecord: &acmtypes.ResourceRecord{
									Name:  awssdk.String("cname-name"),
									Value: awssdk.String("cname-value"),
									Type:  acmtypes.RecordTypeCname,
								},
							},
						},
					},
				}, nil)

				mockRoute53.EXPECT().GetHostedZoneID(gomock.Any(), gomock.Eq("example.com")).Return(awssdk.String("Z0382403B3S5MSK4SVXX"), nil)

				mockRoute53.EXPECT().ChangeRecordsWithContext(gomock.Any(), gomock.Eq(&route53.ChangeResourceRecordSetsInput{
					HostedZoneId: awssdk.String("Z0382403B3S5MSK4SVXX"),
					ChangeBatch: &route53types.ChangeBatch{
						Changes: []route53types.Change{
							{
								Action: "CREATE",
								ResourceRecordSet: &route53types.ResourceRecordSet{
									Name: awssdk.String("cname-name"),
									Type: route53types.RRType(acmtypes.RecordTypeCname),
									TTL:  awssdk.Int64(validationRecordTTL),
									ResourceRecords: []route53types.ResourceRecord{
										{Value: awssdk.String("cname-value")},
									},
								},
							},
						},
					},
				})).Return(nil, nil)

				mockACM.EXPECT().WaitForCertificateIssuedWithContext(gomock.Any(), gomock.Eq("arn-1"), gomock.AssignableToTypeOf(time.Second)).Return(nil)
			},
			checkStack: func(s core.Stack) {
				var resCerts []*acmModel.Certificate
				err := s.ListResources(&resCerts)
				assert.NoError(t, err)
				assert.Len(t, resCerts, 1)
				arn, err := resCerts[0].CertificateARN().Resolve(t.Context())
				assert.NoError(t, err)
				assert.Equal(t, arn, "arn-1")
			},
			wantErr:                  nil,
			wantToDeleteCertificates: []CertificateWithTags(nil),
		},
		{
			name: "happy path with existing certificate",
			setup: func(s core.Stack, mockACM *services.MockACM, mockRoute53 *services.MockRoute53, mockTracking *tracking.MockProvider) {
				acmModel.NewCertificate(s, "amazon_issued/example.com", acmModel.CertificateSpec{
					Type:                    acmtypes.CertificateTypeAmazonIssued,
					DomainName:              "example.com",
					SubjectAlternativeNames: []string{"example.com"},
					ValidationMethod:        acmtypes.ValidationMethodDns,
					Tags:                    map[string]string{},
				})

				mockTracking.EXPECT().StackTags(gomock.Any()).Return(map[string]string{"foo": "bar"})

				mockTracking.EXPECT().StackTagsLegacy(gomock.Any()).Return(map[string]string(nil))

				mockACM.EXPECT().ListCertificatesAsList(gomock.Any(), gomock.Eq(&acm.ListCertificatesInput{})).
					Return([]acmtypes.CertificateSummary{{
						CertificateArn:                  awssdk.String("arn-1"),
						DomainName:                      awssdk.String("example.com"),
						SubjectAlternativeNameSummaries: []string{"example.com"},
						Type:                            acmtypes.CertificateTypeAmazonIssued,
						Status:                          acmtypes.CertificateStatusIssued,
					}}, nil)

				mockACM.EXPECT().ListTagsForCertificate(gomock.Any(), gomock.Eq(&acm.ListTagsForCertificateInput{
					CertificateArn: awssdk.String("arn-1"),
				})).
					Return(&acm.ListTagsForCertificateOutput{Tags: []acmtypes.Tag{
						{
							Key:   awssdk.String("foo"),
							Value: awssdk.String("amazon_issued/example.com"),
						},
					}}, nil)

				mockTracking.EXPECT().ResourceIDTagKey().Return("foo")
				// no further describe/wait calls needed as the required cert is already present
			},
			checkStack: func(s core.Stack) {
				var resCerts []*acmModel.Certificate
				err := s.ListResources(&resCerts)
				assert.NoError(t, err)
				assert.Len(t, resCerts, 1)
				arn, err := resCerts[0].CertificateARN().Resolve(t.Context())
				assert.NoError(t, err)
				assert.Equal(t, arn, "arn-1")
			},
			wantErr:                  nil,
			wantToDeleteCertificates: []CertificateWithTags(nil),
		},
		{
			name: "existing certificate but not enough SANs",
			setup: func(s core.Stack, mockACM *services.MockACM, mockRoute53 *services.MockRoute53, mockTracking *tracking.MockProvider) {
				// add something to the stack
				acmModel.NewCertificate(s, "amazon_issued/example.com", acmModel.CertificateSpec{
					Type:                    acmtypes.CertificateTypeAmazonIssued,
					DomainName:              "example.com",
					SubjectAlternativeNames: []string{"example.com", "otherexample.com"},
					ValidationMethod:        acmtypes.ValidationMethodDns,
					Tags:                    map[string]string{},
				})

				mockTracking.EXPECT().StackTags(gomock.Any()).Return(map[string]string{"foo": "bar"})

				mockTracking.EXPECT().StackTagsLegacy(gomock.Any()).Return(map[string]string(nil))

				mockACM.EXPECT().ListCertificatesAsList(gomock.Any(), gomock.Eq(&acm.ListCertificatesInput{})).
					Return([]acmtypes.CertificateSummary{{
						CertificateArn:                  awssdk.String("arn-1"),
						DomainName:                      awssdk.String("example.com"),
						SubjectAlternativeNameSummaries: []string{"example.com"},
						Type:                            acmtypes.CertificateTypeAmazonIssued,
						Status:                          acmtypes.CertificateStatusIssued,
					}}, nil)

				mockACM.EXPECT().ListTagsForCertificate(gomock.Any(), gomock.Eq(&acm.ListTagsForCertificateInput{
					CertificateArn: awssdk.String("arn-1"),
				})).
					Return(&acm.ListTagsForCertificateOutput{Tags: []acmtypes.Tag{
						{
							Key:   awssdk.String("foo"),
							Value: awssdk.String("amazon_issued/example.com"),
						},
					}}, nil)

				mockTracking.EXPECT().ResourceIDTagKey().Return("foo")

				// calls for requesting a replacment cert
				mockTracking.EXPECT().ResourceTags(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(map[string]string{"foo": "bar"})

				mockACM.EXPECT().RequestCertificateWithContext(gomock.Any(), gomock.Eq(&acm.RequestCertificateInput{
					DomainName:              awssdk.String("example.com"),
					ValidationMethod:        acmtypes.ValidationMethodDns,
					SubjectAlternativeNames: []string{"example.com", "otherexample.com"},
					Tags:                    []acmtypes.Tag{{Key: awssdk.String("foo"), Value: awssdk.String("bar")}},
				})).Return(&acm.RequestCertificateOutput{CertificateArn: awssdk.String("arn-2")}, nil)

				mockACM.EXPECT().DescribeCertificateWithContext(gomock.Any(), gomock.Eq(&acm.DescribeCertificateInput{
					CertificateArn: awssdk.String("arn-2"),
				})).Return(&acm.DescribeCertificateOutput{
					Certificate: &acmtypes.CertificateDetail{DomainValidationOptions: []acmtypes.DomainValidation{}},
				}, nil)
				//
				// no call to route53 since we returned empty domain validation options
				mockACM.EXPECT().WaitForCertificateIssuedWithContext(gomock.Any(), gomock.Eq("arn-2"), gomock.AssignableToTypeOf(time.Second)).Return(nil)
			},
			checkStack: func(s core.Stack) {
				var resCerts []*acmModel.Certificate
				err := s.ListResources(&resCerts)
				assert.NoError(t, err)
				assert.Len(t, resCerts, 1)
				arn, err := resCerts[0].CertificateARN().Resolve(t.Context())
				assert.NoError(t, err)
				assert.Equal(t, arn, "arn-2")
			},
			wantErr: nil,
			wantToDeleteCertificates: []CertificateWithTags{{
				Certificate: &acmtypes.CertificateSummary{
					CertificateArn:                  awssdk.String("arn-1"),
					DomainName:                      awssdk.String("example.com"),
					SubjectAlternativeNameSummaries: []string{"example.com"},
					Status:                          acmtypes.CertificateStatusIssued,
					Type:                            acmtypes.CertificateTypeAmazonIssued,
				},
				Tags: map[string]string{"foo": "amazon_issued/example.com"},
			}},
		},
		{
			name: "existing certificate but not issued within reissueWaitTime",
			setup: func(s core.Stack, mockACM *services.MockACM, mockRoute53 *services.MockRoute53, mockTracking *tracking.MockProvider) {
				// add something to the stack
				acmModel.NewCertificate(s, "amazon_issued/example.com", acmModel.CertificateSpec{
					Type:                    acmtypes.CertificateTypeAmazonIssued,
					DomainName:              "example.com",
					SubjectAlternativeNames: []string{"example.com"},
					ValidationMethod:        acmtypes.ValidationMethodDns,
					Tags:                    map[string]string{},
				})

				mockTracking.EXPECT().StackTags(gomock.Any()).Return(map[string]string{"foo": "bar"})

				mockTracking.EXPECT().StackTagsLegacy(gomock.Any()).Return(map[string]string(nil))

				mockACM.EXPECT().ListCertificatesAsList(gomock.Any(), gomock.Eq(&acm.ListCertificatesInput{})).
					Return([]acmtypes.CertificateSummary{{
						CertificateArn:                  awssdk.String("arn-1"),
						DomainName:                      awssdk.String("example.com"),
						SubjectAlternativeNameSummaries: []string{"example.com"},
						Type:                            acmtypes.CertificateTypeAmazonIssued,
						CreatedAt:                       awssdk.Time(time.Now().Add(-reissueWaitTime)),
						Status:                          acmtypes.CertificateStatusPendingValidation,
					}}, nil)

				mockACM.EXPECT().ListTagsForCertificate(gomock.Any(), gomock.Eq(&acm.ListTagsForCertificateInput{
					CertificateArn: awssdk.String("arn-1"),
				})).
					Return(&acm.ListTagsForCertificateOutput{Tags: []acmtypes.Tag{
						{
							Key:   awssdk.String("foo"),
							Value: awssdk.String("amazon_issued/example.com"),
						},
					}}, nil)

				mockTracking.EXPECT().ResourceIDTagKey().Return("foo")

				// calls for requesting a replacment cert
				mockTracking.EXPECT().ResourceTags(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(map[string]string{"foo": "bar"})

				// the describe before the delete
				mockACM.EXPECT().DescribeCertificateWithContext(gomock.Any(), gomock.Eq(&acm.DescribeCertificateInput{
					CertificateArn: awssdk.String("arn-1"),
				})).Return(&acm.DescribeCertificateOutput{
					Certificate: &acmtypes.CertificateDetail{DomainValidationOptions: []acmtypes.DomainValidation{}},
				}, nil)

				mockACM.EXPECT().DeleteCertificateWithContext(gomock.Any(), gomock.Eq(&acm.DeleteCertificateInput{CertificateArn: awssdk.String("arn-1")})).Return(&acm.DeleteCertificateOutput{}, nil)

				mockACM.EXPECT().RequestCertificateWithContext(gomock.Any(), gomock.Eq(&acm.RequestCertificateInput{
					DomainName:              awssdk.String("example.com"),
					ValidationMethod:        acmtypes.ValidationMethodDns,
					SubjectAlternativeNames: []string{"example.com"},
					Tags:                    []acmtypes.Tag{{Key: awssdk.String("foo"), Value: awssdk.String("bar")}},
				})).Return(&acm.RequestCertificateOutput{CertificateArn: awssdk.String("arn-2")}, nil)

				mockACM.EXPECT().DescribeCertificateWithContext(gomock.Any(), gomock.Eq(&acm.DescribeCertificateInput{
					CertificateArn: awssdk.String("arn-2"),
				})).Return(&acm.DescribeCertificateOutput{
					Certificate: &acmtypes.CertificateDetail{DomainValidationOptions: []acmtypes.DomainValidation{}},
				}, nil)
				//
				// no call to route53 since we returned empty domain validation options
				mockACM.EXPECT().WaitForCertificateIssuedWithContext(gomock.Any(), gomock.Eq("arn-2"), gomock.AssignableToTypeOf(time.Second)).Return(nil)
			},
			checkStack: func(s core.Stack) {
				var resCerts []*acmModel.Certificate
				err := s.ListResources(&resCerts)
				assert.NoError(t, err)
				assert.Len(t, resCerts, 1)
				arn, err := resCerts[0].CertificateARN().Resolve(t.Context())
				assert.NoError(t, err)
				assert.Equal(t, arn, "arn-2")
			},
			wantErr:                  nil,
			wantToDeleteCertificates: []CertificateWithTags(nil),
		},
		{
			name: "no certificate, cleaning orphaned resources",
			setup: func(s core.Stack, mockACM *services.MockACM, mockRoute53 *services.MockRoute53, mockTracking *tracking.MockProvider) {
				mockTracking.EXPECT().StackTags(gomock.Any()).Return(map[string]string{"foo": "bar"})

				mockTracking.EXPECT().StackTagsLegacy(gomock.Any()).Return(map[string]string(nil))

				mockACM.EXPECT().ListCertificatesAsList(gomock.Any(), gomock.Eq(&acm.ListCertificatesInput{})).
					Return([]acmtypes.CertificateSummary{{
						CertificateArn:                  awssdk.String("arn-1"),
						DomainName:                      awssdk.String("example.com"),
						SubjectAlternativeNameSummaries: []string{"example.com"},
						Type:                            acmtypes.CertificateTypeAmazonIssued,
						Status:                          acmtypes.CertificateStatusIssued,
					}}, nil)

				mockACM.EXPECT().ListTagsForCertificate(gomock.Any(), gomock.Eq(&acm.ListTagsForCertificateInput{
					CertificateArn: awssdk.String("arn-1"),
				})).
					Return(&acm.ListTagsForCertificateOutput{
						Tags: []acmtypes.Tag{{
							Key:   awssdk.String("foo"),
							Value: awssdk.String("amazon_issued/example.com"),
						}},
					}, nil)

				mockTracking.EXPECT().ResourceIDTagKey().Return("foo")
			},
			checkStack: func(s core.Stack) {
				var resCerts []*acmModel.Certificate
				err := s.ListResources(&resCerts)
				assert.NoError(t, err)
				assert.Len(t, resCerts, 0)
			},
			wantErr: nil,
			wantToDeleteCertificates: []CertificateWithTags{{
				Certificate: &acmtypes.CertificateSummary{
					CertificateArn:                  awssdk.String("arn-1"),
					DomainName:                      awssdk.String("example.com"),
					SubjectAlternativeNameSummaries: []string{"example.com"},
					Status:                          acmtypes.CertificateStatusIssued,
					Type:                            acmtypes.CertificateTypeAmazonIssued,
				},
				Tags: map[string]string{"foo": "amazon_issued/example.com"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "name"})
			mockACM := services.NewMockACM(ctrl)
			mockRoute53 := services.NewMockRoute53(ctrl)
			mockTracking := tracking.NewMockProvider(ctrl)

			// Setup stack and expectations
			tt.setup(s, mockACM, mockRoute53, mockTracking)

			m := &defaultCertificateManager{
				acmClient:        mockACM,
				route53Client:    mockRoute53,
				logger:           logger,
				trackingProvider: mockTracking,
			}

			if tt.defaultCaArn != "" {
				m.defaultCaArn = tt.defaultCaArn
			}

			c := &certificateSynthesizer{
				certificateManager: m,
				logger:             logger,
				stack:              s,
				taggingManager: &defaultTaggingManager{
					acmClient:            mockACM,
					logger:               logger,
					resourceTagsCache:    cache.NewExpiring(),
					resourceTagsCacheTTL: defaultResourceTagsCacheTTL,
				},
				trackingProvider: mockTracking,
			}

			err := c.Synthesize(t.Context())

			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, tt.wantErr, err.Error())
			}
			assert.Equal(t, tt.wantToDeleteCertificates, c.toDeleteCerts)

			tt.checkStack(s)
		})
	}
}

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
			want: []resAndSDKCertificatePair{
				{
					resCert: &acmModel.Certificate{
						ResourceMeta: core.NewResourceMeta(stack, "AWS::ACM::Certificate", "id-4"),
						Spec: acmModel.CertificateSpec{
							Type:                    acmtypes.CertificateTypeAmazonIssued,
							DomainName:              "example.com",
							SubjectAlternativeNames: []string{"example.com"},
							ValidationMethod:        acmtypes.ValidationMethodDns,
						},
					},
					sdkCert: CertificateWithTags{
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
