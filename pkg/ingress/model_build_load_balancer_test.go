package ingress

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	elbv2deploy "sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	networking2 "sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
	"sigs.k8s.io/controller-runtime/pkg/log"
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

var (
	subnet1 = &ec2.Subnet{
		SubnetId:         awssdk.String("subnet-1"),
		AvailabilityZone: awssdk.String("az1"),
		VpcId:            awssdk.String("vpc-1"),
	}
	subnet2 = &ec2.Subnet{
		SubnetId:         awssdk.String("subnet-2"),
		AvailabilityZone: awssdk.String("az2"),
		Tags: []*ec2.Tag{
			{
				Key:   awssdk.String("Name"),
				Value: awssdk.String("namedsubnet"),
			},
		},
		VpcId: awssdk.String("vpc-1"),
	}
	subnet3 = &ec2.Subnet{
		SubnetId:         awssdk.String("subnet-3"),
		AvailabilityZone: awssdk.String("az3"),
		VpcId:            awssdk.String("vpc-1"),
	}
	subnet4 = &ec2.Subnet{
		SubnetId:         awssdk.String("subnet-4"),
		AvailabilityZone: awssdk.String("az4"),
		Tags: []*ec2.Tag{
			{
				Key:   awssdk.String("Name"),
				Value: awssdk.String("othername"),
			},
		},
		VpcId: awssdk.String("vpc-1"),
	}
	subnet5 = &ec2.Subnet{
		SubnetId:         awssdk.String("subnet-5"),
		AvailabilityZone: awssdk.String("az5"),
		Tags: []*ec2.Tag{
			{
				Key:   awssdk.String("Name"),
				Value: awssdk.String("namedsubnet"),
			},
		},
		VpcId: awssdk.String("vpc-2"),
	}
	subnet6 = &ec2.Subnet{
		SubnetId:         awssdk.String("subnet-6"),
		AvailabilityZone: awssdk.String("az6"),
		Tags: []*ec2.Tag{
			{
				Key:   awssdk.String("Name"),
				Value: awssdk.String("subnet-1"),
			},
		},
		VpcId: awssdk.String("vpc-1"),
	}
	subnet7 = &ec2.Subnet{
		SubnetId:         awssdk.String("subnet-7"),
		AvailabilityZone: awssdk.String("az7"),
		Tags: []*ec2.Tag{
			{
				Key:   awssdk.String("Name"),
				Value: awssdk.String("subnet-1"),
			},
			{
				Key:   awssdk.String("class"),
				Value: awssdk.String("test"),
			}},
		VpcId: awssdk.String("vpc-1"),
	}
	subnet8 = &ec2.Subnet{
		SubnetId:         awssdk.String("subnet-8"),
		AvailabilityZone: awssdk.String("az8"),
		Tags: []*ec2.Tag{
			{
				Key:   awssdk.String("alb"),
				Value: awssdk.String("test"),
			},
		},
		VpcId: awssdk.String("vpc-1"),
	}
	subnet9 = &ec2.Subnet{
		SubnetId:         awssdk.String("subnet-9"),
		AvailabilityZone: awssdk.String("az9"),
		Tags: []*ec2.Tag{
			{
				Key:   awssdk.String("Name"),
				Value: awssdk.String("subnet-12"),
			},
			{
				Key:   awssdk.String("alb"),
				Value: awssdk.String("test"),
			},
			{
				Key:   awssdk.String("class"),
				Value: awssdk.String("test"),
			}},
		VpcId: awssdk.String("vpc-1"),
	}
	subnet10 = &ec2.Subnet{
		SubnetId:         awssdk.String("subnet-10"),
		AvailabilityZone: awssdk.String("az10"),
		Tags: []*ec2.Tag{
			{
				Key:   awssdk.String("alb"),
				Value: awssdk.String("testing"),
			},
			{
				Key:   awssdk.String("class"),
				Value: awssdk.String("test"),
			},
		},
		VpcId: awssdk.String("vpc-1"),
	}
	subnet11 = &ec2.Subnet{
		SubnetId:         awssdk.String("subnet-11"),
		AvailabilityZone: awssdk.String("az11"),
		Tags: []*ec2.Tag{
			{
				Key:   awssdk.String("tagtest"),
				Value: awssdk.String("1"),
			},
			{
				Key:   awssdk.String("tagtest2"),
				Value: awssdk.String("1"),
			},
		},
		VpcId: awssdk.String("vpc-1"),
	}
	subnet12 = &ec2.Subnet{
		SubnetId:         awssdk.String("subnet-12"),
		AvailabilityZone: awssdk.String("az12"),
		Tags: []*ec2.Tag{
			{
				Key:   awssdk.String("tagtest"),
				Value: awssdk.String("1"),
			},
		},
		VpcId: awssdk.String("vpc-1"),
	}
	subnet13 = &ec2.Subnet{
		SubnetId:         awssdk.String("subnet-13"),
		AvailabilityZone: awssdk.String("az13"),
		Tags: []*ec2.Tag{
			{
				Key:   awssdk.String("tagtest"),
				Value: awssdk.String("1"),
			},
			{
				Key:   awssdk.String("kubernetes.io/cluster/other-cluster"),
				Value: awssdk.String("shared"),
			},
		},
		VpcId: awssdk.String("vpc-1"),
	}
	subnet14 = &ec2.Subnet{
		SubnetId:         awssdk.String("subnet-14"),
		AvailabilityZone: awssdk.String("az14"),
		Tags: []*ec2.Tag{
			{
				Key:   awssdk.String("tagtest2"),
				Value: awssdk.String("1"),
			},
		},
		VpcId: awssdk.String("vpc-1"),
	}
	subnet15 = &ec2.Subnet{
		SubnetId:         awssdk.String("subnet-15"),
		AvailabilityZone: awssdk.String("az14"),
		Tags: []*ec2.Tag{
			{
				Key:   awssdk.String("tagtest2"),
				Value: awssdk.String("1"),
			},
			{
				Key:   awssdk.String("kubernetes.io/cluster/test-cluster"),
				Value: awssdk.String("shared"),
			},
		},
		VpcId: awssdk.String("vpc-1"),
	}
)

func stubDescribeSubnetsAsList(ctx context.Context, input *ec2.DescribeSubnetsInput) ([]*ec2.Subnet, error) {
	subnets := []*ec2.Subnet{
		subnet1,
		subnet2,
		subnet3,
		subnet4,
		subnet5,
		subnet6,
		subnet7,
		subnet8,
		subnet9,
		subnet10,
		subnet11,
		subnet12,
		subnet13,
		subnet14,
		subnet15,
	}
	if input.SubnetIds != nil {
		var filtered []*ec2.Subnet
		for _, subnet := range subnets {
			for _, id := range input.SubnetIds {
				if awssdk.StringValue(subnet.SubnetId) == awssdk.StringValue(id) {
					filtered = append(filtered, subnet)
					continue
				}
			}
		}
		subnets = filtered
	}
	if input.Filters != nil {
		var filtered []*ec2.Subnet
	subnetLoop:
		for _, subnet := range subnets {
			for _, filter := range input.Filters {
				eligible := false
				if awssdk.StringValue(filter.Name) == "vpc-id" {
					for _, name := range filter.Values {
						if awssdk.StringValue(subnet.VpcId) == awssdk.StringValue(name) {
							eligible = true
							continue
						}
					}
				} else if strings.HasPrefix(awssdk.StringValue(filter.Name), "tag:") {
					key := strings.TrimPrefix(awssdk.StringValue(filter.Name), "tag:")
					for _, value := range filter.Values {
						for _, tag := range subnet.Tags {
							if awssdk.StringValue(tag.Key) == key && awssdk.StringValue(tag.Value) == awssdk.StringValue(value) {
								eligible = true
								continue
							}
						}
					}
				} else {
					return nil, fmt.Errorf("unexpected filter %q", awssdk.StringValue(filter.Name))
				}
				if !eligible {
					continue subnetLoop
				}
			}
			filtered = append(filtered, subnet)
		}
		subnets = filtered
	}
	return subnets, nil
}

func Test_defaultModelBuildTask_buildLoadBalancerSubnets(t *testing.T) {
	type fields struct {
		ingGroup Group
		scheme   elbv2.LoadBalancerScheme
	}
	tests := []struct {
		name    string
		fields  fields
		want    []string
		wantErr string
	}{
		{
			name: "no annotation implicit subnet",
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
			wantErr: "called ListLoadBalancers()",
		},
		{
			name: "subnet annotation",
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
										"alb.ingress.kubernetes.io/subnets": "subnet-1,namedsubnet",
									},
								},
							},
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{},
								},
							},
						},
					},
				},
			},
			want: []string{"subnet-1", "subnet-2"},
		},
		{
			name: "classparams subnet ids",
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
										"alb.ingress.kubernetes.io/subnets": "subnet-4,othername",
									},
								},
							},
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										Subnets: &v1beta1.SubnetSelector{
											IDs: []v1beta1.SubnetID{"subnet-1", "subnet-2"},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []string{"subnet-1", "subnet-2"},
		},
		{
			name: "classparams subnet tag multiple values",
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
										"alb.ingress.kubernetes.io/subnets": "subnet-4,othername",
									},
								},
							},
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										Subnets: &v1beta1.SubnetSelector{
											Tags: map[string][]string{
												"Name": {"namedsubnet", "othername"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []string{"subnet-2", "subnet-4"},
		},
		{
			name: "classparams subnet tag multiple matches",
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
										"alb.ingress.kubernetes.io/subnets": "subnet-4,othername",
									},
								},
							},
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										Subnets: &v1beta1.SubnetSelector{
											Tags: map[string][]string{
												"Name": {"subnet-1"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []string{"subnet-6", "subnet-7"},
		},
		{
			name: "classparams custom tag",
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
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										Subnets: &v1beta1.SubnetSelector{
											Tags: map[string][]string{
												"alb": {"test"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []string{"subnet-8", "subnet-9"},
		},
		{
			name: "classparams multiple tags",
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
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										Subnets: &v1beta1.SubnetSelector{
											Tags: map[string][]string{
												"alb":   {"test", "testing"},
												"class": {"test"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []string{"subnet-10", "subnet-9"},
		},
		{
			name: "classparams missing id",
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
										"alb.ingress.kubernetes.io/subnets": "subnet-4,othername",
									},
								},
							},
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										Subnets: &v1beta1.SubnetSelector{
											IDs: []v1beta1.SubnetID{"subnet-1234", "subnet-1"},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: "couldn't find all subnets, IDs: [subnet-1234 subnet-1], found: 1",
		},
		{
			name: "classparams ignore tagged other cluster",
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
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										Subnets: &v1beta1.SubnetSelector{
											Tags: map[string][]string{
												"tagtest": {"1"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []string{"subnet-11", "subnet-12"},
		},
		{
			name: "classparams prefer tagged for cluster",
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
							IngClassConfig: ClassConfiguration{
								IngClassParams: &v1beta1.IngressClassParams{
									Spec: v1beta1.IngressClassParamsSpec{
										Subnets: &v1beta1.SubnetSelector{
											Tags: map[string][]string{
												"tagtest2": {"1"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []string{"subnet-11", "subnet-15"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			taggingManager := elbv2deploy.NewMockTaggingManager(ctrl)
			taggingManager.EXPECT().ListLoadBalancers(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, tagFilters ...tracking.TagFilter) ([]elbv2deploy.LoadBalancerWithTags, error) {
					return nil, fmt.Errorf("called ListLoadBalancers()")
				}).AnyTimes()

			mockEC2 := services.NewMockEC2(ctrl)
			mockEC2.EXPECT().DescribeSubnetsAsList(gomock.Any(), gomock.Any()).
				DoAndReturn(stubDescribeSubnetsAsList).
				AnyTimes()

			azInfoProvider := networking2.NewMockAZInfoProvider(ctrl)
			azInfoProvider.EXPECT().FetchAZInfos(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, availabilityZoneIDs []string) (map[string]ec2.AvailabilityZone, error) {
					ret := make(map[string]ec2.AvailabilityZone, len(availabilityZoneIDs))
					for _, id := range availabilityZoneIDs {
						ret[id] = ec2.AvailabilityZone{ZoneType: awssdk.String("availability-zone")}
					}
					return ret, nil
				}).AnyTimes()

			subnetsResolver := networking2.NewDefaultSubnetsResolver(
				azInfoProvider,
				mockEC2,
				"vpc-1",
				"test-cluster",
				logr.New(&log.NullLogSink{}),
			)

			task := &defaultModelBuildTask{
				featureGates:        config.NewFeatureGates(),
				ingGroup:            tt.fields.ingGroup,
				stack:               core.NewDefaultStack(core.StackID(tt.fields.ingGroup.ID)),
				annotationParser:    annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
				elbv2TaggingManager: taggingManager,
				subnetsResolver:     subnetsResolver,
				trackingProvider:    tracking.NewDefaultProvider("ingress.k8s.aws", "test-cluster"),
			}
			got, err := task.buildLoadBalancerSubnetMappings(context.Background(), elbv2.LoadBalancerSchemeInternetFacing)
			if err != nil {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				var gotSubnets []string
				for _, mapping := range got {
					gotSubnets = append(gotSubnets, mapping.SubnetID)
				}
				assert.Equal(t, tt.want, gotSubnets)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildLoadBalancerIPAddressType(t *testing.T) {
	type fields struct {
		ingGroup Group
	}

	tests := []struct {
		name    string
		fields  fields
		want    elbv2.IPAddressType
		wantErr error
	}{
		{
			name: "No ip-address-type annotation set",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "explicit-group"},
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
			},
			want: "",
		},
		{
			name: "The ip-address-type annotation is set to ipv4",
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
										"alb.ingress.kubernetes.io/ip-address-type": "ipv4",
									},
								},
							},
						},
					},
				},
			},
			want: elbv2.IPAddressTypeIPV4,
		},
		{
			name: "The ip-address-type annotation is set to dualstack",
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
										"alb.ingress.kubernetes.io/ip-address-type": "dualstack",
									},
								},
							},
						},
					},
				},
			},
			want: elbv2.IPAddressTypeDualStack,
		},
		{
			name: "The ip-address-type annotation is set to dualstack-without-public-ipv4",
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
										"alb.ingress.kubernetes.io/ip-address-type": "dualstack-without-public-ipv4",
										"alb.ingress.kubernetes.io/scheme":          "internet-facing",
									},
								},
							},
						},
					},
				},
			},
			want: elbv2.IPAddressTypeDualStackWithoutPublicIPV4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				ingGroup:         tt.fields.ingGroup,
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
			}
			got, err := task.buildLoadBalancerIPAddressType(context.Background())
			if err != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
