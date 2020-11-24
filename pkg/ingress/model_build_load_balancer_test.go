package ingress

import (
	"context"
	"errors"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"testing"
)

func Test_defaultModelBuildTask_buildLoadBalancerCOIPv4Pool(t *testing.T) {
	type fields struct {
		ingGroup Group
	}
	tests := []struct {
		name    string
		fields  fields
		want    *string
		wantErr error
	}{
		{
			name: "COIPv4 not configured on standalone Ingress",
			fields: fields{
				ingGroup: Group{
					Members: []*networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "awesome-ns",
								Name:        "ing-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "COIPv4 configured on standalone Ingress",
			fields: fields{
				ingGroup: Group{
					Members: []*networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/customer-owned-ipv4-pool": "my-ip-pool",
								},
							},
						},
					},
				},
			},
			want: awssdk.String("my-ip-pool"),
		},
		{
			name: "specified empty COIPv4 on standalone Ingress",
			fields: fields{
				ingGroup: Group{
					Members: []*networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/customer-owned-ipv4-pool": "",
								},
							},
						},
					},
				},
			},
			wantErr: errors.New("cannot use empty value for customer-owned-ipv4-pool annotation, ingress: awesome-ns/ing-1"),
		},
		{
			name: "COIPv4 not configured on all Ingresses among IngressGroup",
			fields: fields{
				ingGroup: Group{
					Members: []*networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "awesome-ns",
								Name:        "ing-1",
								Annotations: map[string]string{},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "awesome-ns",
								Name:        "ing-2",
								Annotations: map[string]string{},
							},
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "COIPv4 configured on one Ingress among IngressGroup",
			fields: fields{
				ingGroup: Group{
					Members: []*networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/customer-owned-ipv4-pool": "my-ip-pool",
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace:   "awesome-ns",
								Name:        "ing-2",
								Annotations: map[string]string{},
							},
						},
					},
				},
			},
			want: awssdk.String("my-ip-pool"),
		},
		{
			name: "COIPv4 configured on multiple Ingresses among IngressGroup - with same value",
			fields: fields{
				ingGroup: Group{
					Members: []*networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/customer-owned-ipv4-pool": "my-ip-pool",
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-2",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/customer-owned-ipv4-pool": "my-ip-pool",
								},
							},
						},
					},
				},
			},
			want: awssdk.String("my-ip-pool"),
		},
		{
			name: "COIPv4 configured on multiple Ingress among IngressGroup - with different value",
			fields: fields{
				ingGroup: Group{
					Members: []*networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-1",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/customer-owned-ipv4-pool": "my-ip-pool",
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "awesome-ns",
								Name:      "ing-2",
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/customer-owned-ipv4-pool": "my-another-pool",
								},
							},
						},
					},
				},
			},
			wantErr: errors.New("conflicting CustomerOwnedIPv4Pool: [my-another-pool my-ip-pool]"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			task := &defaultModelBuildTask{
				annotationParser: annotationParser,
				ingGroup:         tt.fields.ingGroup,
			}
			got, err := task.buildLoadBalancerCOIPv4Pool(context.Background())
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}
