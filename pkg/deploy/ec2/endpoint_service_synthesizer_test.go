package ec2

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	ec2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/ec2"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/networking"
)

func Test_synthesize_createFlow(t *testing.T) {
	t.Parallel()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	mockEC2 := services.NewMockEC2(mockCtrl)
	mockTaggingManager := NewMockTaggingManager(mockCtrl)
	mockEndpointServiceManager := NewMockEndpointServiceManager(mockCtrl)

	stack := core.NewDefaultStack(core.StackID{})
	resVPCES := &ec2model.VPCEndpointService{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::EC2::VPCEndpointService", "VPCEndpointService"),
	}
	err := stack.AddResource(resVPCES)

	sdkVPCES := []networking.VPCEndpointServiceInfo{}

	assert.NoError(t, err)

	synthesizer := NewEndpointServiceSynthesizer(
		mockEC2,
		tracking.NewDefaultProvider("prefix", "clusterName"),
		mockTaggingManager,
		mockEndpointServiceManager,
		"vpcIP",
		logr.Discard(),
		stack,
	)

	mockTaggingManager.EXPECT().ListVPCEndpointServices(ctx, gomock.Any(), gomock.Any()).Return(sdkVPCES, nil)
	mockEndpointServiceManager.EXPECT().Create(ctx, resVPCES).Times(1)
	mockEndpointServiceManager.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(0)
	mockEndpointServiceManager.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err = synthesizer.Synthesize(ctx)
	assert.NoError(t, err)
}

func Test_synthesize_deleteFlow(t *testing.T) {
	serviceID := "serviceID"
	t.Parallel()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	mockEC2 := services.NewMockEC2(mockCtrl)
	mockTaggingManager := NewMockTaggingManager(mockCtrl)
	mockEndpointServiceManager := NewMockEndpointServiceManager(mockCtrl)

	stack := core.NewDefaultStack(core.StackID{})

	vpcesInfo := networking.VPCEndpointServiceInfo{
		ServiceID: serviceID,
		Tags: map[string]string{
			"prefix/resource": "VPCEndpointService",
		},
	}
	sdkVPCES := []networking.VPCEndpointServiceInfo{vpcesInfo}

	synthesizer := NewEndpointServiceSynthesizer(
		mockEC2,
		tracking.NewDefaultProvider("prefix", "clusterName"),
		mockTaggingManager,
		mockEndpointServiceManager,
		"vpcIP",
		logr.Discard(),
		stack,
	)

	mockTaggingManager.EXPECT().ListVPCEndpointServices(ctx, gomock.Any(), gomock.Any()).Return(sdkVPCES, nil)

	mockEndpointServiceManager.EXPECT().Create(ctx, gomock.Any()).Times(0)
	mockEndpointServiceManager.EXPECT().Delete(ctx, vpcesInfo).Times(1)
	mockEndpointServiceManager.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := synthesizer.Synthesize(ctx)
	assert.NoError(t, err)
}

func Test_synthesize_updateFlow(t *testing.T) {
	serviceID := "serviceID"
	t.Parallel()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	mockEC2 := services.NewMockEC2(mockCtrl)
	mockTaggingManager := NewMockTaggingManager(mockCtrl)
	mockEndpointServiceManager := NewMockEndpointServiceManager(mockCtrl)

	stack := core.NewDefaultStack(core.StackID{})
	resVPCES := &ec2model.VPCEndpointService{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::EC2::VPCEndpointService", "VPCEndpointService"),
		Spec: ec2model.VPCEndpointServiceSpec{
			AcceptanceRequired: awssdk.Bool(false),
			Tags: map[string]string{
				"prefix/resource": "VPCEndpointService",
			},
		},
	}
	err := stack.AddResource(resVPCES)
	assert.NoError(t, err)

	vpcesInfo := networking.VPCEndpointServiceInfo{
		AcceptanceRequired: true,
		ServiceID:          serviceID,
		Tags: map[string]string{
			"prefix/resource": "VPCEndpointService",
		},
	}
	sdkVPCES := []networking.VPCEndpointServiceInfo{vpcesInfo}

	synthesizer := NewEndpointServiceSynthesizer(
		mockEC2,
		tracking.NewDefaultProvider("prefix", "clusterName"),
		mockTaggingManager,
		mockEndpointServiceManager,
		"vpcIP",
		logr.Discard(),
		stack,
	)

	mockTaggingManager.EXPECT().ListVPCEndpointServices(ctx, gomock.Any(), gomock.Any()).Return(sdkVPCES, nil)

	mockEndpointServiceManager.EXPECT().Create(ctx, gomock.Any()).Times(0)
	mockEndpointServiceManager.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(0)
	mockEndpointServiceManager.EXPECT().Update(ctx, resVPCES, vpcesInfo).Times(1)

	err = synthesizer.Synthesize(ctx)
	assert.NoError(t, err)
}

func Test_synthesize_errorFlow(t *testing.T) {
	t.Parallel()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name                      string
		deleteError               error
		createError               error
		updateError               error
		reconcilePermissionsError error
	}{
		{
			name:                      "list endpoints returns an error",
			deleteError:               errors.New("test_error"),
			createError:               nil,
			updateError:               nil,
			reconcilePermissionsError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockEC2 := services.NewMockEC2(mockCtrl)
			mockTaggingManager := NewMockTaggingManager(mockCtrl)
			mockEndpointServiceManager := NewMockEndpointServiceManager(mockCtrl)

			stack := core.NewDefaultStack(core.StackID{})
			updateVPCES := &ec2model.VPCEndpointService{
				ResourceMeta: core.NewResourceMeta(stack, "AWS::EC2::VPCEndpointService", "VPCEndpointServiceUpdate"),
			}
			createVPCES := &ec2model.VPCEndpointService{
				ResourceMeta: core.NewResourceMeta(stack, "AWS::EC2::VPCEndpointService", "VPCEndpointServiceCreate"),
			}
			permissions := &ec2model.VPCEndpointServicePermissions{
				ResourceMeta: core.NewResourceMeta(stack, "AWS::EC2::VPCEndpointService", "VPCEndpointServicePermissions"),
			}
			_ = stack.AddResource(updateVPCES)
			_ = stack.AddResource(createVPCES)
			_ = stack.AddResource(permissions)

			updateVPCESInfo := networking.VPCEndpointServiceInfo{
				AcceptanceRequired: true,
				ServiceID:          "serviceID",
				Tags: map[string]string{
					"prefix/resource": "VPCEndpointServiceUpdate",
				},
			}
			deleteVPCESInfo := networking.VPCEndpointServiceInfo{
				AcceptanceRequired: true,
				ServiceID:          "serviceID",
				Tags: map[string]string{
					"prefix/resource": "VPCEndpointServiceDelete",
				},
			}
			sdkVPCES := []networking.VPCEndpointServiceInfo{updateVPCESInfo, deleteVPCESInfo}

			synthesizer := NewEndpointServiceSynthesizer(
				mockEC2,
				tracking.NewDefaultProvider("prefix", "clusterName"),
				mockTaggingManager,
				mockEndpointServiceManager,
				"vpcIP",
				logr.Discard(),
				stack,
			)

			mockTaggingManager.EXPECT().ListVPCEndpointServices(ctx, gomock.Any(), gomock.Any()).Return(sdkVPCES, nil)

			endpointStatus := ec2model.VPCEndpointServiceStatus{ServiceID: "serviceID"}
			mockEndpointServiceManager.EXPECT().Delete(ctx, deleteVPCESInfo).Return(tt.deleteError).Times(1)
			if tt.deleteError == nil {
				mockEndpointServiceManager.EXPECT().Create(ctx, createVPCES).Return(endpointStatus, tt.createError).Times(1)
			}
			if tt.deleteError == nil && tt.createError == nil {
				mockEndpointServiceManager.EXPECT().Update(ctx, updateVPCES, updateVPCESInfo).Return(endpointStatus, tt.updateError).Times(1)
			}
			if tt.deleteError == nil && tt.createError == nil && tt.updateError == nil {
				mockEndpointServiceManager.EXPECT().ReconcilePermissions(ctx, permissions).Return(tt.reconcilePermissionsError).Times(1)
			}

			err := synthesizer.Synthesize(ctx)
			assert.Error(t, err)
		})
	}
}
