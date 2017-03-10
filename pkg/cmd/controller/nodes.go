package controller

import (
	"sort"

	"github.com/aws/aws-sdk-go/aws"
)

type NodeSlice []*string

func (n NodeSlice) Len() int           { return len(n) }
func (n NodeSlice) Less(i, j int) bool { return *n[i] < *n[j] }
func (n NodeSlice) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }

// GetNodes returns a list of the cluster node external ids
func GetNodes(ac *ALBController) NodeSlice {
	var result NodeSlice
	nodes, _ := ac.storeLister.Node.List()
	for _, node := range nodes.Items {
		result = append(result, aws.String(node.Spec.ExternalID))
	}
	sort.Sort(result)
	return result
}
