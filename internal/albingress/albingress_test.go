package albingress

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/annotations"
	"k8s.io/api/extensions/v1beta1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var a *ALBIngress

func setup() {
	//setupEC2()
	//setupELBV2()

	a = &ALBIngress{
		id:          "clustername-ingressname",
		namespace:   "namespace",
		clusterName: "clustername",
		ingressName: "ingressname",
		// annotations: annotations,
		// nodes:       GetNodes(ac),
	}

}

func TestNewALBIngressFromIngress(t *testing.T) {
	options := &NewALBIngressFromIngressOptions{
		Ingress: &extensions.Ingress{
			Spec: extensions.IngressSpec{
				Rules: []v1beta1.IngressRule{
					v1beta1.IngressRule{
						Host: "example.com",
						IngressRuleValue: v1beta1.IngressRuleValue{
							HTTP: &v1beta1.HTTPIngressRuleValue{
								Paths: []v1beta1.HTTPIngressPath{
									v1beta1.HTTPIngressPath{
										Path: "/",
										Backend: v1beta1.IngressBackend{
											ServicePort: intstr.FromInt(80),
											ServiceName: "testService",
										},
									},
								},
							},
						},
					},
				},
			},
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"alb.ingress.kubernetes.io/subnets":         "subnet-1,subnet-2",
					"alb.ingress.kubernetes.io/security-groups": "sg-1",
					"alb.ingress.kubernetes.io/scheme":          "internet-facing",
				},
				ClusterName: "testCluster",
				Namespace:   "test",
				Name:        "testIngress",
			},
		},
		ClusterName:   "testCluster",
		ALBNamePrefix: "albNamePrefix",
		AnnotationFactory: annotations.NewValidatingAnnotationFactory(&annotations.NewValidatingAnnotationFactoryOptions{
			Validator:   annotations.FakeValidator{VpcId: "vpc-1"},
			ClusterName: aws.String("testCluster"),
		},
		),
	}
	ingress := NewALBIngressFromIngress(options)
	if ingress == nil {
		t.Errorf("NewALBIngressFromIngress returned nil")
	}
}
