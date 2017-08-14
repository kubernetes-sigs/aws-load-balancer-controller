package controller

import "github.com/aws/aws-sdk-go/aws"

var a *ALBIngress

func setup() {
	//setupEC2()
	//setupELBV2()
	//setupRoute53()

	a = &ALBIngress{
		id:          aws.String("clustername-ingressname"),
		namespace:   aws.String("namespace"),
		clusterName: aws.String("clustername"),
		ingressName: aws.String("ingressname"),
		// annotations: annotations,
		// nodes:       GetNodes(ac),
	}

}
