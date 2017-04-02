package controller

// GetNodes returns a list of the cluster node external ids
func GetNodes(ac *ALBController) awsutil.AWSStringSlice {
	var result AWSStringSlice
	nodes, _ := ac.storeLister.Node.List()
	for _, node := range nodes.Items {
		result = append(result, aws.String(node.Spec.ExternalID))
	}
	sort.Sort(result)
	return result
}


