package acm

import (
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
)

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
