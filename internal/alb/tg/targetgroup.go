package tg

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/tags"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/healthcheck"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/backend"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/controller/store"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/k8s"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
	"github.com/pkg/errors"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The port used when creating targetGroup serves as a default value for targets registered without port specified.
// there are cases that a single targetGroup contains different ports, e.g. backend service targets multiple deployments with targetPort
// as "http", but "http" points to 80 or 8080 in different deployment.
// So we used a dummy(but valid) port number when creating targetGroup, and register targets with port number explicitly.
// see https://docs.aws.amazon.com/sdk-for-go/api/service/elbv2/#CreateTargetGroupInput
const targetGroupDefaultPort = 1

// Controller manages a single targetGroup for specific ingress & ingressBackend.
type Controller interface {
	// Reconcile ensures an targetGroup exists for specified backend of ingress.
	Reconcile(ctx context.Context, ingress *extensions.Ingress, backend extensions.IngressBackend) (TargetGroup, error)
	StopReconcilingPodConditionStatus(tgArn string)
}

func NewController(cloud aws.CloudAPI, store store.Storer, nameTagGen NameTagGenerator, tagsController tags.Controller, endpointResolver backend.EndpointResolver, client client.Client) Controller {
	attrsController := NewAttributesController(cloud)
	targetHealthController := NewTargetHealthController(cloud, store, endpointResolver, client)
	targetsController := NewTargetsController(cloud, endpointResolver, targetHealthController)
	return &defaultController{
		cloud:             cloud,
		store:             store,
		nameTagGen:        nameTagGen,
		tagsController:    tagsController,
		attrsController:   attrsController,
		targetsController: targetsController,
	}
}

var _ Controller = (*defaultController)(nil)

type defaultController struct {
	cloud      aws.CloudAPI
	store      store.Storer
	nameTagGen NameTagGenerator

	tagsController    tags.Controller
	attrsController   AttributesController
	targetsController TargetsController
}

func (controller *defaultController) Reconcile(ctx context.Context, ingress *extensions.Ingress, backend extensions.IngressBackend) (TargetGroup, error) {
	ingressAnnos, err := controller.store.GetIngressAnnotations(k8s.MetaNamespaceKey(ingress))
	if err != nil {
		return TargetGroup{}, fmt.Errorf("failed to load ingressAnnotation due to %v", err)
	}
	serviceKey := types.NamespacedName{Namespace: ingress.Namespace, Name: backend.ServiceName}
	serviceAnnos, err := controller.store.GetServiceAnnotations(serviceKey.String(), ingressAnnos)
	if err != nil {
		return TargetGroup{}, fmt.Errorf("failed to load serviceAnnotation due to %v", err)
	}

	protocol := aws.StringValue(serviceAnnos.TargetGroup.BackendProtocol)
	targetType := aws.StringValue(serviceAnnos.TargetGroup.TargetType)

	healthCheckPort, err := controller.resolveServiceHealthCheckPort(ingress.Namespace, backend.ServiceName, intstr.Parse(*serviceAnnos.HealthCheck.Port), targetType)

	if err != nil {
		return TargetGroup{}, fmt.Errorf("failed to resolve healthcheck port due to %v", err)
	}

	tgName := controller.nameTagGen.NameTG(ingress.Namespace, ingress.Name, backend.ServiceName, backend.ServicePort.String(), targetType, protocol)
	tgInstance, err := controller.findExistingTGInstance(ctx, tgName)
	if err != nil {
		return TargetGroup{}, fmt.Errorf("failed to find existing targetGroup due to %v", err)
	}
	if tgInstance == nil {
		if tgInstance, err = controller.newTGInstance(ctx, tgName, serviceAnnos, healthCheckPort); err != nil {
			return TargetGroup{}, fmt.Errorf("failed to create targetGroup due to %v", err)
		}
	} else {
		if tgInstance, err = controller.reconcileTGInstance(ctx, tgInstance, serviceAnnos, healthCheckPort); err != nil {
			return TargetGroup{}, fmt.Errorf("failed to modify targetGroup due to %v", err)
		}
	}

	tgArn := aws.StringValue(tgInstance.TargetGroupArn)
	tgTags := controller.buildTags(ingress, backend, ingressAnnos)
	if err := controller.tagsController.ReconcileELB(ctx, tgArn, tgTags); err != nil {
		return TargetGroup{}, fmt.Errorf("failed to reconcile targetGroup tags due to %v", err)
	}
	if err := controller.attrsController.Reconcile(ctx, tgArn, serviceAnnos.TargetGroup.Attributes); err != nil {
		return TargetGroup{}, fmt.Errorf("failed to reconcile targetGroup attributes due to %v", err)
	}
	tgTargets := NewTargets(targetType, ingress, &backend)
	tgTargets.TgArn = tgArn
	if err = controller.targetsController.Reconcile(ctx, tgTargets); err != nil {
		return TargetGroup{}, fmt.Errorf("failed to reconcile targetGroup targets due to %v", err)
	}

	return TargetGroup{
		Arn:        tgArn,
		TargetType: targetType,
		Targets:    tgTargets.Targets,
	}, nil
}

func (controller *defaultController) StopReconcilingPodConditionStatus(tgArn string) {
	controller.targetsController.StopReconcilingPodConditionStatus(tgArn)
}

func (controller *defaultController) newTGInstance(ctx context.Context, name string, serviceAnnos *annotations.Service, healthCheckPort string) (*elbv2.TargetGroup, error) {
	albctx.GetLogger(ctx).Infof("creating target group %v", name)
	resp, err := controller.cloud.CreateTargetGroupWithContext(ctx, &elbv2.CreateTargetGroupInput{
		Name:                       aws.String(name),
		HealthCheckPath:            serviceAnnos.HealthCheck.Path,
		HealthCheckIntervalSeconds: serviceAnnos.HealthCheck.IntervalSeconds,
		HealthCheckPort:            aws.String(healthCheckPort),
		HealthCheckProtocol:        serviceAnnos.HealthCheck.Protocol,
		HealthCheckTimeoutSeconds:  serviceAnnos.HealthCheck.TimeoutSeconds,
		TargetType:                 serviceAnnos.TargetGroup.TargetType,
		Protocol:                   serviceAnnos.TargetGroup.BackendProtocol,
		Matcher:                    &elbv2.Matcher{HttpCode: serviceAnnos.TargetGroup.SuccessCodes},
		HealthyThresholdCount:      serviceAnnos.TargetGroup.HealthyThresholdCount,
		UnhealthyThresholdCount:    serviceAnnos.TargetGroup.UnhealthyThresholdCount,
		Port:                       aws.Int64(targetGroupDefaultPort),
	})
	if err != nil {
		return nil, err
	}
	tgInstance := resp.TargetGroups[0]
	albctx.GetLogger(ctx).Infof("target group %v created: %v", name, aws.StringValue(tgInstance.TargetGroupArn))
	return tgInstance, nil
}

func (controller *defaultController) reconcileTGInstance(ctx context.Context, instance *elbv2.TargetGroup, serviceAnnos *annotations.Service, healthCheckPort string) (*elbv2.TargetGroup, error) {
	if controller.TGInstanceNeedsModification(ctx, instance, serviceAnnos) {
		albctx.GetLogger(ctx).Infof("modify target group %v", aws.StringValue(instance.TargetGroupArn))

		output, err := controller.cloud.ModifyTargetGroupWithContext(ctx, &elbv2.ModifyTargetGroupInput{
			TargetGroupArn:             instance.TargetGroupArn,
			HealthCheckPath:            serviceAnnos.HealthCheck.Path,
			HealthCheckIntervalSeconds: serviceAnnos.HealthCheck.IntervalSeconds,
			HealthCheckPort:            aws.String(healthCheckPort),
			HealthCheckProtocol:        serviceAnnos.HealthCheck.Protocol,
			HealthCheckTimeoutSeconds:  serviceAnnos.HealthCheck.TimeoutSeconds,
			Matcher:                    &elbv2.Matcher{HttpCode: serviceAnnos.TargetGroup.SuccessCodes},
			HealthyThresholdCount:      serviceAnnos.TargetGroup.HealthyThresholdCount,
			UnhealthyThresholdCount:    serviceAnnos.TargetGroup.UnhealthyThresholdCount,
		})
		if err != nil {
			return instance, err
		}
		return output.TargetGroups[0], err
	}
	return instance, nil
}

// resolveServiceHealthCheckPort checks if the service-port annotation is a string. If so, it tries to look up a port with the same name
// on the service and use that port's NodePort as the health check port.
func (controller *defaultController) resolveServiceHealthCheckPort(namespace string, serviceName string, servicePortAnnotation intstr.IntOrString, targetType string) (string, error) {

	if servicePortAnnotation.Type == intstr.Int {
		//Nothing to do if it's an Int - return original value
		return servicePortAnnotation.String(), nil
	}

	servicePort := servicePortAnnotation.String()

	//If the annotation uses the default port ("traffic-port"), do not try to look up a port by that name.
	if servicePort == healthcheck.DefaultPort {
		return servicePort, nil
	}

	serviceKey := namespace + "/" + serviceName
	service, err := controller.store.GetService(serviceKey)

	if err != nil {
		return servicePort, errors.Wrap(err, "failed to resolve healthcheck service name")
	}

	resolvedServicePort, err := k8s.LookupServicePort(service, servicePortAnnotation)
	if err != nil {
		return servicePort, errors.Wrap(err, "failed to resolve healthcheck port for service")
	}
	if targetType == elbv2.TargetTypeEnumInstance {
		if resolvedServicePort.NodePort == 0 {
			return servicePort, fmt.Errorf("failed to find valid NodePort for service %s with port %s", serviceName, resolvedServicePort.Name)
		}
		return strconv.Itoa(int(resolvedServicePort.NodePort)), nil
	}
	return resolvedServicePort.TargetPort.String(), nil

}

func (controller *defaultController) TGInstanceNeedsModification(ctx context.Context, instance *elbv2.TargetGroup, serviceAnnos *annotations.Service) bool {
	needsChange := false
	if !util.DeepEqual(instance.HealthCheckPath, serviceAnnos.HealthCheck.Path) {
		needsChange = true
	}
	if !util.DeepEqual(instance.HealthCheckPort, serviceAnnos.HealthCheck.Port) {
		needsChange = true
	}
	if !util.DeepEqual(instance.HealthCheckProtocol, serviceAnnos.HealthCheck.Protocol) {
		needsChange = true
	}
	if !util.DeepEqual(instance.HealthCheckIntervalSeconds, serviceAnnos.HealthCheck.IntervalSeconds) {
		needsChange = true
	}
	if !util.DeepEqual(instance.HealthCheckTimeoutSeconds, serviceAnnos.HealthCheck.TimeoutSeconds) {
		needsChange = true
	}
	if !util.DeepEqual(instance.Matcher.HttpCode, serviceAnnos.TargetGroup.SuccessCodes) {
		needsChange = true
	}
	if !util.DeepEqual(instance.HealthyThresholdCount, serviceAnnos.TargetGroup.HealthyThresholdCount) {
		needsChange = true
	}
	if !util.DeepEqual(instance.UnhealthyThresholdCount, serviceAnnos.TargetGroup.UnhealthyThresholdCount) {
		needsChange = true
	}
	return needsChange
}

func (controller *defaultController) buildTags(ingress *extensions.Ingress, backend extensions.IngressBackend, ingressAnnos *annotations.Ingress) map[string]string {
	tgTags := make(map[string]string)
	for k, v := range controller.nameTagGen.TagTGGroup(ingress.Namespace, ingress.Name) {
		tgTags[k] = v
	}
	for k, v := range controller.nameTagGen.TagTG(ingress.Namespace, ingress.Name, backend.ServiceName, backend.ServicePort.String()) {
		tgTags[k] = v
	}
	for k, v := range ingressAnnos.Tags.LoadBalancer {
		tgTags[k] = v
	}
	return tgTags
}

func (controller *defaultController) findExistingTGInstance(ctx context.Context, tgName string) (*elbv2.TargetGroup, error) {
	return controller.cloud.GetTargetGroupByName(ctx, tgName)
}
