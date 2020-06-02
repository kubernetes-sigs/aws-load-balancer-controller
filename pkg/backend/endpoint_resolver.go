package backend

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/cloud"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/k8s"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"strings"
)

// EndpointResolver resolves the endpoints for specific ingress backend
type EndpointResolver interface {
	Resolve(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString, targetType api.TargetType) ([]*elbv2.TargetDescription, error)
}

// NewEndpointResolver constructs a new EndpointResolver
func NewEndpointResolver(cloud cloud.Cloud, cache cache.Cache) EndpointResolver {
	return &endpointResolver{
		cloud: cloud,
		cache: cache,
	}
}

type endpointResolver struct {
	cloud cloud.Cloud
	cache cache.Cache
}

func (r *endpointResolver) Resolve(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString, targetType api.TargetType) ([]*elbv2.TargetDescription, error) {
	svc := &corev1.Service{}
	if err := r.cache.Get(ctx, svcKey, svc); err != nil {
		return nil, err
	}
	svcPort, err := k8s.LookupServicePort(svc, port)
	if err != nil {
		return nil, err
	}

	if targetType == api.TargetTypeInstance {
		return r.resolveModeInstance(ctx, svc, svcPort)
	}
	return r.resolveModeIP(ctx, svc, svcPort)
}

func (r *endpointResolver) resolveModeInstance(ctx context.Context, svc *corev1.Service, svcPort corev1.ServicePort) ([]*elbv2.TargetDescription, error) {
	if svc.Spec.Type != corev1.ServiceTypeNodePort && svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return nil, errors.Errorf("service %s must be of type NodePort or LoadBalancer when targetType is %s", k8s.NamespacedName(svc).Name, api.TargetTypeInstance)
	}
	nodePort := svcPort.NodePort
	nodes, err := r.getNodePool(ctx)
	if err != nil {
		return nil, err
	}

	var result []*elbv2.TargetDescription
	for _, node := range nodes {
		instanceID, err := r.getNodeInstanceID(ctx, node)
		if err != nil {
			return nil, err
		}
		result = append(result, &elbv2.TargetDescription{
			Id:   aws.String(instanceID),
			Port: aws.Int64(int64(nodePort)),
		})
	}
	return result, nil
}

func (r *endpointResolver) resolveModeIP(ctx context.Context, svc *corev1.Service, svcPort corev1.ServicePort) ([]*elbv2.TargetDescription, error) {
	eps := &corev1.Endpoints{}
	if err := r.cache.Get(ctx, k8s.NamespacedName(svc), eps); err != nil {
		return nil, err
	}

	var result []*elbv2.TargetDescription
	for _, epSubset := range eps.Subsets {
		for _, epPort := range epSubset.Ports {
			// servicePort.Name is optional if there is only one port
			if svcPort.Name != "" && svcPort.Name != epPort.Name {
				continue
			}
			// TODO(@M00nF1sh): populate AZ here
			for _, epAddr := range epSubset.Addresses {
				result = append(result, &elbv2.TargetDescription{
					Id:   aws.String(epAddr.IP),
					Port: aws.Int64(int64(epPort.Port)),
				})
			}
		}
	}

	return result, nil
}

func (r *endpointResolver) getNodePool(ctx context.Context) ([]*corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.cache.List(ctx, nil, nodeList); err != nil {
		return nil, err
	}
	nodes := make([]*corev1.Node, 0, len(nodeList.Items))
	for index, _ := range nodeList.Items {
		node := &nodeList.Items[index]
		if IsNodeSuitableAsTrafficProxy(node) {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func (r *endpointResolver) getNodeInstanceID(ctx context.Context, node *corev1.Node) (string, error) {
	nodeVersion, _ := semver.ParseTolerant(node.Status.NodeInfo.KubeletVersion)
	if nodeVersion.Major == 1 && nodeVersion.Minor <= 10 {
		return node.Spec.DoNotUse_ExternalID, nil
	}

	providerID := node.Spec.ProviderID
	if providerID == "" {
		return "", errors.Errorf("no providerID found for node %s", node.Name)
	}

	parts := strings.Split(providerID, "/")
	return parts[len(parts)-1], nil
}
