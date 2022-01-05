package ingress

import (
	"context"
	"errors"
	"testing"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
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
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "ing-1",
									Annotations: map[string]string{},
								},
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
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
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
			},
			want: awssdk.String("my-ip-pool"),
		},
		{
			name: "specified empty COIPv4 on standalone Ingress",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
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
			},
			wantErr: errors.New("cannot use empty value for customer-owned-ipv4-pool annotation, ingress: awesome-ns/ing-1"),
		},
		{
			name: "COIPv4 not configured on all Ingresses among IngressGroup",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "ing-1",
									Annotations: map[string]string{},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "ing-2",
									Annotations: map[string]string{},
								},
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
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/customer-owned-ipv4-pool": "my-ip-pool",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace:   "awesome-ns",
									Name:        "ing-2",
									Annotations: map[string]string{},
								},
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
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/customer-owned-ipv4-pool": "my-ip-pool",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
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
			},
			want: awssdk.String("my-ip-pool"),
		},
		{
			name: "COIPv4 configured on multiple Ingress among IngressGroup - with different value",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/customer-owned-ipv4-pool": "my-ip-pool",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
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

func Test_defaultModelBuildTask_buildLoadBalancerTags(t *testing.T) {
	type fields struct {
		ingGroup            Group
		defaultTags         map[string]string
		externalManagedTags sets.String
	}
	tests := []struct {
		name    string
		fields  fields
		want    map[string]string
		wantErr error
	}{
		{
			name: "empty default tags, no tags annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{},
								},
							},
						},
					},
				},
				defaultTags: nil,
			},
			want: map[string]string{},
		},
		{
			name: "empty default tags, non-empty tags annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k1=v1",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k2=v2",
									},
								},
							},
						},
					},
				},
				defaultTags: nil,
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
		},
		{
			name: "non-empty default tags, empty tags annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{},
								},
							},
						},
					},
				},
				defaultTags: map[string]string{
					"k3": "v3",
					"k4": "v4",
				},
			},
			want: map[string]string{
				"k3": "v3",
				"k4": "v4",
			},
		},
		{
			name: "non-empty default tags, non-empty tags annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k1=v1,k3=v3a",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k2=v2",
									},
								},
							},
						},
					},
				},
				defaultTags: map[string]string{
					"k3": "v3",
					"k4": "v4",
				},
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
				"k3": "v3",
				"k4": "v4",
			},
		},
		{
			name: "empty default tags, conflicting tags annotation",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k1=v1,k3=v3a",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k2=v2,k3=v3b",
									},
								},
							},
						},
					},
				},
				defaultTags: nil,
			},
			wantErr: errors.New("conflicting tag k3: v3a | v3b"),
		},
		{
			name: "non empty external managed tags, no conflicts",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k1=v1",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-2",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k2=v2",
									},
								},
							},
						},
					},
				},
				externalManagedTags: sets.NewString("k3"),
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
		},
		{
			name: "non empty external managed tags, has conflicts",
			fields: fields{
				ingGroup: Group{
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k1=v1",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-2",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/tags": "k2=v2",
									},
								},
							},
						},
					},
				},
				externalManagedTags: sets.NewString("k2"),
			},
			wantErr: errors.New("failed build tags for Ingress awesome-ns/ing-2: external managed tag key k2 cannot be specified"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				ingGroup:            tt.fields.ingGroup,
				defaultTags:         tt.fields.defaultTags,
				externalManagedTags: tt.fields.externalManagedTags,
				annotationParser:    annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
			}
			got, err := task.buildLoadBalancerTags(context.Background())
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildLoadBalancerName(t *testing.T) {
	type fields struct {
		ingGroup Group
		scheme   elbv2.LoadBalancerScheme
	}
	tests := []struct {
		name    string
		fields  fields
		want    string
		wantErr error
	}{
		{
			name: "no annotation implicit group",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Namespace: "awesome-ns", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
								},
							},
						},
					},
				},
				scheme: elbv2.LoadBalancerSchemeInternetFacing,
			},
			want: "k8s-awesomen-ing1-43b698093c",
		},
		{
			name: "no annotation explicit group",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "explicit-group"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/group.name": "explicit-group",
									},
								},
							},
						},
					},
				},
				scheme: elbv2.LoadBalancerSchemeInternal,
			},
			want: "k8s-explicitgroup-5bf9e53c23",
		},
		{
			name: "name annotation",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Namespace: "awesome-ns", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/load-balancer-name": "foo",
									},
								},
							},
						},
					},
				},
				scheme: elbv2.LoadBalancerSchemeInternetFacing,
			},
			want: "foo",
		},
		{
			name: "trim name annotation",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Namespace: "awesome-ns", Name: "ing-1"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/load-balancer-name": "bazbazfoofoobazbazfoofoobazbazfoo",
									},
								},
							},
						},
					},
				},
				scheme: elbv2.LoadBalancerSchemeInternetFacing,
			},
			wantErr: errors.New("load balancer name cannot be longer than 32 characters"),
		},
		{
			name: "name annotation on single ingress only",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "bar"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/load-balancer-name": "foo",
										"alb.ingress.kubernetes.io/group.name":         "bar",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/group.name": "bar",
									},
								},
							},
						},
					},
				},
				scheme: elbv2.LoadBalancerSchemeInternetFacing,
			},
			want: "foo",
		},
		{
			name: "conflicting name annotation",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "bar"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/load-balancer-name": "foo",
										"alb.ingress.kubernetes.io/group.name":         "bar",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/load-balancer-name": "baz",
										"alb.ingress.kubernetes.io/group.name":         "bar",
									},
								},
							},
						},
					},
				},
				scheme: elbv2.LoadBalancerSchemeInternetFacing,
			},
			wantErr: errors.New("conflicting load balancer name: map[baz:{} foo:{}]"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				ingGroup:         tt.fields.ingGroup,
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
			}
			got, err := task.buildLoadBalancerName(context.Background(), tt.fields.scheme)
			if err != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
