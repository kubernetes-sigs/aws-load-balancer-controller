package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

type mockGlobalAccelerator struct {
	mock.Mock
}

func (m *mockGlobalAccelerator) CreateAccelerator(ctx context.Context, input *globalaccelerator.CreateAcceleratorInput) (*globalaccelerator.CreateAcceleratorOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.CreateAcceleratorOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) CreateListener(ctx context.Context, input *globalaccelerator.CreateListenerInput) (*globalaccelerator.CreateListenerOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.CreateListenerOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) CreateEndpointGroup(ctx context.Context, input *globalaccelerator.CreateEndpointGroupInput) (*globalaccelerator.CreateEndpointGroupOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.CreateEndpointGroupOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) DescribeAccelerator(ctx context.Context, input *globalaccelerator.DescribeAcceleratorInput) (*globalaccelerator.DescribeAcceleratorOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.DescribeAcceleratorOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) DeleteAccelerator(ctx context.Context, input *globalaccelerator.DeleteAcceleratorInput) (*globalaccelerator.DeleteAcceleratorOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.DeleteAcceleratorOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) UpdateAccelerator(ctx context.Context, input *globalaccelerator.UpdateAcceleratorInput) (*globalaccelerator.UpdateAcceleratorOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.UpdateAcceleratorOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) UpdateListener(ctx context.Context, input *globalaccelerator.UpdateListenerInput) (*globalaccelerator.UpdateListenerOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.UpdateListenerOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) UpdateEndpointGroup(ctx context.Context, input *globalaccelerator.UpdateEndpointGroupInput) (*globalaccelerator.UpdateEndpointGroupOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.UpdateEndpointGroupOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) DeleteListener(ctx context.Context, input *globalaccelerator.DeleteListenerInput) (*globalaccelerator.DeleteListenerOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.DeleteListenerOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) DeleteEndpointGroup(ctx context.Context, input *globalaccelerator.DeleteEndpointGroupInput) (*globalaccelerator.DeleteEndpointGroupOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.DeleteEndpointGroupOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) ListListeners(ctx context.Context, input *globalaccelerator.ListListenersInput) (*globalaccelerator.ListListenersOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.ListListenersOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) ListEndpointGroups(ctx context.Context, input *globalaccelerator.ListEndpointGroupsInput) (*globalaccelerator.ListEndpointGroupsOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.ListEndpointGroupsOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) ListAccelerators(ctx context.Context, input *globalaccelerator.ListAcceleratorsInput) (*globalaccelerator.ListAcceleratorsOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.ListAcceleratorsOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) DescribeListener(ctx context.Context, input *globalaccelerator.DescribeListenerInput) (*globalaccelerator.DescribeListenerOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.DescribeListenerOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) DescribeEndpointGroup(ctx context.Context, input *globalaccelerator.DescribeEndpointGroupInput) (*globalaccelerator.DescribeEndpointGroupOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.DescribeEndpointGroupOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) TagResource(ctx context.Context, input *globalaccelerator.TagResourceInput) (*globalaccelerator.TagResourceOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.TagResourceOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) UntagResource(ctx context.Context, input *globalaccelerator.UntagResourceInput) (*globalaccelerator.UntagResourceOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.UntagResourceOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) ListTagsForResource(ctx context.Context, input *globalaccelerator.ListTagsForResourceInput) (*globalaccelerator.ListTagsForResourceOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.ListTagsForResourceOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) UpdateAcceleratorAttributes(ctx context.Context, input *globalaccelerator.UpdateAcceleratorAttributesInput) (*globalaccelerator.UpdateAcceleratorAttributesOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.UpdateAcceleratorAttributesOutput), args.Error(1)
}

func (m *mockGlobalAccelerator) DescribeAcceleratorAttributes(ctx context.Context, input *globalaccelerator.DescribeAcceleratorAttributesInput) (*globalaccelerator.DescribeAcceleratorAttributesOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*globalaccelerator.DescribeAcceleratorAttributesOutput), args.Error(1)
}

type mockCloud struct {
	mock.Mock
}

func (m *mockCloud) EC2() services.EC2 {
	args := m.Called()
	return args.Get(0).(services.EC2)
}

func (m *mockCloud) ELBV2() services.ELBV2 {
	args := m.Called()
	return args.Get(0).(services.ELBV2)
}

func (m *mockCloud) ACM() services.ACM {
	args := m.Called()
	return args.Get(0).(services.ACM)
}

func (m *mockCloud) WAFv2() services.WAFv2 {
	args := m.Called()
	return args.Get(0).(services.WAFv2)
}

func (m *mockCloud) WAFRegional() services.WAFRegional {
	args := m.Called()
	return args.Get(0).(services.WAFRegional)
}

func (m *mockCloud) Shield() services.Shield {
	args := m.Called()
	return args.Get(0).(services.Shield)
}

func (m *mockCloud) RGT() services.RGT {
	args := m.Called()
	return args.Get(0).(services.RGT)
}

func (m *mockCloud) GlobalAccelerator() services.GlobalAccelerator {
	args := m.Called()
	return args.Get(0).(services.GlobalAccelerator)
}

func (m *mockCloud) Region() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockCloud) VpcID() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockCloud) GetAssumedRoleELBV2(ctx context.Context, assumeRoleArn string, externalId string) (services.ELBV2, error) {
	args := m.Called(ctx, assumeRoleArn, externalId)
	return args.Get(0).(services.ELBV2), args.Error(1)
}

func TestGlobalAcceleratorReconcilerReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	elbv2api.AddToScheme(scheme)

	ctx := context.Background()
	testCases := []struct {
		name              string
		globalAccelerator *elbv2api.GlobalAccelerator
		expectedResult    ctrl.Result
		expectedError     bool
		mockSetup         func(*mockCloud, *mockGlobalAccelerator)
	}{
		{
			name: "successful reconcile - create new accelerator",
			globalAccelerator: &elbv2api.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ga",
					Namespace: "default",
				},
				Spec: elbv2api.GlobalAcceleratorSpec{
					Name:    aws.String("test-accelerator"),
					Enabled: aws.Bool(true),
					Listeners: []elbv2api.GlobalAcceleratorListener{
						{
							Protocol: elbv2api.GlobalAcceleratorProtocolTCP,
							PortRanges: []elbv2api.PortRange{
								{
									FromPort: 80,
									ToPort:   80,
								},
							},
						},
					},
					EndpointGroups: []elbv2api.EndpointGroup{
						{
							Region: "us-west-2",
							Endpoints: []elbv2api.GlobalAcceleratorEndpoint{
								{
									EndpointID: "test-endpoint",
								},
							},
						},
					},
				},
			},
			expectedResult: ctrl.Result{RequeueAfter: 5 * time.Minute},
			expectedError:  false,
			mockSetup: func(mockCloud *mockCloud, mockGA *mockGlobalAccelerator) {
				mockCloud.On("GlobalAccelerator").Return(mockGA)

				// Mock CreateAccelerator
				mockGA.On("CreateAccelerator", mock.Anything, mock.AnythingOfType("*globalaccelerator.CreateAcceleratorInput")).Return(
					&globalaccelerator.CreateAcceleratorOutput{
						Accelerator: &types.Accelerator{
							AcceleratorArn: aws.String("arn:aws:globalaccelerator::123456789012:accelerator/test-arn"),
						},
					}, nil)

				// Mock CreateListener
				mockGA.On("CreateListener", mock.Anything, mock.AnythingOfType("*globalaccelerator.CreateListenerInput")).Return(
					&globalaccelerator.CreateListenerOutput{
						Listener: &types.Listener{
							ListenerArn: aws.String("arn:aws:globalaccelerator::123456789012:accelerator/test-arn/listener/test-listener"),
						},
					}, nil)

				// Mock CreateEndpointGroup
				mockGA.On("CreateEndpointGroup", mock.Anything, mock.AnythingOfType("*globalaccelerator.CreateEndpointGroupInput")).Return(
					&globalaccelerator.CreateEndpointGroupOutput{
						EndpointGroup: &types.EndpointGroup{
							EndpointGroupArn: aws.String("arn:aws:globalaccelerator::123456789012:accelerator/test-arn/listener/test-listener/endpoint-group/test-endpoint-group"),
						},
					}, nil)
			},
		},
		{
			name: "successful reconcile - existing accelerator",
			globalAccelerator: &elbv2api.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ga",
					Namespace: "default",
				},
				Spec: elbv2api.GlobalAcceleratorSpec{
					Name:    aws.String("test-accelerator"),
					Enabled: aws.Bool(true),
					Listeners: []elbv2api.GlobalAcceleratorListener{
						{
							Protocol: elbv2api.GlobalAcceleratorProtocolTCP,
							PortRanges: []elbv2api.PortRange{
								{
									FromPort: 80,
									ToPort:   80,
								},
							},
						},
					},
					EndpointGroups: []elbv2api.EndpointGroup{
						{
							Region: "us-west-2",
							Endpoints: []elbv2api.GlobalAcceleratorEndpoint{
								{
									EndpointID: "test-endpoint",
								},
							},
						},
					},
				},
				Status: elbv2api.GlobalAcceleratorStatus{
					AcceleratorARN: aws.String("arn:aws:globalaccelerator::123456789012:accelerator/existing-arn"),
				},
			},
			expectedResult: ctrl.Result{RequeueAfter: 5 * time.Minute},
			expectedError:  false,
			mockSetup: func(mockCloud *mockCloud, mockGA *mockGlobalAccelerator) {
				mockCloud.On("GlobalAccelerator").Return(mockGA)

				// Mock DescribeAccelerator - accelerator exists
				mockGA.On("DescribeAccelerator", mock.Anything, mock.AnythingOfType("*globalaccelerator.DescribeAcceleratorInput")).Return(
					&globalaccelerator.DescribeAcceleratorOutput{
						Accelerator: &types.Accelerator{
							AcceleratorArn: aws.String("arn:aws:globalaccelerator::123456789012:accelerator/existing-arn"),
						},
					}, nil)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock
			mockCloud := &mockCloud{}
			mockGA := &mockGlobalAccelerator{}
			tc.mockSetup(mockCloud, mockGA)

			// Setup k8s client
			k8sClient := testclient.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.globalAccelerator).
				WithStatusSubresource(tc.globalAccelerator).
				Build()

			// Setup recorder
			recorder := record.NewFakeRecorder(10)

			// Create reconciler
			reconciler := &GlobalAcceleratorReconciler{
				Client:   k8sClient,
				Scheme:   scheme,
				Logger:   logr.New(&log.NullLogSink{}),
				Recorder: recorder,
				cloud:    mockCloud,
			}

			// Execute
			req := ctrl.Request{
				NamespacedName: k8stypes.NamespacedName{
					Namespace: tc.globalAccelerator.Namespace,
					Name:      tc.globalAccelerator.Name,
				},
			}

			result, err := reconciler.Reconcile(ctx, req)

			// Assert
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expectedResult, result)

			// Verify mocks
			mockCloud.AssertExpectations(t)
			mockGA.AssertExpectations(t)
		})
	}
}

func TestGlobalAcceleratorReconcilerResolveServiceEndpoints(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	elbv2api.AddToScheme(scheme)

	testCases := []struct {
		name              string
		serviceEndpoints  []elbv2api.ServiceEndpointReference
		services          []corev1.Service
		namespace         string
		expectedEndpoints []elbv2api.GlobalAcceleratorEndpoint
		expectedError     bool
	}{
		{
			name: "resolve load balancer service",
			serviceEndpoints: []elbv2api.ServiceEndpointReference{
				{
					Name:   "test-service",
					Weight: aws.Int32(100),
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeLoadBalancer,
					},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									Hostname: "test-lb-hostname.elb.amazonaws.com",
								},
							},
						},
					},
				},
			},
			namespace: "default",
			expectedEndpoints: []elbv2api.GlobalAcceleratorEndpoint{
				{
					EndpointID: "test-lb-hostname.elb.amazonaws.com",
					Weight:     aws.Int32(100),
				},
			},
			expectedError: false,
		},
		{
			name: "service not found",
			serviceEndpoints: []elbv2api.ServiceEndpointReference{
				{
					Name: "nonexistent-service",
				},
			},
			services:          []corev1.Service{},
			namespace:         "default",
			expectedEndpoints: nil,
			expectedError:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup k8s client
			objects := make([]runtime.Object, len(tc.services))
			for i := range tc.services {
				objects[i] = &tc.services[i]
			}

			k8sClient := testclient.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objects...).
				Build()

			// Create reconciler
			reconciler := &GlobalAcceleratorReconciler{
				Client: k8sClient,
				Logger: logr.New(&log.NullLogSink{}),
			}

			// Execute
			endpoints, err := reconciler.resolveServiceEndpoints(
				context.Background(),
				tc.serviceEndpoints,
				tc.namespace,
				logr.New(&log.NullLogSink{}),
			)

			// Assert
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedEndpoints, endpoints)
			}
		})
	}
}

func TestGlobalAcceleratorReconcilerCleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	elbv2api.AddToScheme(scheme)

	ctx := context.Background()
	now := metav1.Now()

	globalAccelerator := &elbv2api.GlobalAccelerator{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-ga",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{globalAcceleratorFinalizer},
		},
		Status: elbv2api.GlobalAcceleratorStatus{
			AcceleratorARN: aws.String("arn:aws:globalaccelerator::123456789012:accelerator/test-arn"),
		},
	}

	// Setup mock
	mockCloud := &mockCloud{}
	mockGA := &mockGlobalAccelerator{}

	mockCloud.On("GlobalAccelerator").Return(mockGA)

	// Mock DescribeAccelerator - accelerator exists
	mockGA.On("DescribeAccelerator", mock.Anything, mock.AnythingOfType("*globalaccelerator.DescribeAcceleratorInput")).Return(
		&globalaccelerator.DescribeAcceleratorOutput{
			Accelerator: &types.Accelerator{
				AcceleratorArn: aws.String("arn:aws:globalaccelerator::123456789012:accelerator/test-arn"),
			},
		}, nil)

	// Mock DeleteAccelerator
	mockGA.On("DeleteAccelerator", mock.Anything, mock.AnythingOfType("*globalaccelerator.DeleteAcceleratorInput")).Return(
		&globalaccelerator.DeleteAcceleratorOutput{}, nil)

	// Setup k8s client
	k8sClient := testclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(globalAccelerator).
		Build()

	// Setup recorder
	recorder := record.NewFakeRecorder(10)

	// Create reconciler
	reconciler := &GlobalAcceleratorReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		Logger:   logr.New(&log.NullLogSink{}),
		Recorder: recorder,
		cloud:    mockCloud,
	}

	// Execute
	req := ctrl.Request{
		NamespacedName: k8stypes.NamespacedName{
			Namespace: globalAccelerator.Namespace,
			Name:      globalAccelerator.Name,
		},
	}

	result, err := reconciler.Reconcile(ctx, req)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify mocks
	mockCloud.AssertExpectations(t)
	mockGA.AssertExpectations(t)
}
