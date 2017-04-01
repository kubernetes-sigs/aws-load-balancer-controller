package testing

import "github.com/aws/aws-sdk-go/aws"

var a *albIngress

func setup() {
	setupEC2()
	setupELBV2()
	setupRoute53()

	a = &albIngress{
		id:          aws.String("clustername-ingressname"),
		namespace:   aws.String("namespace"),
		clusterName: aws.String("clustername"),
		ingressName: aws.String("ingressname"),
		// annotations: annotations,
		// nodes:       GetNodes(ac),
	}

}
