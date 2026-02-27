package elbv2

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
)

func TestCheckGatewayTGCUniqueness(t *testing.T) {
	testCases := []struct {
		name        string
		existing    []elbv2gw.TargetGroupConfiguration
		newTGC      *elbv2gw.TargetGroupConfiguration
		expectError bool
	}{
		{
			name: "service TGC — no uniqueness check",
			newTGC: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "svc-tgc", Namespace: "ns"},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Kind: awssdk.String("Service"),
						Name: "my-svc",
					},
				},
			},
		},
		{
			name: "service TGC — multiple for same service in same namespace — no conflict",
			existing: []elbv2gw.TargetGroupConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc-tgc-1", Namespace: "ns"},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						TargetReference: elbv2gw.Reference{
							Kind: awssdk.String("Service"),
							Name: "my-svc",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc-tgc-2", Namespace: "ns"},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						TargetReference: elbv2gw.Reference{
							Kind: awssdk.String("Service"),
							Name: "my-svc",
						},
					},
				},
			},
			newTGC: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "svc-tgc-3", Namespace: "ns"},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Kind: awssdk.String("Service"),
						Name: "my-svc",
					},
				},
			},
		},
		{
			name: "gateway TGC — no conflict",
			newTGC: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-tgc", Namespace: "ns"},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Kind: awssdk.String(gatewayKind),
						Name: "my-gw",
					},
				},
			},
		},
		{
			name: "gateway TGC — conflict with existing",
			existing: []elbv2gw.TargetGroupConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "existing-tgc", Namespace: "ns"},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						TargetReference: elbv2gw.Reference{
							Kind: awssdk.String(gatewayKind),
							Name: "my-gw",
						},
					},
				},
			},
			newTGC: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "new-tgc", Namespace: "ns"},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Kind: awssdk.String(gatewayKind),
						Name: "my-gw",
					},
				},
			},
			expectError: true,
		},
		{
			name: "gateway TGC — same name (update case) — no conflict",
			existing: []elbv2gw.TargetGroupConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "gw-tgc", Namespace: "ns"},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						TargetReference: elbv2gw.Reference{
							Kind: awssdk.String(gatewayKind),
							Name: "my-gw",
						},
					},
				},
			},
			newTGC: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-tgc", Namespace: "ns"},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Kind: awssdk.String(gatewayKind),
						Name: "my-gw",
					},
				},
			},
		},
		{
			name: "gateway TGC — different gateway name — no conflict",
			existing: []elbv2gw.TargetGroupConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "existing-tgc", Namespace: "ns"},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						TargetReference: elbv2gw.Reference{
							Kind: awssdk.String(gatewayKind),
							Name: "other-gw",
						},
					},
				},
			},
			newTGC: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "new-tgc", Namespace: "ns"},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Kind: awssdk.String(gatewayKind),
						Name: "my-gw",
					},
				},
			},
		},
		{
			name: "gateway TGC — different namespace — no conflict",
			existing: []elbv2gw.TargetGroupConfiguration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "existing-tgc", Namespace: "other-ns"},
					Spec: elbv2gw.TargetGroupConfigurationSpec{
						TargetReference: elbv2gw.Reference{
							Kind: awssdk.String(gatewayKind),
							Name: "my-gw",
						},
					},
				},
			},
			newTGC: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "new-tgc", Namespace: "ns"},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Kind: awssdk.String(gatewayKind),
						Name: "my-gw",
					},
				},
			},
		},
		{
			name: "nil kind — no uniqueness check",
			newTGC: &elbv2gw.TargetGroupConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "tgc", Namespace: "ns"},
				Spec: elbv2gw.TargetGroupConfigurationSpec{
					TargetReference: elbv2gw.Reference{
						Name: "my-svc",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sClient := testutils.GenerateTestClient()

			for i := range tc.existing {
				err := k8sClient.Create(context.Background(), &tc.existing[i])
				assert.NoError(t, err)
			}

			v := &targetGroupConfigurationValidator{
				k8sClient: k8sClient,
			}

			err := v.checkGatewayTGCUniqueness(context.Background(), tc.newTGC)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
