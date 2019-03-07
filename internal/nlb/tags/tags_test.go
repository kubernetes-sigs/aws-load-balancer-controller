package tags

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	"github.com/stretchr/testify/assert"
)

func Test_tagsChangeSet(t *testing.T) {
	emptyChangeSet := make(map[string]string)
	for _, tc := range []struct {
		name      string
		a         map[string]string
		b         map[string]string
		changeSet map[string]string
		removeSet map[string]string
	}{
		{
			name:      "empty a, b",
			a:         nil,
			b:         nil,
			changeSet: emptyChangeSet,
			removeSet: emptyChangeSet,
		},
		{
			name:      "empty a, b adds a key to a",
			a:         nil,
			b:         map[string]string{"k": "v"},
			changeSet: map[string]string{"k": "v"},
			removeSet: emptyChangeSet,
		},
		{
			name:      "a, b changes a key in a",
			a:         map[string]string{"k": "v"},
			b:         map[string]string{"k": "v2"},
			changeSet: map[string]string{"k": "v2"},
			removeSet: emptyChangeSet,
		},
		{
			name:      "a, b removes a key in a",
			a:         map[string]string{"k": "v"},
			b:         nil,
			changeSet: emptyChangeSet,
			removeSet: map[string]string{"k": "v"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changeSet, removeSet := changeSets(tc.a, tc.b)
			assert.Equal(t, tc.changeSet, changeSet, "add/modify list not as expected")
			assert.Equal(t, tc.removeSet, removeSet, "remove list not as expected")
		})
	}
}

func Test_ConvertToELBV2(t *testing.T) {
	source := map[string]string{"key": "val"}
	expected := []*elbv2.Tag{
		{Key: aws.String("key"), Value: aws.String("val")},
	}
	assert.Equal(t, ConvertToELBV2(source), expected)
}

func Test_ConvertToEC2(t *testing.T) {
	source := map[string]string{"key": "val"}
	expected := []*ec2.Tag{
		{Key: aws.String("key"), Value: aws.String("val")},
	}
	assert.Equal(t, ConvertToEC2(source), expected)
}

type DescribeELBV2TagsWithContextCall struct {
	Output *elbv2.DescribeTagsOutput
	Err    error
}

type AddELBV2TagsWithContextCall struct {
	Input *elbv2.AddTagsInput
	Err   error
}

type RemoveELBV2TagsWithContextCall struct {
	Input *elbv2.RemoveTagsInput
	Err   error
}

func elbv2Tag(k, v string) *elbv2.Tag {
	return &elbv2.Tag{Key: aws.String(k), Value: aws.String(v)}
}

func Test_ReconcileELB(t *testing.T) {
	arn := "arn:aws:elasticloadbalancing:us-east-1:111111111111:targetgroup/bee29091-73cab466d431a284f6f/65fc536333193179"
	for _, tc := range []struct {
		Name                             string
		DesiredTags                      map[string]string
		DescribeELBV2TagsWithContextCall *DescribeELBV2TagsWithContextCall
		AddELBV2TagsWithContextCall      *AddELBV2TagsWithContextCall
		RemoveELBV2TagsWithContextCall   *RemoveELBV2TagsWithContextCall
		ExpectedError                    error
	}{
		{
			Name:        "empty current, empty desired",
			DesiredTags: nil,
			DescribeELBV2TagsWithContextCall: &DescribeELBV2TagsWithContextCall{
				Output: &elbv2.DescribeTagsOutput{},
			},
		},
		{
			Name:        "add a tag",
			DesiredTags: map[string]string{"k": "v"},
			DescribeELBV2TagsWithContextCall: &DescribeELBV2TagsWithContextCall{
				Output: &elbv2.DescribeTagsOutput{},
			},
			AddELBV2TagsWithContextCall: &AddELBV2TagsWithContextCall{
				Input: &elbv2.AddTagsInput{
					ResourceArns: []*string{aws.String(arn)},
					Tags: []*elbv2.Tag{
						elbv2Tag("k", "v"),
					},
				},
			},
		},
		{
			Name:        "modify a tag",
			DesiredTags: map[string]string{"k": "new"},
			DescribeELBV2TagsWithContextCall: &DescribeELBV2TagsWithContextCall{
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
			AddELBV2TagsWithContextCall: &AddELBV2TagsWithContextCall{
				Input: &elbv2.AddTagsInput{
					ResourceArns: []*string{aws.String(arn)},
					Tags: []*elbv2.Tag{
						elbv2Tag("k", "new"),
					},
				},
			},
		},
		{
			Name:        "remove a tag",
			DesiredTags: nil,
			DescribeELBV2TagsWithContextCall: &DescribeELBV2TagsWithContextCall{
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
			RemoveELBV2TagsWithContextCall: &RemoveELBV2TagsWithContextCall{
				Input: &elbv2.RemoveTagsInput{
					ResourceArns: []*string{aws.String(arn)},
					TagKeys:      []*string{aws.String("k")},
				},
			},
		},
		{
			Name:        "describe error",
			DesiredTags: nil,
			DescribeELBV2TagsWithContextCall: &DescribeELBV2TagsWithContextCall{
				Err: fmt.Errorf("nope"),
			},
			ExpectedError: fmt.Errorf("nope"),
		},
		{
			Name:        "add error",
			DesiredTags: map[string]string{"k": "v"},
			DescribeELBV2TagsWithContextCall: &DescribeELBV2TagsWithContextCall{
				Output: &elbv2.DescribeTagsOutput{},
			},
			AddELBV2TagsWithContextCall: &AddELBV2TagsWithContextCall{
				Input: &elbv2.AddTagsInput{
					ResourceArns: []*string{aws.String(arn)},
					Tags: []*elbv2.Tag{
						elbv2Tag("k", "v"),
					},
				},
				Err: fmt.Errorf("nope"),
			},
			ExpectedError: fmt.Errorf("nope"),
		},
		{
			Name:        "remove error",
			DesiredTags: nil,
			DescribeELBV2TagsWithContextCall: &DescribeELBV2TagsWithContextCall{
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
			RemoveELBV2TagsWithContextCall: &RemoveELBV2TagsWithContextCall{
				Input: &elbv2.RemoveTagsInput{
					ResourceArns: []*string{aws.String(arn)},
					TagKeys:      []*string{aws.String("k")},
				},
				Err: fmt.Errorf("nope"),
			},
			ExpectedError: fmt.Errorf("nope"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			cloud := &mocks.CloudAPI{}

			if tc.DescribeELBV2TagsWithContextCall != nil {
				cloud.On("DescribeELBV2TagsWithContext", ctx, &elbv2.DescribeTagsInput{ResourceArns: []*string{aws.String(arn)}}).Return(tc.DescribeELBV2TagsWithContextCall.Output, tc.DescribeELBV2TagsWithContextCall.Err)
			}

			if tc.AddELBV2TagsWithContextCall != nil {
				cloud.On("AddELBV2TagsWithContext", ctx, tc.AddELBV2TagsWithContextCall.Input).Return(nil, tc.AddELBV2TagsWithContextCall.Err)
			}

			if tc.RemoveELBV2TagsWithContextCall != nil {
				cloud.On("RemoveELBV2TagsWithContext", ctx, tc.RemoveELBV2TagsWithContextCall.Input).Return(nil, tc.RemoveELBV2TagsWithContextCall.Err)
			}

			controller := NewController(cloud)
			err := controller.ReconcileELB(context.Background(), arn, tc.DesiredTags)
			if tc.ExpectedError != nil {
				assert.Equal(t, tc.ExpectedError, err)
			} else {
				assert.NoError(t, err)
			}
			cloud.AssertExpectations(t)
			cloud.AssertExpectations(t)
			cloud.AssertExpectations(t)
		})
	}
}

type CreateEC2TagsWithContextCall struct {
	Input *ec2.CreateTagsInput
	Err   error
}

type DeleteEC2TagsWithContextCall struct {
	Input *ec2.DeleteTagsInput
	Err   error
}

func ec2Tag(k, v string) *ec2.Tag {
	return &ec2.Tag{Key: aws.String(k), Value: aws.String(v)}
}

func Test_ReconcileEC2WithCurTags(t *testing.T) {
	resourceID := "sg-4242424242"
	for _, tc := range []struct {
		Name                         string
		DesiredTags                  map[string]string
		CurrentTags                  map[string]string
		CreateEC2TagsWithContextCall *CreateEC2TagsWithContextCall
		DeleteEC2TagsWithContextCall *DeleteEC2TagsWithContextCall
		ExpectedErr                  error
	}{
		{
			Name:        "empty desired & current",
			DesiredTags: nil,
			CurrentTags: nil,
		},
		{
			Name:        "add an tag",
			DesiredTags: map[string]string{"k": "v"},
			CurrentTags: nil,
			CreateEC2TagsWithContextCall: &CreateEC2TagsWithContextCall{
				Input: &ec2.CreateTagsInput{
					Resources: []*string{aws.String(resourceID)},
					Tags: []*ec2.Tag{
						ec2Tag("k", "v"),
					},
				},
			},
		},
		{
			Name:        "modify an tag",
			DesiredTags: map[string]string{"k": "new"},
			CurrentTags: map[string]string{"k": "v"},
			CreateEC2TagsWithContextCall: &CreateEC2TagsWithContextCall{
				Input: &ec2.CreateTagsInput{
					Resources: []*string{aws.String(resourceID)},
					Tags: []*ec2.Tag{
						ec2Tag("k", "new"),
					},
				},
			},
		},
		{
			Name:        "remove an tag",
			DesiredTags: nil,
			CurrentTags: map[string]string{"k": "v"},
			DeleteEC2TagsWithContextCall: &DeleteEC2TagsWithContextCall{
				Input: &ec2.DeleteTagsInput{
					Resources: []*string{aws.String(resourceID)},
					Tags: []*ec2.Tag{
						ec2Tag("k", "v"),
					},
				},
			},
		},
		{
			Name:        "error when modify an tag",
			DesiredTags: map[string]string{"k": "new"},
			CurrentTags: map[string]string{"k": "v"},
			CreateEC2TagsWithContextCall: &CreateEC2TagsWithContextCall{
				Input: &ec2.CreateTagsInput{
					Resources: []*string{aws.String(resourceID)},
					Tags: []*ec2.Tag{
						ec2Tag("k", "new"),
					},
				},
				Err: errors.New("CreateEC2TagsWithContextCall"),
			},
			ExpectedErr: errors.New("CreateEC2TagsWithContextCall"),
		},
		{
			Name:        "error when remove an tag",
			DesiredTags: nil,
			CurrentTags: map[string]string{"k": "v"},
			DeleteEC2TagsWithContextCall: &DeleteEC2TagsWithContextCall{
				Input: &ec2.DeleteTagsInput{
					Resources: []*string{aws.String(resourceID)},
					Tags: []*ec2.Tag{
						ec2Tag("k", "v"),
					},
				},
				Err: errors.New("DeleteEC2TagsWithContextCall"),
			},
			ExpectedErr: errors.New("DeleteEC2TagsWithContextCall"),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			cloud := &mocks.CloudAPI{}
			if tc.CreateEC2TagsWithContextCall != nil {
				cloud.On("CreateEC2TagsWithContext", ctx, tc.CreateEC2TagsWithContextCall.Input).Return(nil, tc.CreateEC2TagsWithContextCall.Err)
			}
			if tc.DeleteEC2TagsWithContextCall != nil {
				cloud.On("DeleteEC2TagsWithContext", ctx, tc.DeleteEC2TagsWithContextCall.Input).Return(nil, tc.DeleteEC2TagsWithContextCall.Err)
			}
			controller := NewController(cloud)
			err := controller.ReconcileEC2WithCurTags(ctx, resourceID, tc.DesiredTags, tc.CurrentTags)
			assert.Equal(t, err, tc.ExpectedErr)
			cloud.AssertExpectations(t)
		})
	}
}
