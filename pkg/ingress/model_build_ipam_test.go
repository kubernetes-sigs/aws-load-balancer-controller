package ingress

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"strings"
	"testing"
)

func Test_buildIPv4IPAMPoolID(t *testing.T) {
	testCases := []struct {
		name           string
		ingGroup       Group
		expectedPoolId *string
		errSubString   string
	}{
		{
			name: "no ingresses configured",
		},
		{
			name: "one ingress configured",
			ingGroup: Group{
				Members: []ClassifiedIngress{
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "foo",
								},
							},
						},
					},
				},
			},
			expectedPoolId: awssdk.String("foo"),
		},
		{
			name: "multiple ingress configured",
			ingGroup: Group{
				Members: []ClassifiedIngress{
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "foo",
								},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing2",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "foo",
								},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "awesome-ns3",
								Name:        "awesome-ing",
								Annotations: map[string]string{},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing4",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "foo",
								},
							},
						},
					},
				},
			},
			expectedPoolId: awssdk.String("foo"),
		},
		{
			name: "multiple ingress configured with different pool ids",
			ingGroup: Group{
				Members: []ClassifiedIngress{
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "foo",
								},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing2",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "bar",
								},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "awesome-ns3",
								Name:        "awesome-ing",
								Annotations: map[string]string{},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing4",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "baz",
								},
							},
						},
					},
				},
			},
			errSubString: "conflicting ipv4 ipam pools",
		},
		{
			name: "no ing class params",
			ingGroup: Group{
				Members: []ClassifiedIngress{
					{
						IngClassConfig: ClassConfiguration{
							IngClass:       nil,
							IngClassParams: nil,
						},
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "foo",
								},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing2",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "bar",
								},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "awesome-ns3",
								Name:        "awesome-ing",
								Annotations: map[string]string{},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing4",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "baz",
								},
							},
						},
					},
				},
			},
			errSubString: "conflicting ipv4 ipam pools",
		},
		{
			name: "no ipam configuration",
			ingGroup: Group{
				Members: []ClassifiedIngress{
					{
						IngClassConfig: ClassConfiguration{
							IngClassParams: &elbv2api.IngressClassParams{
								Spec: elbv2api.IngressClassParamsSpec{},
							},
						},
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "foo",
								},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing2",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "bar",
								},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "awesome-ns3",
								Name:        "awesome-ing",
								Annotations: map[string]string{},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing4",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "baz",
								},
							},
						},
					},
				},
			},
			errSubString: "conflicting ipv4 ipam pools",
		},
		{
			name: "no pool configuration",
			ingGroup: Group{
				Members: []ClassifiedIngress{
					{
						IngClassConfig: ClassConfiguration{
							IngClassParams: &elbv2api.IngressClassParams{
								Spec: elbv2api.IngressClassParamsSpec{
									IPAMConfiguration: &elbv2api.IPAMConfiguration{},
								},
							},
						},
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "foo",
								},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing2",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "bar",
								},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "awesome-ns3",
								Name:        "awesome-ing",
								Annotations: map[string]string{},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing4",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "baz",
								},
							},
						},
					},
				},
			},
			errSubString: "conflicting ipv4 ipam pools",
		},
		{
			name: "ingress class parameter preferred",
			ingGroup: Group{
				Members: []ClassifiedIngress{
					{
						IngClassConfig: ClassConfiguration{
							IngClassParams: &elbv2api.IngressClassParams{
								Spec: elbv2api.IngressClassParamsSpec{
									IPAMConfiguration: &elbv2api.IPAMConfiguration{
										IPv4IPAMPoolId: awssdk.String("ing-class-value"),
									},
								},
							},
						},
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "foo",
								},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing2",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "bar",
								},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "awesome-ns3",
								Name:        "awesome-ing",
								Annotations: map[string]string{},
							},
						},
					},
					{
						Ing: &v1.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "awesome-ing4",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/ipam-ipv4-pool-id": "baz",
								},
							},
						},
					},
				},
			},
			expectedPoolId: awssdk.String("ing-class-value"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			buildTask := &defaultModelBuildTask{
				ingGroup:         tc.ingGroup,
				annotationParser: annotationParser,
			}

			resolvedPoolId, err := buildTask.buildIPv4IPAMPoolID()
			if len(tc.errSubString) > 0 {
				assert.True(t, strings.Contains(err.Error(), tc.errSubString))
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expectedPoolId, resolvedPoolId)
			}
		})
	}
}
