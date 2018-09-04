package tg

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/aws/albrgt"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

func DummyTG(arn, svcname string) *TargetGroup {
	albrgt.RGTsvc = &albrgt.Dummy{}
	albrgt.RGTsvc.SetResponse(&albrgt.Resources{
		TargetGroups: map[string]util.ELBv2Tags{arn: util.ELBv2Tags{
			&elbv2.Tag{
				Key:   aws.String("kubernetes.io/service-name"),
				Value: aws.String(svcname),
			},
			&elbv2.Tag{
				Key:   aws.String("kubernetes.io/service-port"),
				Value: aws.String("8080"),
			},
		}}}, nil)

	albelbv2.ELBV2svc.SetField("DescribeTargetHealthOutput", &elbv2.DescribeTargetHealthOutput{})

	t, err := NewCurrentTargetGroup(&NewCurrentTargetGroupOptions{
		Logger:         log.New("test"),
		LoadBalancerID: "nnnnn",
		TargetGroup: &elbv2.TargetGroup{
			TargetGroupName: aws.String("name"),
			TargetGroupArn:  aws.String(arn),
			Port:            aws.Int64(8080),
			Protocol:        aws.String(elbv2.ProtocolEnumHttp),
		},
		Metric: metric.DummyCollector{},
	})
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	CopyCurrentToDesired(t)
	return t
}
