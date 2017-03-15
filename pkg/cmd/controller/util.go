package controller

import (
	"crypto/md5"
	"encoding/hex"
	"sort"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/karlseguin/ccache"
)

var cache = ccache.New(ccache.Configure())

type AwsStringSlice []*string

func (n AwsStringSlice) Len() int           { return len(n) }
func (n AwsStringSlice) Less(i, j int) bool { return *n[i] < *n[j] }
func (n AwsStringSlice) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }

// GetNodes returns a list of the cluster node external ids
func GetNodes(ac *ALBController) AwsStringSlice {
	var result AwsStringSlice
	nodes, _ := ac.storeLister.Node.List()
	for _, node := range nodes.Items {
		result = append(result, aws.String(node.Spec.ExternalID))
	}
	sort.Sort(result)
	return result
}

func (a AwsStringSlice) Hash() *string {
	sort.Sort(a)
	hasher := md5.New()
	for _, str := range a {
		hasher.Write([]byte(*str))
	}
	output := hex.EncodeToString(hasher.Sum(nil))
	return aws.String(output)
}
