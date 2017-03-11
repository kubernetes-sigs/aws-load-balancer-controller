package controller

import (
	"reflect"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/glog"

	"k8s.io/ingress/core/pkg/ingress"
	"k8s.io/kubernetes/pkg/apis/extensions"
)

var (
	route53svc *Route53
	elbv2svc   *ELBV2
	ec2svc     *EC2
	noop       bool
)

// ALBController is our main controller
type ALBController struct {
	storeLister      ingress.StoreLister
	lastAlbIngresses albIngressesT
	lastNodes        NodeSlice
	clusterName      *string
}

// NewALBController returns an ALBController
func NewALBController(awsconfig *aws.Config, config *Config) *ALBController {
	ac := &ALBController{
		clusterName: aws.String(config.ClusterName),
	}

	route53svc = newRoute53(awsconfig)
	elbv2svc = newELBV2(awsconfig)
	ec2svc = newEC2(awsconfig)
	noop = config.Noop
	ac.lastAlbIngresses = assembleIngresses(ac)

	return ingress.Controller(ac).(*ALBController)
}

func (ac *ALBController) OnUpdate(ingressConfiguration ingress.Configuration) ([]byte, error) {
	OnUpdateCount.Add(float64(1))

	var albIngresses albIngressesT
	nodesChanged := false
	currentNodes := GetNodes(ac)

	if !reflect.DeepEqual(currentNodes, ac.lastNodes) {
		glog.Info("Detected a change in cluster nodes, forcing re-evaluation of ALB targets")
		nodesChanged = true
	}

	ac.lastNodes = currentNodes

	for _, ingress := range ac.storeLister.Ingress.List() {
		if ingress.(*extensions.Ingress).Namespace == "tectonic-system" {
			continue
		}

		// first assemble the albIngress objects
	NEWINGRESSES:
		for _, albIngress := range newAlbIngressesFromIngress(ingress.(*extensions.Ingress), ac) {
			albIngresses = append(albIngresses, albIngress)

			// search for albIngress in ac.lastAlbIngresses, if found and
			// unchanged, continue
			for _, lastIngress := range ac.lastAlbIngresses {
				// TODO: deepequal ingresses
				if *albIngress.id == *lastIngress.id && !nodesChanged {
					continue NEWINGRESSES
				}
			}

			if err := albIngress.createOrModify(); err != nil {
				glog.Errorf("%s: Error creating/modifying ingress!: %s", albIngress.Name(), err)
			}
		}
	}

	ManagedIngresses.Set(float64(len(albIngresses)))

	for _, albIngress := range ac.lastAlbIngresses {
		if albIngresses.find(albIngress) < 0 {
			albIngress.delete()
		}
	}

	ac.lastAlbIngresses = albIngresses
	return []byte(""), nil
}
