package tags

import (
	"context"
	"fmt"
	"strings"

	api "k8s.io/api/core/v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/albctx"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

const (
	ServiceName = "kubernetes.io/service-name"
	ServicePort = "kubernetes.io/service-port"
	Namespace   = "kubernetes.io/namespace"
	IngressName = "kubernetes.io/ingress-name"
)

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

func (t *Tags) Copy() *Tags {
	n := NewTags()
	for k, v := range t.Tags {
		n.Tags[k] = v
	}
	return n
}

// TagsController manages tags on a resource
type TagsController interface {
	Reconcile(context.Context, *Tags) error
}

// NewTagsController constructs a new tags controller
func NewTagsController(ec2 ec2iface.EC2API, elbv2 elbv2iface.ELBV2API) TagsController {
	return &tagsController{
		ec2:   ec2,
		elbv2: elbv2,
	}
}

type tagsController struct {
	ec2   ec2iface.EC2API
	elbv2 elbv2iface.ELBV2API
}

func (c *tagsController) Reconcile(ctx context.Context, desired *Tags) error {
	if strings.HasPrefix(desired.Arn, "arn:aws:elasticloadbalancing") {
		return c.reconcileElasticloadbalancing(ctx, desired)
	}
	return fmt.Errorf("%v tags not implemented", desired.Arn)
}

func (c *tagsController) reconcileElasticloadbalancing(ctx context.Context, desired *Tags) error {
	current := NewTags()

	resp, err := c.elbv2.DescribeTags(&elbv2.DescribeTagsInput{ResourceArns: []*string{aws.String(desired.Arn)}})
	if err != nil {
		return err
	}

	for _, tagDescription := range resp.TagDescriptions {
		if aws.StringValue(tagDescription.ResourceArn) != desired.Arn {
			continue
		}
		for _, tag := range tagDescription.Tags {
			current.Tags[aws.StringValue(tag.Key)] = aws.StringValue(tag.Value)
		}
	}

	modify, remove := tagsChangeSet(current, desired)

	if len(modify) > 0 {
		albctx.GetLogger(ctx).Infof("Modifying tags on %v to %v", desired.Arn, log.Prettify(modify))

		addParams := &elbv2.AddTagsInput{
			ResourceArns: []*string{aws.String(desired.Arn)},
			Tags:         mapToELBV2(modify),
		}
		if _, err := c.elbv2.AddTags(addParams); err != nil {
			if eventf, ok := albctx.GetEventf(ctx); ok {
				eventf(api.EventTypeWarning, "ERROR", "Error tagging %s: %s", desired.Arn, err.Error())
			}
			return err
		}
	}

	if len(remove) > 0 {
		albctx.GetLogger(ctx).Infof("Removing %v tags from %v", strings.Join(remove, ", "), desired.Arn)

		removeParams := &elbv2.RemoveTagsInput{
			ResourceArns: []*string{aws.String(desired.Arn)},
			TagKeys:      aws.StringSlice(remove),
		}

		if _, err := c.elbv2.RemoveTags(removeParams); err != nil {
			if eventf, ok := albctx.GetEventf(ctx); ok {
				eventf(api.EventTypeWarning, "ERROR", "Error tagging %s: %s", desired.Arn, err.Error())
			}
			return err
		}
	}

	return nil
}

// tagsChangeSet compares b to a, returning a map of tags to add/change to a and a list of tags to remove from a
func tagsChangeSet(a, b *Tags) (map[string]string, []string) {
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

func (t *Tags) AsELBV2() []*elbv2.Tag {
	return mapToELBV2(t.Tags)
}

func mapToELBV2(m map[string]string) (o []*elbv2.Tag) {
	for k, v := range m {
		o = append(o, &elbv2.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return o
}
