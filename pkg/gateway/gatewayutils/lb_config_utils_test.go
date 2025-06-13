package gatewayutils

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/apis/gateway/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/testutils"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
	"time"
)

func Test_ResolveLoadBalancerConfig(t *testing.T) {
	testCases := []struct {
		name      string
		reference *gwv1.ParametersReference
		lbConf    *elbv2gw.LoadBalancerConfiguration
		expectErr bool
	}{
		{
			name: "lb conf found",
			reference: &gwv1.ParametersReference{
				Name:      "foo",
				Namespace: (*gwv1.Namespace)(awssdk.String("ns")),
			},
			lbConf: &elbv2gw.LoadBalancerConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "ns",
				},
			},
		},
		{
			name: "no lb conf",
			reference: &gwv1.ParametersReference{
				Name:      "foo",
				Namespace: (*gwv1.Namespace)(awssdk.String("ns")),
			},
			expectErr: true,
		},
		{
			name: "no namespace",
			reference: &gwv1.ParametersReference{
				Name: "foo",
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := testutils.GenerateTestClient()
			if tc.lbConf != nil {
				err := mockClient.Create(context.Background(), tc.lbConf)
				assert.NoError(t, err)
			}
			time.Sleep(1 * time.Second)

			res, err := ResolveLoadBalancerConfig(context.Background(), mockClient, tc.reference)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.lbConf, res)
		})
	}
}
