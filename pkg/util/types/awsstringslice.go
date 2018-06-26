package types

import (
	"crypto/md5"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
)

type AWSStringSlice []*string
type Subnets AWSStringSlice
type Cidrs AWSStringSlice

func (n AWSStringSlice) Len() int           { return len(n) }
func (n AWSStringSlice) Less(i, j int) bool { return *n[i] < *n[j] }
func (n AWSStringSlice) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }

// NewAWSStringSlice converts a string with comma separated strings into an AWSStringSlice.
func NewAWSStringSlice(s string) (out AWSStringSlice) {
	parts := strings.Split(s, ",")
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		out = append(out, aws.String(p))
	}
	return out
}

// Hash returns a hash representing security group names
func (a AWSStringSlice) Hash() string {
	sort.Sort(a)
	hasher := md5.New()
	for _, str := range a {
		hasher.Write([]byte(*str))
	}
	output := hex.EncodeToString(hasher.Sum(nil))
	return output
}
