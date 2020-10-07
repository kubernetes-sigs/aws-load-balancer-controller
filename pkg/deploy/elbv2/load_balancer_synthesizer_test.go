package elbv2

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	elbv2sdk "github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	coremodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

func Test_matchResAndSDKLoadBalancers(t *testing.T) {
	stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})
	type args struct {
		resLBs           []*elbv2model.LoadBalancer
		sdkLBs           []LoadBalancerWithTags
		resourceIDTagKey string
	}
	tests := []struct {
		name    string
		args    args
		want    []resAndSDKLoadBalancerPair
		want1   []*elbv2model.LoadBalancer
		want2   []LoadBalancerWithTags
		wantErr error
	}{
		{
			name: "all loadBalancer has match",
			args: args{
				resLBs: []*elbv2model.LoadBalancer{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-1",
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-2"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-2",
						},
					},
				},
				sdkLBs: []LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: []resAndSDKLoadBalancerPair{
				{
					resLB: &elbv2model.LoadBalancer{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-1",
						},
					},
					sdkLB: LoadBalancerWithTags{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				{
					resLB: &elbv2model.LoadBalancer{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-2"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-2",
						},
					},
					sdkLB: LoadBalancerWithTags{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
			},
		},
		{
			name: "some res LoadBalancer don't have match",
			args: args{
				resLBs: []*elbv2model.LoadBalancer{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-1",
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-2"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-2",
						},
					},
				},
				sdkLBs: []LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: []resAndSDKLoadBalancerPair{
				{
					resLB: &elbv2model.LoadBalancer{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-1",
						},
					},
					sdkLB: LoadBalancerWithTags{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
			},
			want1: []*elbv2model.LoadBalancer{
				{
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-2"),
					Spec: elbv2model.LoadBalancerSpec{
						Name: "id-2",
					},
				},
			},
		},
		{
			name: "some sdk LoadBalancer don't have match",
			args: args{
				resLBs: []*elbv2model.LoadBalancer{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-1",
						},
					},
				},
				sdkLBs: []LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: []resAndSDKLoadBalancerPair{
				{
					resLB: &elbv2model.LoadBalancer{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-1",
						},
					},
					sdkLB: LoadBalancerWithTags{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
			},
			want2: []LoadBalancerWithTags{
				{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						LoadBalancerArn: awssdk.String("arn-2"),
					},
					Tags: map[string]string{
						"ingress.k8s.aws/resource": "id-2",
					},
				},
			},
		},
		{
			name: "one loadBalancer need to be replaced",
			args: args{
				resLBs: []*elbv2model.LoadBalancer{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "my-name",
							Type: elbv2model.LoadBalancerTypeNetwork,
						},
					},
				},
				sdkLBs: []LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
							Type:            awssdk.String("application"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2"),
							Type:            awssdk.String("network"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: []resAndSDKLoadBalancerPair{
				{
					resLB: &elbv2model.LoadBalancer{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "my-name",
							Type: elbv2model.LoadBalancerTypeNetwork,
						},
					},
					sdkLB: LoadBalancerWithTags{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2"),
							Type:            awssdk.String("network"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
			},
			want2: []LoadBalancerWithTags{
				{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						LoadBalancerArn: awssdk.String("arn-1"),
						Type:            awssdk.String("application"),
					},
					Tags: map[string]string{
						"ingress.k8s.aws/resource": "id-1",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, got2, err := matchResAndSDKLoadBalancers(tt.args.resLBs, tt.args.sdkLBs, tt.args.resourceIDTagKey)
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

func Test_mapResLoadBalancerByResourceID(t *testing.T) {
	stack := coremodel.NewDefaultStack(coremodel.StackID{Namespace: "namespace", Name: "name"})
	type args struct {
		resLBs []*elbv2model.LoadBalancer
	}
	tests := []struct {
		name string
		args args
		want map[string]*elbv2model.LoadBalancer
	}{
		{
			name: "standard case",
			args: args{
				resLBs: []*elbv2model.LoadBalancer{
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-1",
						},
					},
					{
						ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-2"),
						Spec: elbv2model.LoadBalancerSpec{
							Name: "id-2",
						},
					},
				},
			},
			want: map[string]*elbv2model.LoadBalancer{
				"id-1": {
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-1"),
					Spec: elbv2model.LoadBalancerSpec{
						Name: "id-1",
					},
				},
				"id-2": {
					ResourceMeta: coremodel.NewResourceMeta(stack, "AWS::ElasticLoadBalancingV2::LoadBalancer", "id-2"),
					Spec: elbv2model.LoadBalancerSpec{
						Name: "id-2",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapResLoadBalancerByResourceID(tt.args.resLBs)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_mapSDKLoadBalancerByResourceID(t *testing.T) {
	type args struct {
		sdkLBs           []LoadBalancerWithTags
		resourceIDTagKey string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string][]LoadBalancerWithTags
		wantErr error
	}{
		{
			name: "standard case",
			args: args{
				sdkLBs: []LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: map[string][]LoadBalancerWithTags{
				"id-1": {
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				"id-2": {
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
			},
		},
		{
			name: "multiple LoadBalancers with same ID",
			args: args{
				sdkLBs: []LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2A"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2B"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			want: map[string][]LoadBalancerWithTags{
				"id-1": {
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-1",
						},
					},
				},
				"id-2": {
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2A"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-2B"),
						},
						Tags: map[string]string{
							"ingress.k8s.aws/resource": "id-2",
						},
					},
				},
			},
		},
		{
			name: "LoadBalancers without resourceID tag",
			args: args{
				sdkLBs: []LoadBalancerWithTags{
					{
						LoadBalancer: &elbv2sdk.LoadBalancer{
							LoadBalancerArn: awssdk.String("arn-1"),
						},
						Tags: map[string]string{},
					},
				},
				resourceIDTagKey: "ingress.k8s.aws/resource",
			},
			wantErr: errors.New("unexpected loadBalancer with no resourceID: arn-1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapSDKLoadBalancerByResourceID(tt.args.sdkLBs, tt.args.resourceIDTagKey)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_isSDKLoadBalancerRequiresReplacement(t *testing.T) {
	schemaInternetFacing := elbv2model.LoadBalancerSchemeInternetFacing
	type args struct {
		sdkLB LoadBalancerWithTags
		resLB *elbv2model.LoadBalancer
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "don't need replacement",
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						Type:             awssdk.String("application"),
						Scheme:           awssdk.String("internet-facing"),
						LoadBalancerName: awssdk.String("my-lb"),
					},
				},
				resLB: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						Type:   elbv2model.LoadBalancerTypeApplication,
						Scheme: &schemaInternetFacing,
						Name:   "my-lb",
					},
				},
			},
			want: false,
		},
		{
			name: "name-only change shouldn't need replacement",
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						Type:             awssdk.String("application"),
						Scheme:           awssdk.String("internet-facing"),
						LoadBalancerName: awssdk.String("my-lb1"),
					},
				},
				resLB: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						Type:   elbv2model.LoadBalancerTypeApplication,
						Scheme: &schemaInternetFacing,
						Name:   "my-lb",
					},
				},
			},
			want: false,
		},
		{
			name: "type change need replacement",
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						Type:             awssdk.String("network"),
						Scheme:           awssdk.String("internet-facing"),
						LoadBalancerName: awssdk.String("my-lb"),
					},
				},
				resLB: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						Type:   elbv2model.LoadBalancerTypeApplication,
						Scheme: &schemaInternetFacing,
						Name:   "my-lb",
					},
				},
			},
			want: true,
		},
		{
			name: "scheme need replacement",
			args: args{
				sdkLB: LoadBalancerWithTags{
					LoadBalancer: &elbv2sdk.LoadBalancer{
						Type:             awssdk.String("application"),
						Scheme:           awssdk.String("internal"),
						LoadBalancerName: awssdk.String("my-lb"),
					},
				},
				resLB: &elbv2model.LoadBalancer{
					Spec: elbv2model.LoadBalancerSpec{
						Type:   elbv2model.LoadBalancerTypeApplication,
						Scheme: &schemaInternetFacing,
						Name:   "my-lb",
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSDKLoadBalancerRequiresReplacement(tt.args.sdkLB, tt.args.resLB)
			assert.Equal(t, tt.want, got)
		})
	}
}
