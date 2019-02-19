package shared

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/test/e2e/framework/utils"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"
)

// TODO(@M00nF1sh): check the targets equals to targets in k8s(nodePort/podIP).
// ExpectTargetGroupTargetsEventuallyHealth checks the targets eventually become healthy :D.
func ExpectTargetGroupTargetsEventuallyHealth(ctx context.Context, cloud aws.CloudAPI, tgArn string) {
	err := wait.PollImmediateUntil(utils.PollIntervalMedium, func() (done bool, err error) {
		resp, err := cloud.DescribeTargetHealthWithContext(ctx, &elbv2.DescribeTargetHealthInput{
			TargetGroupArn: aws.String(tgArn),
		})
		if err != nil {
			return false, err
		}
		allTargetsAreHealthy := true
		for _, thd := range resp.TargetHealthDescriptions {
			if aws.StringValue(thd.TargetHealth.State) != elbv2.TargetHealthStateEnumHealthy {
				utils.Logf("target %v:%v is still %v", aws.StringValue(thd.Target.Id), aws.Int64Value(thd.Target.Port), aws.StringValue(thd.TargetHealth.State))
				allTargetsAreHealthy = false
			}
		}
		if !allTargetsAreHealthy {
			return false, nil
		}
		return true, nil
	}, ctx.Done())
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("all targets in %s should be healthy", tgArn))
}
