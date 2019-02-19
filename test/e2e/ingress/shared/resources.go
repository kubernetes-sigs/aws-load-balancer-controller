package shared

import (
	"context"
	"fmt"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/alb/generator"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/test/e2e/framework/utils"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"
)

type AWSResources struct {
	ALBs           []string
	TargetGroups   []string
	SecurityGroups []string
}

func GetAWSResourcesByIngress(cloud aws.CloudAPI, clusterName string, namespace string, ingressName string) (AWSResources, error) {
	ingResFilter := map[string][]string{
		fmt.Sprintf("kubernetes.io/cluster/%s", clusterName): {"owned", "shared"},
		generator.TagKeyNamespace:                            {namespace},
		generator.TagKeyIngressName:                          {ingressName},
	}
	sgResFilter := map[string][]string{
		generator.TagKeyClusterName: {clusterName},
		generator.TagKeyNamespace:   {namespace},
		generator.TagKeyIngressName: {ingressName},
	}

	albs, err := cloud.GetResourcesByFilters(ingResFilter, aws.ResourceTypeEnumELBLoadBalancer)
	if err != nil {
		return AWSResources{}, err
	}
	tgs, err := cloud.GetResourcesByFilters(ingResFilter, aws.ResourceTypeEnumELBTargetGroup)
	if err != nil {
		return AWSResources{}, err
	}
	sgs, err := cloud.GetResourcesByFilters(sgResFilter, aws.ResourceTypeEnumEC2SecurityGroup)
	if err != nil {
		return AWSResources{}, err
	}
	return AWSResources{
		ALBs:           albs,
		TargetGroups:   tgs,
		SecurityGroups: sgs,
	}, nil
}

func ExpectAWSResourcedByIngressEventuallyDeleted(ctx context.Context, cloud aws.CloudAPI, clusterName string, namespace string, ingressName string) {
	err := wait.PollImmediateUntil(utils.PollIntervalMedium, func() (bool, error) {
		awsRes, err := GetAWSResourcesByIngress(cloud, clusterName, namespace, ingressName)
		if err != nil {
			return false, err
		}
		allAWSResDeleted := true
		if len(awsRes.ALBs) != 0 {
			for _, alb := range awsRes.ALBs {
				utils.Logf("ALB %s is still exists", alb)
			}
			allAWSResDeleted = false
		}
		if len(awsRes.TargetGroups) != 0 {
			for _, tg := range awsRes.TargetGroups {
				utils.Logf("TargetGroup %s is still exists", tg)
			}
			allAWSResDeleted = false
		}
		if len(awsRes.SecurityGroups) != 0 {
			for _, sg := range awsRes.SecurityGroups {
				utils.Logf("SecurityGroup %s is still exists", sg)
			}
			allAWSResDeleted = false
		}
		if !allAWSResDeleted {
			return false, nil
		}
		return true, nil
	}, ctx.Done())
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("all aws resources for %s/%s should be cleaned up", namespace, ingressName))
}
