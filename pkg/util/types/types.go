package types

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

const (
	IdleTimeoutKey           = "idle_timeout.timeout_seconds"
	restrictIngressConfigMap = "alb-ingress-controller-internet-facing-ingresses"
)

type AvailabilityZones []*elbv2.AvailabilityZone

var logger *log.Logger

func init() {
	logger = log.New("util")
}

func DeepEqual(x, y interface{}) bool {
	b := awsutil.DeepEqual(x, y)
	if b == false {
		logger.Debugf("DeepEqual(%v, %v) found inequality", log.Prettify(x), log.Prettify(y))
	}
	return b
}

// Get the configmap that holds the whitelisted internet facing
func GetInternetFacingConfigMap(namespace string) map[string]string {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Errorf(err.Error())
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Errorf(err.Error())
	}
	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(restrictIngressConfigMap, metav1.GetOptions{})
	if err != nil {
		logger.Errorf(err.Error())
	}
	return cm.Data
}

// Returns true if the namespace/ingress allows creating internet-facing ALBs
func IngressAllowedExternal(configNamespace, namespace, ingressName string) bool {
	cm := GetInternetFacingConfigMap(configNamespace)
	for ns, ingressString := range cm {
		ingressString := strings.Replace(ingressString, " ", "", -1)
		ingresses := strings.Split(ingressString, ",")
		for _, ing := range ingresses {
			if namespace == ns && ing == ingressName {
				return true
			}
		}
	}
	return false
}

func SortedMap(m map[string]string) []string {
	var t []string
	for k, v := range m {
		t = append(t, fmt.Sprintf("%v:%v", k, v))
	}
	sort.Strings(t)
	return t
}

func (az AvailabilityZones) AsSubnets() AWSStringSlice {
	var out []*string
	for _, a := range az {
		out = append(out, a.SubnetId)
	}
	return out
}

func (subnets Subnets) AsAvailabilityZones() AvailabilityZones {
	var out []*elbv2.AvailabilityZone
	for _, s := range subnets {
		out = append(out, &elbv2.AvailabilityZone{SubnetId: s, ZoneName: aws.String("")})
	}
	return out
}

func (s Subnets) String() string {
	var out string
	for _, sub := range s {
		out += *sub
	}
	return out
}

func Difference(a, b AWSStringSlice) (ab AWSStringSlice) {
	mb := map[string]bool{}
	for _, x := range b {
		mb[*x] = true
	}
	for _, x := range a {
		if _, ok := mb[*x]; !ok {
			ab = append(ab, x)
		}
	}
	return
}
