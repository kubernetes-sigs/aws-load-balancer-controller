package aws

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	ctrl "sigs.k8s.io/controller-runtime"
)

func Test_getVpcID(t *testing.T) {
	logger := ctrl.Log.WithName("test")

	tests := []struct {
		name       string
		cfg        CloudConfig
		setupMocks func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata)
		wantVpcID  string
		wantErr    string
	}{
		{
			name: "explicit vpc-id takes priority over everything",
			cfg: CloudConfig{
				VpcID:   "vpc-explicit",
				VpcTags: map[string]string{"Name": "my-vpc"},
			},
			setupMocks: func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata) {
				// no calls expected
			},
			wantVpcID: "vpc-explicit",
		},
		{
			name: "tags lookup uses all tags as filters",
			cfg: CloudConfig{
				VpcTags: map[string]string{"Name": "my-vpc"},
			},
			setupMocks: func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata) {
				ec2Service.EXPECT().DescribeVPCsAsList(gomock.Any(), &ec2.DescribeVpcsInput{
					Filters: []ec2types.Filter{
						{Name: aws.String("tag:Name"), Values: []string{"my-vpc"}},
					},
				}).Return([]ec2types.Vpc{
					{VpcId: aws.String("vpc-from-name-tag")},
				}, nil)
			},
			wantVpcID: "vpc-from-name-tag",
		},
		{
			name: "tags lookup with multiple tags uses all as AND filters",
			cfg: CloudConfig{
				VpcTags: map[string]string{"foo": "bar", "baz": "buzz"},
			},
			setupMocks: func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata) {
				ec2Service.EXPECT().DescribeVPCsAsList(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, input *ec2.DescribeVpcsInput) ([]ec2types.Vpc, error) {
						assert.Len(t, input.Filters, 2)
						filterNames := map[string]string{}
						for _, f := range input.Filters {
							filterNames[*f.Name] = f.Values[0]
						}
						assert.Equal(t, "bar", filterNames["tag:foo"])
						assert.Equal(t, "buzz", filterNames["tag:baz"])
						return []ec2types.Vpc{
							{VpcId: aws.String("vpc-multi-tag")},
						}, nil
					})
			},
			wantVpcID: "vpc-multi-tag",
		},
		{
			name: "tags lookup with no matching VPCs returns error",
			cfg: CloudConfig{
				VpcTags: map[string]string{"foo": "bar", "baz": "buzz"},
			},
			setupMocks: func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata) {
				ec2Service.EXPECT().DescribeVPCsAsList(gomock.Any(), gomock.Any()).
					Return([]ec2types.Vpc{}, nil)
			},
			wantErr: "no VPC exists with tags",
		},
		{
			name: "tags lookup with multiple matching VPCs returns error",
			cfg: CloudConfig{
				VpcTags: map[string]string{"Env": "prod"},
			},
			setupMocks: func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata) {
				ec2Service.EXPECT().DescribeVPCsAsList(gomock.Any(), gomock.Any()).
					Return([]ec2types.Vpc{
						{VpcId: aws.String("vpc-1")},
						{VpcId: aws.String("vpc-2")},
					}, nil)
			},
			wantErr: "multiple VPCs exist with tags",
		},
		{
			name: "no vpc-id and no tags falls back to IMDS",
			cfg:  CloudConfig{},
			setupMocks: func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata) {
				ec2Metadata.EXPECT().VpcID().Return("vpc-from-imds", nil)
			},
			wantVpcID: "vpc-from-imds",
		},
		{
			name: "IMDS fallback failure returns error",
			cfg:  CloudConfig{},
			setupMocks: func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata) {
				ec2Metadata.EXPECT().VpcID().Return("", fmt.Errorf("IMDS unavailable"))
			},
			wantErr: "failed to fetch VPC ID from instance metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Service := services.NewMockEC2(ctrl)
			ec2Metadata := services.NewMockEC2Metadata(ctrl)
			tt.setupMocks(ec2Service, ec2Metadata)

			got, err := getVpcID(tt.cfg, ec2Service, ec2Metadata, logger)
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantVpcID, got)
			}
		})
	}
}
