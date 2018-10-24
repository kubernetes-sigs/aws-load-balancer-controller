package tags

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi/resourcegroupstaggingapiiface"
	api "k8s.io/api/core/v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

// Standard tag key names
const (
	IngressName = "kubernetes.io/ingress-name"
	Namespace   = "kubernetes.io/namespace"
	ServiceName = "kubernetes.io/service-name"
	ServicePort = "kubernetes.io/service-port"
)

// Tags stores the tags for an ARN
type Tags struct {
	// Arn is the ARN of the resource to be tagged
	Arn string

	// Tags is a map[key]value of tags for the resource
	Tags map[string]string
}

// NewTags allocates an empty Tags struct
func NewTags(m ...map[string]string) *Tags {
	t := &Tags{Tags: make(map[string]string)}
	for i := range m {
		for k, v := range m[i] {
			t.Tags[k] = v
		}
	}
	return t
}

// Copy returns a copy of t
func (t *Tags) Copy() *Tags {
	return NewTags(t.Tags)
}

// Controller manages tags on a resource
type Controller interface {
	Reconcile(context.Context, *Tags) error
}

// NewController constructs a new tags controller
func NewController(ec2 ec2iface.EC2API, elbv2 elbv2iface.ELBV2API, rgt resourcegroupstaggingapiiface.ResourceGroupsTaggingAPIAPI) Controller {
	return &controller{
		ec2:   ec2,
		elbv2: elbv2,
		rgt:   rgt,
	}
}

type controller struct {
	ec2   ec2iface.EC2API
	elbv2 elbv2iface.ELBV2API
	rgt   resourcegroupstaggingapiiface.ResourceGroupsTaggingAPIAPI
}

func (c *controller) Reconcile(ctx context.Context, desired *Tags) error {
	var current *Tags
	var err error

	if strings.HasPrefix(desired.Arn, "arn:aws:elasticloadbalancing") {
		if current, err = c.elbTags(ctx, desired.Arn); err != nil {
			return err
		}
	}

	modify, remove := changeSets(current, desired)

	if len(modify) > 0 {
		albctx.GetLogger(ctx).Infof("Modifying tags on %v to %v", desired.Arn, log.Prettify(modify))

		p := &resourcegroupstaggingapi.TagResourcesInput{
			ResourceARNList: []*string{aws.String(desired.Arn)},
			Tags:            aws.StringMap(modify),
		}
		if _, err := c.rgt.TagResources(p); err != nil {
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", "Error tagging %s: %s", desired.Arn, err.Error())
			return err
		}
	}

	if len(remove) > 0 {
		albctx.GetLogger(ctx).Infof("Removing %v tags from %v", strings.Join(remove, ", "), desired.Arn)

		p := &resourcegroupstaggingapi.UntagResourcesInput{
			ResourceARNList: []*string{aws.String(desired.Arn)},
			TagKeys:         aws.StringSlice(remove),
		}
		if _, err := c.rgt.UntagResources(p); err != nil {
			albctx.GetEventf(ctx)(api.EventTypeWarning, "ERROR", "Error tagging %s: %s", desired.Arn, err.Error())
			return err
		}
	}

	return nil
}

func (c *controller) elbTags(ctx context.Context, arn string) (t *Tags, err error) {
	var r *elbv2.DescribeTagsOutput
	t = NewTags()

	if r, err = c.elbv2.DescribeTags(&elbv2.DescribeTagsInput{ResourceArns: []*string{aws.String(arn)}}); err == nil {
		for _, tagDescription := range r.TagDescriptions {
			for _, tag := range tagDescription.Tags {
				t.Tags[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
			}
		}
	}

	return
}

// changeSets compares b to a, returning a map of tags to add/change to a and a list of tags to remove from a
func changeSets(a, b *Tags) (map[string]string, []string) {
	modify := make(map[string]string)
	var remove []string

	for k, v := range b.Tags {
		if a.Tags[k] != v {
			modify[k] = v
		}
	}
	for k := range a.Tags {
		if _, ok := b.Tags[k]; !ok {
			remove = append(remove, k)
		}
	}

	return modify, remove
}

// AsELBV2 returns a []*elbv2.Tag copy of tags
func (t *Tags) AsELBV2() (output []*elbv2.Tag) {
	for k, v := range t.Tags {
		output = append(output, &elbv2.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return
}

// ConvertToELBV2 will convert tags to ELBV2 Tags
func ConvertToELBV2(tags map[string]string) ([]*elbv2.Tag) {
	var output []*elbv2.Tag
	for k, v := range tags {
		output = append(output, &elbv2.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return output
}
