package controller

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/golang/glog"

	"k8s.io/ingress/core/pkg/ingress"
	"k8s.io/kubernetes/pkg/apis/extensions"
)

// ALBController is our main controller
type ALBController struct {
	route53svc       *Route53
	elbv2svc         *ELBV2
	ec2svc           *EC2
	storeLister      ingress.StoreLister
	lastAlbIngresses albIngressesT
	clusterName      string
}

type albIngressesT []*albIngress

// NewALBController returns an ALBController
func NewALBController(awsconfig *aws.Config, clusterName string) *ALBController {
	ac := ALBController{
		route53svc:  newRoute53(awsconfig),
		elbv2svc:    newELBV2(awsconfig),
		ec2svc:      newEC2(awsconfig),
		clusterName: clusterName,
	}

	ac.lastAlbIngresses = ac.assembleIngresses()

	return ingress.Controller(&ac).(*ALBController)
}

func (ac *ALBController) OnUpdate(ingressConfiguration ingress.Configuration) ([]byte, error) {
	OnUpdateCount.Add(float64(1))

	var albIngresses albIngressesT

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
				if albIngress.id == lastIngress.id {
					// glog.Info("Found: ", albIngress.id)
					continue NEWINGRESSES
				}
			}

			// new/modified ingress, execute .Create
			glog.Info("ac.Create ", albIngress.id)
			if err := ac.Create(albIngress); err != nil {
				glog.Errorf("Error creating ingress!: %s", err)
			}
		}
	}

	ManagedIngresses.Set(float64(len(albIngresses)))

	// compare albIngresses to ac.lastAlbIngresses
	// execute .Destroy on any that were removed
	for _, albIngress := range ac.lastAlbIngresses {
		if albIngresses.find(albIngress) < 0 {
			glog.Info("ac.Destroy ", albIngress.id)
			ac.Destroy(albIngress)
		}
	}

	ac.lastAlbIngresses = albIngresses
	return []byte(""), nil
}

// assembleIngresses builds a list of ingresses with only the id
func (ac *ALBController) assembleIngresses() albIngressesT {
	var albIngresses albIngressesT
	glog.Info("Build up list of existing ingresses")

	describeLoadBalancersInput := &elbv2.DescribeLoadBalancersInput{
		PageSize: aws.Int64(100),
	}

	for {
		describeLoadBalancersResp, err := ac.elbv2svc.DescribeLoadBalancers(describeLoadBalancersInput)
		if err != nil {
			glog.Fatal(err)
		}

		describeLoadBalancersInput.Marker = describeLoadBalancersResp.NextMarker

		for _, loadBalancer := range describeLoadBalancersResp.LoadBalancers {
			if strings.HasPrefix(*loadBalancer.LoadBalancerName, ac.clusterName+"-") {
				if s := strings.Split(*loadBalancer.LoadBalancerName, "-"); len(s) == 2 {
					if s[0] == ac.clusterName {
						albIngress := &albIngress{
							id: *loadBalancer.LoadBalancerName,
						}
						albIngresses = append(albIngresses, albIngress)
					}
				}
			}
		}

		if describeLoadBalancersResp.NextMarker == nil {
			break
		}
	}

	return albIngresses
}

func (ac *ALBController) Create(a *albIngress) error {
	glog.Infof("Creating an ALB for %v", a.serviceName)
	if err := ac.elbv2svc.alterALB(a); err != nil {
		return err
	}

	glog.Infof("Creating a Route53 record for %v", a.hostname)
	if err := ac.route53svc.upsertRecord(a); err != nil {
		return err
	}
	return nil
}

func (ac *ALBController) Destroy(a *albIngress) error {
	glog.Infof("Deleting the ALB for %v", a.serviceName)
	if err := ac.elbv2svc.deleteALB(a); err != nil {
		return err
	}

	glog.Infof("Deleting the Route53 record for %v", a.hostname)
	if err := ac.route53svc.deleteRecord(a); err != nil {
		return err
	}

	return nil
}

func (a albIngressesT) find(b *albIngress) int {
	for p, v := range a {
		if v.id == b.id {
			return p
		}
	}
	return -1
}
