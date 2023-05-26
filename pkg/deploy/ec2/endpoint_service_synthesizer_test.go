package ec2

import (
	"context"
	"testing"

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

func Test_synthesize_happyPath(t *testing.T) {
	serviceID := "serviceID"
	t.Parallel()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name               string
		sdkVPCESEnabled    bool
		resVPCESEnabled    bool
		createCalls        int
		deleteCalls        int
		updateCalls        int
		callSynthesize     bool
		callPostSynthesize bool
	}{
		{
			name:               "create with synthesize",
			sdkVPCESEnabled:    false,
			resVPCESEnabled:    true,
			createCalls:        0,
			deleteCalls:        0,
			updateCalls:        0,
			callSynthesize:     true,
			callPostSynthesize: false,
		},
		{
			name:               "delete with synthesize",
			sdkVPCESEnabled:    true,
			resVPCESEnabled:    false,
			createCalls:        0,
			deleteCalls:        1,
			updateCalls:        0,
			callSynthesize:     true,
			callPostSynthesize: false,
		},
		{
			name:               "update with synthesize",
			sdkVPCESEnabled:    true,
			resVPCESEnabled:    true,
			createCalls:        0,
			deleteCalls:        0,
			updateCalls:        0,
			callSynthesize:     true,
			callPostSynthesize: false,
		},
		{
			name:               "create with post synthesize",
			sdkVPCESEnabled:    false,
			resVPCESEnabled:    true,
			createCalls:        1,
			deleteCalls:        0,
			updateCalls:        0,
			callSynthesize:     false,
			callPostSynthesize: true,
		},
		{
			name:               "delete with post synthesize",
			sdkVPCESEnabled:    true,
			resVPCESEnabled:    false,
			createCalls:        0,
			deleteCalls:        0,
			updateCalls:        0,
			callSynthesize:     false,
			callPostSynthesize: true,
		},
		{
			name:               "update with post synthesize",
			sdkVPCESEnabled:    true,
			resVPCESEnabled:    true,
			createCalls:        0,
			deleteCalls:        0,
			updateCalls:        1,
			callSynthesize:     false,
			callPostSynthesize: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockEC2 := services.NewMockEC2(mockCtrl)
			mockTaggingManager := NewMockTaggingManager(mockCtrl)
			mockEndpointServiceManager := NewMockEndpointServiceManager(mockCtrl)

			stack := core.NewDefaultStack(core.StackID{})

			var resVPCES *ec2model.VPCEndpointService
			if tt.resVPCESEnabled {
				resVPCES = &ec2model.VPCEndpointService{
					ResourceMeta: core.NewResourceMeta(stack, "AWS::EC2::VPCEndpointService", "VPCEndpointService"),
				}
				err := stack.AddResource(resVPCES)
				assert.NoError(t, err)
			}

			sdkVPCES := []networking.VPCEndpointServiceInfo{}
			vpcesInfo := networking.VPCEndpointServiceInfo{
				ServiceID: serviceID,
				Tags: map[string]string{
					"prefix/resource": "VPCEndpointService",
				},
			}
			if tt.sdkVPCESEnabled {
				sdkVPCES = []networking.VPCEndpointServiceInfo{vpcesInfo}
			}

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
			if tt.resVPCESEnabled && !tt.sdkVPCESEnabled && tt.callPostSynthesize {
				mockEndpointServiceManager.EXPECT().Create(ctx, resVPCES).Times(tt.createCalls)
			} else {
				mockEndpointServiceManager.EXPECT().Create(ctx, gomock.Any()).Times(tt.createCalls)
			}
			if !tt.resVPCESEnabled && tt.sdkVPCESEnabled && tt.callSynthesize {
				mockEndpointServiceManager.EXPECT().Delete(ctx, vpcesInfo).Times(tt.deleteCalls)
			} else {
				mockEndpointServiceManager.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(tt.deleteCalls)
			}
			if tt.resVPCESEnabled && tt.sdkVPCESEnabled && tt.callSynthesize {
				mockEndpointServiceManager.EXPECT().Update(ctx, resVPCES, vpcesInfo).Times(tt.updateCalls)
			} else {
				mockEndpointServiceManager.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Times(tt.updateCalls)
			}

			if tt.callSynthesize {
				err := synthesizer.Synthesize(ctx)
				assert.NoError(t, err)
			}

			if tt.callPostSynthesize {
				err := synthesizer.PostSynthesize(ctx)
				assert.NoError(t, err)
			}
		})
	}
}

func Test_Synthesize_errorPath(t *testing.T) {
	t.Parallel()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

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

	mockEndpointServiceManager.EXPECT().Delete(ctx, deleteVPCESInfo).Return(errors.New("test_error")).Times(1)

	err := synthesizer.Synthesize(ctx)
	assert.Error(t, err)
}

func Test_postSynthesize_errorPath(t *testing.T) {
	t.Parallel()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name                      string
		createError               error
		updateError               error
		reconcilePermissionsError error
	}{
		{
			name:                      "create endpoint returns an error",
			createError:               errors.New("test_error"),
			updateError:               nil,
			reconcilePermissionsError: nil,
		},
		{
			name:                      "update endpoint returns an error",
			createError:               nil,
			updateError:               errors.New("test_error"),
			reconcilePermissionsError: nil,
		},
		{
			name:                      "reconcile endpoint returns an error",
			createError:               nil,
			updateError:               nil,
			reconcilePermissionsError: errors.New("test_error"),
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
			sdkVPCES := []networking.VPCEndpointServiceInfo{updateVPCESInfo}

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
			mockEndpointServiceManager.EXPECT().Create(ctx, createVPCES).Return(endpointStatus, tt.createError).Times(1)
			if tt.createError == nil {
				mockEndpointServiceManager.EXPECT().Update(ctx, updateVPCES, updateVPCESInfo).Return(endpointStatus, tt.updateError).Times(1)
			} else {
				mockEndpointServiceManager.EXPECT().Update(ctx, gomock.Any(), gomock.Any()).Times(0)
			}
			if tt.createError == nil && tt.updateError == nil {
				mockEndpointServiceManager.EXPECT().ReconcilePermissions(ctx, permissions).Return(tt.reconcilePermissionsError).Times(1)
			} else {
				mockEndpointServiceManager.EXPECT().ReconcilePermissions(ctx, gomock.Any()).Times(0)
			}

			err := synthesizer.PostSynthesize(ctx)
			assert.Error(t, err)
		})
	}
}
