package acm

import (
	"context"
	"errors"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Split-horizon cleanup: the unfiltered lookup resolves the private zone (legacy
// records) while the public lookup resolves the public zone (records written by
// current controllers). Delete must attempt cleanup in both zones.
func TestDeleteWithValidationRecords_SplitHorizonCleansBothZones(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockACM := services.NewMockACM(ctrl)
	mockRoute53 := services.NewMockRoute53(ctrl)
	m := &defaultCertificateManager{
		acmClient:     mockACM,
		route53Client: mockRoute53,
		logger:        logr.New(&log.NullLogSink{}),
	}

	arn := "arn:aws:acm:us-east-1:123456789012:certificate/test"
	mockACM.EXPECT().DescribeCertificateWithContext(gomock.Any(), gomock.Eq(&acm.DescribeCertificateInput{
		CertificateArn: awssdk.String(arn),
	})).Return(&acm.DescribeCertificateOutput{
		Certificate: &acmtypes.CertificateDetail{
			DomainValidationOptions: []acmtypes.DomainValidation{
				{
					ValidationMethod: acmtypes.ValidationMethodDns,
					DomainName:       awssdk.String("app.sub.example.com"),
					ResourceRecord: &acmtypes.ResourceRecord{
						Name:  awssdk.String("cname-name"),
						Value: awssdk.String("cname-value"),
						Type:  acmtypes.RecordTypeCname,
					},
				},
			},
		},
	}, nil)

	mockRoute53.EXPECT().GetHostedZoneID(gomock.Any(), gomock.Eq("app.sub.example.com")).Return(awssdk.String("Z_PRIVATE"), nil)
	mockRoute53.EXPECT().GetPublicHostedZoneID(gomock.Any(), gomock.Eq("app.sub.example.com")).Return(awssdk.String("Z_PUBLIC"), nil)

	deleteInput := func(zoneID string) *route53.ChangeResourceRecordSetsInput {
		return &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: awssdk.String(zoneID),
			ChangeBatch: &route53types.ChangeBatch{
				Changes: []route53types.Change{
					{
						Action: "DELETE",
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
		}
	}
	// record was written to the public zone: private-zone delete fails "not found" (tolerated)
	mockRoute53.EXPECT().ChangeRecordsWithContext(gomock.Any(), gomock.Eq(deleteInput("Z_PRIVATE"))).
		Return(nil, errors.New("InvalidChangeBatch: Tried to delete resource record set but it was not found"))
	mockRoute53.EXPECT().ChangeRecordsWithContext(gomock.Any(), gomock.Eq(deleteInput("Z_PUBLIC"))).
		Return(&route53.ChangeResourceRecordSetsOutput{}, nil)

	mockACM.EXPECT().DeleteCertificateWithContext(gomock.Any(), gomock.Eq(&acm.DeleteCertificateInput{
		CertificateArn: awssdk.String(arn),
	})).Return(&acm.DeleteCertificateOutput{}, nil)

	err := m.DeleteWithValidationRecords(context.Background(), arn)
	assert.NoError(t, err)
}

// A DNS validation option may have no ResourceRecord yet (ACM populates it
// asynchronously); delete must skip record cleanup instead of panicking.
func TestDeleteWithValidationRecords_NilResourceRecordSkipsCleanup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockACM := services.NewMockACM(ctrl)
	mockRoute53 := services.NewMockRoute53(ctrl)
	m := &defaultCertificateManager{
		acmClient:     mockACM,
		route53Client: mockRoute53,
		logger:        logr.New(&log.NullLogSink{}),
	}

	arn := "arn:aws:acm:us-east-1:123456789012:certificate/test"
	mockACM.EXPECT().DescribeCertificateWithContext(gomock.Any(), gomock.Eq(&acm.DescribeCertificateInput{
		CertificateArn: awssdk.String(arn),
	})).Return(&acm.DescribeCertificateOutput{
		Certificate: &acmtypes.CertificateDetail{
			DomainValidationOptions: []acmtypes.DomainValidation{
				{
					ValidationMethod: acmtypes.ValidationMethodDns,
					DomainName:       awssdk.String("app.sub.example.com"),
					ResourceRecord:   nil,
				},
			},
		},
	}, nil)

	mockACM.EXPECT().DeleteCertificateWithContext(gomock.Any(), gomock.Eq(&acm.DeleteCertificateInput{
		CertificateArn: awssdk.String(arn),
	})).Return(&acm.DeleteCertificateOutput{}, nil)

	err := m.DeleteWithValidationRecords(context.Background(), arn)
	assert.NoError(t, err)
}

func Test_defaultCertificateManager_buildValidationResourceRecordSet(t *testing.T) {
	validationOpts := acmtypes.DomainValidation{
		DomainName: awssdk.String("example.com"),
		ResourceRecord: &acmtypes.ResourceRecord{
			Name:  awssdk.String("_abc123.example.com."),
			Type:  acmtypes.RecordTypeCname,
			Value: awssdk.String("_xyz456.acm-validations.aws."),
		},
	}

	tests := []struct {
		name              string
		routingPolicy     string
		clusterName       string
		weight            int64
		wantSetIdentifier *string
		wantWeight        *int64
	}{
		{
			name:              "simple routing policy does not set SetIdentifier or Weight",
			routingPolicy:     config.Route53RoutingPolicySimple,
			clusterName:       "blue-cluster",
			weight:            100,
			wantSetIdentifier: nil,
			wantWeight:        nil,
		},
		{
			name:              "weighted routing policy sets SetIdentifier from cluster name and configured Weight",
			routingPolicy:     config.Route53RoutingPolicyWeighted,
			clusterName:       "blue-cluster",
			weight:            50,
			wantSetIdentifier: awssdk.String("blue-cluster"),
			wantWeight:        awssdk.Int64(50),
		},
		{
			name:              "weighted routing policy with a different cluster name and weight",
			routingPolicy:     config.Route53RoutingPolicyWeighted,
			clusterName:       "green-cluster",
			weight:            100,
			wantSetIdentifier: awssdk.String("green-cluster"),
			wantWeight:        awssdk.Int64(100),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &defaultCertificateManager{
				clusterName:          tt.clusterName,
				route53RoutingPolicy: tt.routingPolicy,
				route53RecordWeight:  tt.weight,
			}

			got := c.buildValidationResourceRecordSet(validationOpts)

			assert.Equal(t, awssdk.ToString(validationOpts.ResourceRecord.Name), awssdk.ToString(got.Name))
			assert.Equal(t, route53types.RRType(validationOpts.ResourceRecord.Type), got.Type)
			assert.Equal(t, int64(validationRecordTTL), awssdk.ToInt64(got.TTL))
			assert.Len(t, got.ResourceRecords, 1)
			assert.Equal(t, awssdk.ToString(validationOpts.ResourceRecord.Value), awssdk.ToString(got.ResourceRecords[0].Value))

			assert.Equal(t, awssdk.ToString(tt.wantSetIdentifier), awssdk.ToString(got.SetIdentifier))
			assert.Equal(t, awssdk.ToInt64(tt.wantWeight), awssdk.ToInt64(got.Weight))
		})
	}
}

// Test_defaultCertificateManager_buildValidationResourceRecordSet_createDeleteParity guards against the
// create and delete paths drifting apart - Route53 requires a DELETE's ResourceRecordSet to be byte-for-byte
// identical (including SetIdentifier/Weight) to what was created, or the delete fails.
func Test_defaultCertificateManager_buildValidationResourceRecordSet_createDeleteParity(t *testing.T) {
	c := &defaultCertificateManager{
		clusterName:          "blue-cluster",
		route53RoutingPolicy: config.Route53RoutingPolicyWeighted,
		route53RecordWeight:  50,
	}
	validationOpts := acmtypes.DomainValidation{
		DomainName: awssdk.String("example.com"),
		ResourceRecord: &acmtypes.ResourceRecord{
			Name:  awssdk.String("_abc123.example.com."),
			Type:  acmtypes.RecordTypeCname,
			Value: awssdk.String("_xyz456.acm-validations.aws."),
		},
	}

	createRRS := c.buildValidationResourceRecordSet(validationOpts)
	deleteRRS := c.buildValidationResourceRecordSet(validationOpts)

	assert.Equal(t, createRRS, deleteRRS)
}

// Split-horizon + weighted routing:
// The weighted record must be sent to both zones. The wrong zone returns "not found".
func TestDeleteWithValidationRecords_SplitHorizonWeightedCleansBothZones(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockACM := services.NewMockACM(ctrl)
	mockRoute53 := services.NewMockRoute53(ctrl)
	m := &defaultCertificateManager{
		acmClient:            mockACM,
		route53Client:        mockRoute53,
		logger:               logr.New(&log.NullLogSink{}),
		clusterName:          "blue-cluster",
		route53RoutingPolicy: config.Route53RoutingPolicyWeighted,
		route53RecordWeight:  100,
	}

	arn := "arn:aws:acm:us-east-1:123456789012:certificate/test"
	mockACM.EXPECT().DescribeCertificateWithContext(gomock.Any(), gomock.Eq(&acm.DescribeCertificateInput{
		CertificateArn: awssdk.String(arn),
	})).Return(&acm.DescribeCertificateOutput{
		Certificate: &acmtypes.CertificateDetail{
			DomainValidationOptions: []acmtypes.DomainValidation{
				{
					ValidationMethod: acmtypes.ValidationMethodDns,
					DomainName:       awssdk.String("app.sub.example.com"),
					ResourceRecord: &acmtypes.ResourceRecord{
						Name:  awssdk.String("cname-name"),
						Value: awssdk.String("cname-value"),
						Type:  acmtypes.RecordTypeCname,
					},
				},
			},
		},
	}, nil)

	mockRoute53.EXPECT().GetHostedZoneID(gomock.Any(), gomock.Eq("app.sub.example.com")).Return(awssdk.String("Z_PRIVATE"), nil)
	mockRoute53.EXPECT().GetPublicHostedZoneID(gomock.Any(), gomock.Eq("app.sub.example.com")).Return(awssdk.String("Z_PUBLIC"), nil)

	// Both deletes must carry the weighted record shape.
	deleteInput := func(zoneID string) *route53.ChangeResourceRecordSetsInput {
		return &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: awssdk.String(zoneID),
			ChangeBatch: &route53types.ChangeBatch{
				Changes: []route53types.Change{
					{
						Action: "DELETE",
						ResourceRecordSet: &route53types.ResourceRecordSet{
							Name:          awssdk.String("cname-name"),
							Type:          route53types.RRType(acmtypes.RecordTypeCname),
							TTL:           awssdk.Int64(validationRecordTTL),
							SetIdentifier: awssdk.String("blue-cluster"),
							Weight:        awssdk.Int64(100),
							ResourceRecords: []route53types.ResourceRecord{
								{Value: awssdk.String("cname-value")},
							},
						},
					},
				},
			},
		}
	}
	// Private-zone delete returns "not found".
	mockRoute53.EXPECT().ChangeRecordsWithContext(gomock.Any(), gomock.Eq(deleteInput("Z_PRIVATE"))).
		Return(nil, errors.New("InvalidChangeBatch: Tried to delete resource record set but it was not found"))
	mockRoute53.EXPECT().ChangeRecordsWithContext(gomock.Any(), gomock.Eq(deleteInput("Z_PUBLIC"))).
		Return(&route53.ChangeResourceRecordSetsOutput{}, nil)

	mockACM.EXPECT().DeleteCertificateWithContext(gomock.Any(), gomock.Eq(&acm.DeleteCertificateInput{
		CertificateArn: awssdk.String(arn),
	})).Return(&acm.DeleteCertificateOutput{}, nil)

	err := m.DeleteWithValidationRecords(context.Background(), arn)
	assert.NoError(t, err)
}
