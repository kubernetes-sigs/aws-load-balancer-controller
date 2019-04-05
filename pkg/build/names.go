package build

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"regexp"
	api "sigs.k8s.io/aws-alb-ingress-controller/pkg/apis/ingress/v1alpha1"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/ingress"
)

const (
	ResourceIDLoadBalancer           = "LoadBalancer"
	ResourceIDManagedLBSecurityGroup = "ManagedLBSecurityGroup"
)

var namePtn, _ = regexp.Compile("[[:^alnum:]]")

func (b *defaultBuilder) nameManagedLBSecurityGroup(groupID ingress.GroupID) string {
	uuidHash := md5.New()
	_, _ = uuidHash.Write([]byte(b.cloud.ClusterName()))
	_, _ = uuidHash.Write([]byte(groupID.String()))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	payload := namePtn.ReplaceAllString(groupID.String(), "-")
	return fmt.Sprintf("k8s-%.17s-%.10s", payload, uuid)
}

func (b *defaultBuilder) nameLoadBalancer(groupID ingress.GroupID, schema api.LoadBalancerSchema) string {
	uuidHash := md5.New()
	_, _ = uuidHash.Write([]byte(b.cloud.ClusterName()))
	_, _ = uuidHash.Write([]byte(groupID.String()))
	_, _ = uuidHash.Write([]byte(schema))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	payload := namePtn.ReplaceAllString(groupID.String(), "-")
	return fmt.Sprintf("k8s-%.17s-%.10s", payload, uuid)
}

func (b *defaultBuilder) nameTargetGroup(groupID ingress.GroupID,
	ingKey types.NamespacedName, backend extensions.IngressBackend,
	targetType api.TargetType, protocol api.Protocol) string {

	uuidHash := md5.New()
	_, _ = uuidHash.Write([]byte(b.cloud.ClusterName()))
	_, _ = uuidHash.Write([]byte(groupID.String()))
	_, _ = uuidHash.Write([]byte(ingKey.Namespace))
	_, _ = uuidHash.Write([]byte(ingKey.Name))
	_, _ = uuidHash.Write([]byte(backend.ServiceName))
	_, _ = uuidHash.Write([]byte(backend.ServicePort.String()))
	_, _ = uuidHash.Write([]byte(targetType))
	_, _ = uuidHash.Write([]byte(protocol))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	return fmt.Sprintf("k8s-%.8s-%.8s-%.10s", ingKey.Namespace, backend.ServiceName, uuid)
}

func (b *defaultBuilder) buildTargetGroupID(ingKey types.NamespacedName, backend extensions.IngressBackend) string {
	return fmt.Sprintf("%s/%s-%s:%s", ingKey.Namespace, ingKey.Name, backend.ServiceName, backend.ServicePort.String())
}
