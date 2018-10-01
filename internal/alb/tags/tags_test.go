package tags

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
)

func Test_TagsCopy(t *testing.T) {
	source := NewTags()
	source.Tags["k"] = "v"

	copy := source.Copy()

	assert.Equal(t, source, copy)
}

func Test_tagsChangeSet(t *testing.T) {
	emptyChangeSet := make(map[string]string)
	for _, tc := range []struct {
		name      string
		a         *Tags
		b         *Tags
		changeSet map[string]string
		removeSet []string
	}{
		{
			name:      "empty a, b",
			a:         NewTags(),
			b:         NewTags(),
			changeSet: emptyChangeSet,
		},
		{
			name:      "empty a, b adds a key to a",
			a:         NewTags(),
			b:         NewTags(map[string]string{"k": "v"}),
			changeSet: map[string]string{"k": "v"},
		},
		{
			name:      "a, b changes a key in a",
			a:         NewTags(map[string]string{"k": "v"}),
			b:         NewTags(map[string]string{"k": "v2"}),
			changeSet: map[string]string{"k": "v2"},
		},
		{
			name:      "a, b removes a key in a",
			a:         NewTags(map[string]string{"k": "v"}),
			b:         NewTags(),
			changeSet: emptyChangeSet,
			removeSet: []string{"k"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changeSet, removeSet := changeSets(tc.a, tc.b)
			assert.Equal(t, tc.changeSet, changeSet, "add/modify list not as expected")
			assert.Equal(t, tc.removeSet, removeSet, "remove list not as expected")
		})
	}
}

func Test_AsELBV2(t *testing.T) {
	source := NewTags(map[string]string{"key": "val"})
	expected := []*elbv2.Tag{
		&elbv2.Tag{Key: aws.String("key"), Value: aws.String("val")},
	}
	assert.Equal(t, source.AsELBV2(), expected)
}

type DescribeTagsELBV2Call struct {
	Output *elbv2.DescribeTagsOutput
	Err    error
}

type TagResourcesCall struct {
	Input *resourcegroupstaggingapi.TagResourcesInput
	Err   error
}

type UntagResourcesCall struct {
	Input *resourcegroupstaggingapi.UntagResourcesInput
	Err   error
}

func elbv2Tag(k, v string) *elbv2.Tag {
	return &elbv2.Tag{Key: aws.String(k), Value: aws.String(v)}
}

func Test_Reconcile(t *testing.T) {
	arn := "arn:aws:elasticloadbalancing:us-east-1:111111111111:targetgroup/bee29091-73cab466d431a284f6f/65fc536333193179"

	emptyTags := func() *Tags {
		emptyTags := NewTags()
		emptyTags.Arn = arn
		return emptyTags
	}

	for _, tc := range []struct {
		name                  string
		Tags                  *Tags
		DescribeTagsELBV2Call *DescribeTagsELBV2Call
		TagResourcesCall      *TagResourcesCall
		UntagResourcesCall    *UntagResourcesCall
		ExpectedError         error
	}{
		{
			name:                  "empty current, empty desired",
			Tags:                  emptyTags(),
			DescribeTagsELBV2Call: &DescribeTagsELBV2Call{Output: &elbv2.DescribeTagsOutput{}},
		},
		{
			name: "add a tag",
			Tags: func() *Tags { t := emptyTags(); t.Tags["k"] = "v"; return t }(),
			DescribeTagsELBV2Call: &DescribeTagsELBV2Call{
				Output: &elbv2.DescribeTagsOutput{},
			},
			TagResourcesCall: &TagResourcesCall{
				Input: &resourcegroupstaggingapi.TagResourcesInput{
					ResourceARNList: []*string{aws.String(arn)},
					Tags:            map[string]*string{"k": aws.String("v")},
				},
			},
		},
		{
			name: "modify a tag",
			Tags: func() *Tags { t := emptyTags(); t.Tags["k"] = "new"; return t }(),
			DescribeTagsELBV2Call: &DescribeTagsELBV2Call{
				Output: &elbv2.DescribeTagsOutput{
					TagDescriptions: []*elbv2.TagDescription{
						{
							ResourceArn: aws.String(arn),
							Tags: []*elbv2.Tag{
								elbv2Tag("k", "v"),
							},
						},
					},
				},
			},
			TagResourcesCall: &TagResourcesCall{
				Input: &resourcegroupstaggingapi.TagResourcesInput{
					ResourceARNList: []*string{aws.String(arn)},
					Tags:            map[string]*string{"k": aws.String("new")},
				},
			},
		},
		{
			name: "remove a tag",
			Tags: emptyTags(),
			DescribeTagsELBV2Call: &DescribeTagsELBV2Call{
				Output: &elbv2.DescribeTagsOutput{
					TagDescriptions: []*elbv2.TagDescription{
						{
							ResourceArn: aws.String(arn),
							Tags: []*elbv2.Tag{
								elbv2Tag("k", "v"),
							},
						},
					},
				},
			},
			UntagResourcesCall: &UntagResourcesCall{
				Input: &resourcegroupstaggingapi.UntagResourcesInput{
					ResourceARNList: []*string{aws.String(arn)},
					TagKeys:         []*string{aws.String("k")},
				},
			},
		},
		{
			name: "describe error",
			Tags: emptyTags(),
			DescribeTagsELBV2Call: &DescribeTagsELBV2Call{
				Err: fmt.Errorf("nope"),
			},
			ExpectedError: fmt.Errorf("nope"),
		},
		{
			name:                  "add error",
			Tags:                  func() *Tags { t := emptyTags(); t.Tags["k"] = "v"; return t }(),
			DescribeTagsELBV2Call: &DescribeTagsELBV2Call{Output: &elbv2.DescribeTagsOutput{}},
			TagResourcesCall: &TagResourcesCall{
				Input: &resourcegroupstaggingapi.TagResourcesInput{
					ResourceARNList: []*string{aws.String(arn)},
					Tags:            map[string]*string{"k": aws.String("v")},
				},
				Err: fmt.Errorf("nope"),
			},
			ExpectedError: fmt.Errorf("nope"),
		},
		{
			name: "remove error",
			Tags: emptyTags(),
			DescribeTagsELBV2Call: &DescribeTagsELBV2Call{
				Output: &elbv2.DescribeTagsOutput{
					TagDescriptions: []*elbv2.TagDescription{
						{
							ResourceArn: aws.String(arn),
							Tags: []*elbv2.Tag{
								elbv2Tag("k", "v"),
							},
						},
					},
				},
			},
			UntagResourcesCall: &UntagResourcesCall{
				Input: &resourcegroupstaggingapi.UntagResourcesInput{
					ResourceARNList: []*string{aws.String(arn)},
					TagKeys:         []*string{aws.String("k")},
				},
				Err: fmt.Errorf("nope"),
			},
			ExpectedError: fmt.Errorf("nope"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ec2svc := &mocks.EC2API{}
			elbv2svc := &mocks.ELBV2API{}
			rgtsvc := &mocks.ResourceGroupsTaggingAPIAPI{}

			if tc.DescribeTagsELBV2Call != nil {
				elbv2svc.On("DescribeTags", &elbv2.DescribeTagsInput{ResourceArns: []*string{aws.String(tc.Tags.Arn)}}).Return(tc.DescribeTagsELBV2Call.Output, tc.DescribeTagsELBV2Call.Err)
			}

			if tc.TagResourcesCall != nil {
				rgtsvc.On("TagResources", tc.TagResourcesCall.Input).Return(nil, tc.TagResourcesCall.Err)
			}

			if tc.UntagResourcesCall != nil {
				rgtsvc.On("UntagResources", tc.UntagResourcesCall.Input).Return(nil, tc.UntagResourcesCall.Err)
			}

			controller := NewController(ec2svc, elbv2svc, rgtsvc)
			err := controller.Reconcile(context.Background(), tc.Tags)

			if tc.ExpectedError != nil {
				assert.Equal(t, tc.ExpectedError, err)
			} else {
				assert.NoError(t, err)
			}
			elbv2svc.AssertExpectations(t)
			ec2svc.AssertExpectations(t)
			rgtsvc.AssertExpectations(t)
		})
	}
}
