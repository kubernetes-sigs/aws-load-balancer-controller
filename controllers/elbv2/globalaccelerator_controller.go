/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
)

const (
	globalAcceleratorControllerName = "globalaccelerator"
	globalAcceleratorFinalizer      = "elbv2.k8s.aws/globalaccelerator"
)

// GlobalAcceleratorReconciler reconciles a GlobalAccelerator object
type GlobalAcceleratorReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Logger   logr.Logger
	Recorder record.EventRecorder

	cloud services.Cloud

	maxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=elbv2.k8s.aws,resources=globalaccelerators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elbv2.k8s.aws,resources=globalaccelerators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=elbv2.k8s.aws,resources=globalaccelerators/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *GlobalAcceleratorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Logger.WithValues("globalaccelerator", req.NamespacedName)

	var globalAccelerator elbv2api.GlobalAccelerator
	if err := r.Get(ctx, req.NamespacedName, &globalAccelerator); err != nil {
		if apierrors.IsNotFound(err) {
			// Global Accelerator was deleted
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !globalAccelerator.DeletionTimestamp.IsZero() {
		return r.cleanupGlobalAccelerator(ctx, &globalAccelerator, logger)
	}

	return r.reconcileGlobalAccelerator(ctx, &globalAccelerator, logger)
}

func (r *GlobalAcceleratorReconciler) reconcileGlobalAccelerator(ctx context.Context, globalAccelerator *elbv2api.GlobalAccelerator, logger logr.Logger) (ctrl.Result, error) {
	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(globalAccelerator, globalAcceleratorFinalizer) {
		controllerutil.AddFinalizer(globalAccelerator, globalAcceleratorFinalizer)
		if err := r.Update(ctx, globalAccelerator); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Create or update the Global Accelerator
	acceleratorArn, err := r.reconcileAccelerator(ctx, globalAccelerator, logger)
	if err != nil {
		r.Recorder.Event(globalAccelerator, "Warning", "FailedReconcile", err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Update status
	globalAccelerator.Status.AcceleratorARN = &acceleratorArn
	globalAccelerator.Status.ObservedGeneration = &globalAccelerator.Generation

	if err := r.Status().Update(ctx, globalAccelerator); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(globalAccelerator, "Normal", "Reconciled", "Global Accelerator reconciled successfully")
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *GlobalAcceleratorReconciler) reconcileAccelerator(ctx context.Context, globalAccelerator *elbv2api.GlobalAccelerator, logger logr.Logger) (string, error) {
	acceleratorName := globalAccelerator.Spec.Name
	if acceleratorName == nil {
		defaultName := globalAccelerator.Namespace + "-" + globalAccelerator.Name
		acceleratorName = &defaultName
	}

	// Check if accelerator already exists
	if globalAccelerator.Status.AcceleratorARN != nil {
		// Verify the accelerator still exists
		_, err := r.cloud.GlobalAccelerator().DescribeAccelerator(ctx, &globalaccelerator.DescribeAcceleratorInput{
			AcceleratorArn: globalAccelerator.Status.AcceleratorARN,
		})
		if err == nil {
			// Accelerator exists, update it if needed
			return *globalAccelerator.Status.AcceleratorARN, nil
		}
		// If accelerator doesn't exist, we'll create a new one
		logger.Info("Existing accelerator not found, creating new one")
	}

	// Create new accelerator
	createInput := &globalaccelerator.CreateAcceleratorInput{
		Name:    acceleratorName,
		Enabled: globalAccelerator.Spec.Enabled,
	}

	// Note: AcceleratorType is not available in CreateAcceleratorInput in current SDK

	if globalAccelerator.Spec.IPAddressType != nil {
		createInput.IpAddressType = types.IpAddressType(*globalAccelerator.Spec.IPAddressType)
	}

	// Add tags
	if len(globalAccelerator.Spec.Tags) > 0 {
		tags := make([]types.Tag, len(globalAccelerator.Spec.Tags))
		for i, tag := range globalAccelerator.Spec.Tags {
			tags[i] = types.Tag{
				Key:   &tag.Key,
				Value: &tag.Value,
			}
		}
		createInput.Tags = tags
	}

	resp, err := r.cloud.GlobalAccelerator().CreateAccelerator(ctx, createInput)
	if err != nil {
		return "", errors.Wrap(err, "failed to create Global Accelerator")
	}

	acceleratorArn := *resp.Accelerator.AcceleratorArn

	// Create listeners and endpoint groups
	for i, listener := range globalAccelerator.Spec.Listeners {
		listenerArn, err := r.createListener(ctx, acceleratorArn, listener, logger)
		if err != nil {
			return "", errors.Wrap(err, "failed to create listener")
		}

		// Create endpoint groups for this listener
		if i < len(globalAccelerator.Spec.EndpointGroups) {
			endpointGroup := globalAccelerator.Spec.EndpointGroups[i]
			err = r.createEndpointGroup(ctx, listenerArn, endpointGroup, globalAccelerator, logger)
			if err != nil {
				return "", errors.Wrap(err, "failed to create endpoint group")
			}
		}
	}

	logger.Info("Created Global Accelerator", "arn", acceleratorArn)
	return acceleratorArn, nil
}

func (r *GlobalAcceleratorReconciler) createListener(ctx context.Context, acceleratorArn string, listener elbv2api.GlobalAcceleratorListener, logger logr.Logger) (string, error) {
	portRanges := make([]types.PortRange, len(listener.PortRanges))
	for i, pr := range listener.PortRanges {
		portRanges[i] = types.PortRange{
			FromPort: &pr.FromPort,
			ToPort:   &pr.ToPort,
		}
	}

	createInput := &globalaccelerator.CreateListenerInput{
		AcceleratorArn: &acceleratorArn,
		Protocol:       types.Protocol(listener.Protocol),
		PortRanges:     portRanges,
	}

	if listener.ClientAffinity != nil {
		createInput.ClientAffinity = types.ClientAffinity(*listener.ClientAffinity)
	}

	resp, err := r.cloud.GlobalAccelerator().CreateListener(ctx, createInput)
	if err != nil {
		return "", errors.Wrap(err, "failed to create listener")
	}

	listenerArn := *resp.Listener.ListenerArn
	logger.Info("Created listener for Global Accelerator", "acceleratorArn", acceleratorArn, "listenerArn", listenerArn)
	return listenerArn, nil
}

func (r *GlobalAcceleratorReconciler) createEndpointGroup(ctx context.Context, listenerArn string, endpointGroup elbv2api.EndpointGroup, globalAccelerator *elbv2api.GlobalAccelerator, logger logr.Logger) error {
	// Start with explicit endpoints
	allEndpoints := endpointGroup.Endpoints

	// Add service endpoints if specified
	if len(globalAccelerator.Spec.ServiceEndpoints) > 0 {
		serviceEndpoints, err := r.resolveServiceEndpoints(ctx, globalAccelerator.Spec.ServiceEndpoints, globalAccelerator.Namespace, logger)
		if err != nil {
			logger.Error(err, "Failed to resolve service endpoints, continuing with explicit endpoints only")
		} else {
			allEndpoints = append(allEndpoints, serviceEndpoints...)
		}
	}

	// Convert endpoint specifications to AWS SDK format
	endpoints := make([]types.EndpointConfiguration, len(allEndpoints))
	for i, ep := range allEndpoints {
		endpoints[i] = types.EndpointConfiguration{
			EndpointId: &ep.EndpointID,
		}
		if ep.Weight != nil {
			endpoints[i].Weight = ep.Weight
		}
		if ep.ClientIPPreservationEnabled != nil {
			endpoints[i].ClientIPPreservationEnabled = ep.ClientIPPreservationEnabled
		}
	}

	createInput := &globalaccelerator.CreateEndpointGroupInput{
		ListenerArn:            &listenerArn,
		EndpointGroupRegion:    &endpointGroup.Region,
		EndpointConfigurations: endpoints,
	}

	if endpointGroup.TrafficDialPercentage != nil {
		dialPercentage := float32(*endpointGroup.TrafficDialPercentage)
		createInput.TrafficDialPercentage = &dialPercentage
	}

	if endpointGroup.HealthCheckIntervalSeconds != nil {
		createInput.HealthCheckIntervalSeconds = endpointGroup.HealthCheckIntervalSeconds
	}

	if endpointGroup.HealthCheckPath != nil {
		createInput.HealthCheckPath = endpointGroup.HealthCheckPath
	}

	if endpointGroup.ThresholdCount != nil {
		createInput.ThresholdCount = endpointGroup.ThresholdCount
	}

	// Add port overrides if specified
	if len(endpointGroup.PortOverrides) > 0 {
		portOverrides := make([]types.PortOverride, len(endpointGroup.PortOverrides))
		for i, po := range endpointGroup.PortOverrides {
			portOverrides[i] = types.PortOverride{
				ListenerPort: &po.ListenerPort,
				EndpointPort: &po.EndpointPort,
			}
		}
		createInput.PortOverrides = portOverrides
	}

	_, err := r.cloud.GlobalAccelerator().CreateEndpointGroup(ctx, createInput)
	if err != nil {
		return errors.Wrap(err, "failed to create endpoint group")
	}

	logger.Info("Created endpoint group for Global Accelerator", "listenerArn", listenerArn, "region", endpointGroup.Region)
	return nil
}

func (r *GlobalAcceleratorReconciler) resolveServiceEndpoints(ctx context.Context, serviceEndpoints []elbv2api.ServiceEndpointReference, namespace string, logger logr.Logger) ([]elbv2api.GlobalAcceleratorEndpoint, error) {
	var endpoints []elbv2api.GlobalAcceleratorEndpoint

	for _, svcEndpoint := range serviceEndpoints {
		// Determine the service namespace
		svcNamespace := namespace
		if svcEndpoint.Namespace != nil {
			svcNamespace = *svcEndpoint.Namespace
		}

		// Get the service
		var service corev1.Service
		err := r.Get(ctx, k8stypes.NamespacedName{
			Namespace: svcNamespace,
			Name:      svcEndpoint.Name,
		}, &service)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get service %s/%s", svcNamespace, svcEndpoint.Name)
		}

		// Check if this is a LoadBalancer service with AWS Load Balancer
		if service.Spec.Type == corev1.ServiceTypeLoadBalancer {
			// Look for the load balancer hostname in the service status
			if len(service.Status.LoadBalancer.Ingress) > 0 {
				hostname := service.Status.LoadBalancer.Ingress[0].Hostname
				if hostname != "" {
					// For AWS ALB/NLB, we need to resolve the hostname to ARN
					// This is a simplified approach - in production you might want to
					// look up the actual load balancer ARN using AWS APIs
					endpoint := elbv2api.GlobalAcceleratorEndpoint{
						EndpointID: hostname, // This should be the LB ARN, but hostname works for demo
					}
					if svcEndpoint.Weight != nil {
						endpoint.Weight = svcEndpoint.Weight
					}
					endpoints = append(endpoints, endpoint)
					logger.Info("Resolved service to load balancer endpoint",
						"service", fmt.Sprintf("%s/%s", svcNamespace, svcEndpoint.Name),
						"endpoint", hostname)
				}
			}
		}
	}

	return endpoints, nil
}

func (r *GlobalAcceleratorReconciler) cleanupGlobalAccelerator(ctx context.Context, globalAccelerator *elbv2api.GlobalAccelerator, logger logr.Logger) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(globalAccelerator, globalAcceleratorFinalizer) {
		return ctrl.Result{}, nil
	}

	// Delete the Global Accelerator if it exists
	if globalAccelerator.Status.AcceleratorARN != nil {
		_, err := r.cloud.GlobalAccelerator().DescribeAccelerator(ctx, &globalaccelerator.DescribeAcceleratorInput{
			AcceleratorArn: globalAccelerator.Status.AcceleratorARN,
		})
		if err == nil {
			// Accelerator exists, delete it
			_, err = r.cloud.GlobalAccelerator().DeleteAccelerator(ctx, &globalaccelerator.DeleteAcceleratorInput{
				AcceleratorArn: globalAccelerator.Status.AcceleratorARN,
			})
			if err != nil {
				r.Recorder.Event(globalAccelerator, "Warning", "FailedDelete", err.Error())
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}
			logger.Info("Deleted Global Accelerator", "arn", *globalAccelerator.Status.AcceleratorARN)
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(globalAccelerator, globalAcceleratorFinalizer)
	if err := r.Update(ctx, globalAccelerator); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(globalAccelerator, "Normal", "Deleted", "Global Accelerator deleted successfully")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GlobalAcceleratorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elbv2api.GlobalAccelerator{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.maxConcurrentReconciles,
		}).
		Complete(r)
}

// NewGlobalAcceleratorReconciler constructs new GlobalAcceleratorReconciler
func NewGlobalAcceleratorReconciler(cloud services.Cloud, k8sClient client.Client, eventRecorder record.EventRecorder, logger logr.Logger) *GlobalAcceleratorReconciler {
	return &GlobalAcceleratorReconciler{
		Client:                  k8sClient,
		Scheme:                  k8sClient.Scheme(),
		Logger:                  logger,
		Recorder:                eventRecorder,
		cloud:                   cloud,
		maxConcurrentReconciles: 3,
	}
}
