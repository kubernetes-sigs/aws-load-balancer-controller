package networking

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	ec2sdk "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_defaultNodeInfoProvider_FetchNodeInstances(t *testing.T) {
	type describeInstancesAsListCall struct {
		req  *ec2sdk.DescribeInstancesInput
		resp []*ec2sdk.Instance
		err  error
	}
	type fields struct {
		describeInstancesAsListCalls []describeInstancesAsListCall
	}
	type args struct {
		nodes []*corev1.Node
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    map[types.NamespacedName]*ec2sdk.Instance
		wantErr error
	}{
		{
			name: "successfully fetched instance for each node",
			fields: fields{
				describeInstancesAsListCalls: []describeInstancesAsListCall{
					{
						req: &ec2sdk.DescribeInstancesInput{
							InstanceIds: awssdk.StringSlice([]string{"i-0fa2d0064e848c69e", "i-0fa2d0064e848c69f", "i-0fa2d0064e848c69g"}),
						},
						resp: []*ec2sdk.Instance{
							{
								InstanceId: awssdk.String("i-0fa2d0064e848c69e"),
							},
							{
								InstanceId: awssdk.String("i-0fa2d0064e848c69f"),
							},
							{
								InstanceId: awssdk.String("i-0fa2d0064e848c69g"),
							},
						},
					},
				},
			},
			args: args{
				nodes: []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69e",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-2",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69f",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-3",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69g",
						},
					},
				},
			},
			want: map[types.NamespacedName]*ec2sdk.Instance{
				types.NamespacedName{Name: "node-1"}: {
					InstanceId: awssdk.String("i-0fa2d0064e848c69e"),
				},
				types.NamespacedName{Name: "node-2"}: {
					InstanceId: awssdk.String("i-0fa2d0064e848c69f"),
				},
				types.NamespacedName{Name: "node-3"}: {
					InstanceId: awssdk.String("i-0fa2d0064e848c69g"),
				},
			},
		},
		{
			name: "successfully fetched instance for each node",
			fields: fields{
				describeInstancesAsListCalls: []describeInstancesAsListCall{
					{
						req: &ec2sdk.DescribeInstancesInput{
							InstanceIds: awssdk.StringSlice([]string{"i-0fa2d0064e848c69e", "i-0fa2d0064e848c69f", "i-0fa2d0064e848c69g"}),
						},
						err: errors.New("some AWS API error"),
					},
				},
			},
			args: args{
				nodes: []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69e",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-2",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69f",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-3",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69g",
						},
					},
				},
			},
			wantErr: errors.New("some AWS API error"),
		},
		{
			name: "failed to extract instanceID from some nodes",
			args: args{
				nodes: []*corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "aws:///us-west-2a/i-0fa2d0064e848c69e",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-2",
						},
						Spec: corev1.NodeSpec{
							ProviderID: "",
						},
					},
				},
			},
			wantErr: errors.New("providerID is not specified for node: node-2"),
		},
		{
			name: "empty nodes",
			args: args{
				nodes: nil,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Client := services.NewMockEC2(ctrl)
			for _, call := range tt.fields.describeInstancesAsListCalls {
				ec2Client.EXPECT().DescribeInstancesAsList(gomock.Any(), call.req).Return(call.resp, call.err)
			}
			p := &defaultNodeInfoProvider{
				ec2Client: ec2Client,
				logger:    &log.NullLogger{},
			}
			got, err := p.FetchNodeInstances(context.Background(), tt.args.nodes)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
