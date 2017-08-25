package albingress

var a *ALBIngress

func setup() {
	//setupEC2()
	//setupELBV2()

	a = &ALBIngress{
		ID:          "clustername-ingressname",
		namespace:   "namespace",
		clusterName: "clustername",
		ingressName: "ingressname",
		// annotations: annotations,
		// nodes:       GetNodes(ac),
	}

}
