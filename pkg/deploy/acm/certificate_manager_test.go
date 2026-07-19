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
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
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
