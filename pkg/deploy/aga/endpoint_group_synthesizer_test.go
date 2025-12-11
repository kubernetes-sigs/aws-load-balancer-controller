package aga

import (
	"context"
	"sort"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	agatypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

// Helper function to create a test endpoint group with port overrides
func createEndpointGroupWithPortOverrides(id string, region string, listenerARN string, portOverrides []agamodel.PortOverride) *agamodel.EndpointGroup {
	mockStack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})

	return &agamodel.EndpointGroup{
		ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::EndpointGroup", id),
		Spec: agamodel.EndpointGroupSpec{
			ListenerARN:           core.LiteralStringToken(listenerARN),
			Region:                region,
			TrafficDialPercentage: awssdk.Int32(100),
			PortOverrides:         portOverrides,
		},
	}
}

func Test_matchResAndSDKEndpointGroups(t *testing.T) {
	mockStack := core.NewDefaultStack(core.StackID{Namespace: "test-namespace", Name: "test-name"})
	testListenerARN := "arn:aws:globalaccelerator::123456789012:listener/1234abcd-abcd-1234-abcd-1234abcdefgh"

	tests := []struct {
		name              string
		resEndpointGroups []*agamodel.EndpointGroup
		sdkEndpointGroups []agatypes.EndpointGroup
		wantMatchedPairs  []struct {
			resID       string
			sdkRegion   string
			sdkGroupARN string
		}
		wantUnmatchedResIDs     []string
		wantUnmatchedSDKRegions []string
	}{
		{
			name:              "empty lists",
			resEndpointGroups: []*agamodel.EndpointGroup{},
			sdkEndpointGroups: []agatypes.EndpointGroup{},
			wantMatchedPairs: []struct {
				resID       string
				sdkRegion   string
				sdkGroupARN string
			}{},
			wantUnmatchedResIDs:     []string{},
			wantUnmatchedSDKRegions: []string{},
		},
		{
			name: "single exact match by region",
			resEndpointGroups: []*agamodel.EndpointGroup{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::EndpointGroup", "endpoint-group-1"),
					Spec: agamodel.EndpointGroupSpec{
						ListenerARN:           core.LiteralStringToken(testListenerARN),
						Region:                "us-west-2",
						TrafficDialPercentage: awssdk.Int32(100),
					},
				},
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				{
					EndpointGroupArn:      awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/1234abcd-abcd-1234-abcd-1234abcdefgh/eg-1234"),
					EndpointGroupRegion:   awssdk.String("us-west-2"),
					TrafficDialPercentage: awssdk.Float32(100.0),
				},
			},
			wantMatchedPairs: []struct {
				resID       string
				sdkRegion   string
				sdkGroupARN string
			}{
				{
					resID:       "endpoint-group-1",
					sdkRegion:   "us-west-2",
					sdkGroupARN: "arn:aws:globalaccelerator::123456789012:endpointgroup/1234abcd-abcd-1234-abcd-1234abcdefgh/eg-1234",
				},
			},
			wantUnmatchedResIDs:     []string{},
			wantUnmatchedSDKRegions: []string{},
		},
		{
			name: "multiple matches by region",
			resEndpointGroups: []*agamodel.EndpointGroup{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::EndpointGroup", "endpoint-group-1"),
					Spec: agamodel.EndpointGroupSpec{
						ListenerARN: core.LiteralStringToken(testListenerARN),
						Region:      "us-west-2",
					},
				},
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::EndpointGroup", "endpoint-group-2"),
					Spec: agamodel.EndpointGroupSpec{
						ListenerARN: core.LiteralStringToken(testListenerARN),
						Region:      "us-east-1",
					},
				},
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				{
					EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/1234abcd-abcd-1234-abcd-1234abcdefgh/eg-west"),
					EndpointGroupRegion: awssdk.String("us-west-2"),
				},
				{
					EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/1234abcd-abcd-1234-abcd-1234abcdefgh/eg-east"),
					EndpointGroupRegion: awssdk.String("us-east-1"),
				},
			},
			wantMatchedPairs: []struct {
				resID       string
				sdkRegion   string
				sdkGroupARN string
			}{
				{
					resID:       "endpoint-group-1",
					sdkRegion:   "us-west-2",
					sdkGroupARN: "arn:aws:globalaccelerator::123456789012:endpointgroup/1234abcd-abcd-1234-abcd-1234abcdefgh/eg-west",
				},
				{
					resID:       "endpoint-group-2",
					sdkRegion:   "us-east-1",
					sdkGroupARN: "arn:aws:globalaccelerator::123456789012:endpointgroup/1234abcd-abcd-1234-abcd-1234abcdefgh/eg-east",
				},
			},
			wantUnmatchedResIDs:     []string{},
			wantUnmatchedSDKRegions: []string{},
		},
		{
			name: "unmatched resource endpoint groups",
			resEndpointGroups: []*agamodel.EndpointGroup{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::EndpointGroup", "endpoint-group-1"),
					Spec: agamodel.EndpointGroupSpec{
						ListenerARN: core.LiteralStringToken(testListenerARN),
						Region:      "us-west-2",
					},
				},
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::EndpointGroup", "endpoint-group-2"),
					Spec: agamodel.EndpointGroupSpec{
						ListenerARN: core.LiteralStringToken(testListenerARN),
						Region:      "eu-west-1", // No matching SDK endpoint group
					},
				},
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				{
					EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/1234abcd-abcd-1234-abcd-1234abcdefgh/eg-west"),
					EndpointGroupRegion: awssdk.String("us-west-2"),
				},
			},
			wantMatchedPairs: []struct {
				resID       string
				sdkRegion   string
				sdkGroupARN string
			}{
				{
					resID:       "endpoint-group-1",
					sdkRegion:   "us-west-2",
					sdkGroupARN: "arn:aws:globalaccelerator::123456789012:endpointgroup/1234abcd-abcd-1234-abcd-1234abcdefgh/eg-west",
				},
			},
			wantUnmatchedResIDs:     []string{"endpoint-group-2"},
			wantUnmatchedSDKRegions: []string{},
		},
		{
			name: "unmatched SDK endpoint groups",
			resEndpointGroups: []*agamodel.EndpointGroup{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::EndpointGroup", "endpoint-group-1"),
					Spec: agamodel.EndpointGroupSpec{
						ListenerARN: core.LiteralStringToken(testListenerARN),
						Region:      "us-west-2",
					},
				},
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				{
					EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/1234abcd-abcd-1234-abcd-1234abcdefgh/eg-west"),
					EndpointGroupRegion: awssdk.String("us-west-2"),
				},
				{
					EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/1234abcd-abcd-1234-abcd-1234abcdefgh/eg-east"),
					EndpointGroupRegion: awssdk.String("us-east-1"), // No matching resource endpoint group
				},
			},
			wantMatchedPairs: []struct {
				resID       string
				sdkRegion   string
				sdkGroupARN string
			}{
				{
					resID:       "endpoint-group-1",
					sdkRegion:   "us-west-2",
					sdkGroupARN: "arn:aws:globalaccelerator::123456789012:endpointgroup/1234abcd-abcd-1234-abcd-1234abcdefgh/eg-west",
				},
			},
			wantUnmatchedResIDs:     []string{},
			wantUnmatchedSDKRegions: []string{"us-east-1"},
		},
		{
			name: "mixed matches and unmatches",
			resEndpointGroups: []*agamodel.EndpointGroup{
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::EndpointGroup", "endpoint-group-west"),
					Spec: agamodel.EndpointGroupSpec{
						ListenerARN: core.LiteralStringToken(testListenerARN),
						Region:      "us-west-2",
					},
				},
				{
					ResourceMeta: core.NewResourceMeta(mockStack, "AWS::GlobalAccelerator::EndpointGroup", "endpoint-group-central"),
					Spec: agamodel.EndpointGroupSpec{
						ListenerARN: core.LiteralStringToken(testListenerARN),
						Region:      "us-central-1", // No matching SDK endpoint group
					},
				},
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				{
					EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/1234abcd-abcd-1234-abcd-1234abcdefgh/eg-west"),
					EndpointGroupRegion: awssdk.String("us-west-2"),
				},
				{
					EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/1234abcd-abcd-1234-abcd-1234abcdefgh/eg-east"),
					EndpointGroupRegion: awssdk.String("us-east-1"), // No matching resource endpoint group
				},
			},
			wantMatchedPairs: []struct {
				resID       string
				sdkRegion   string
				sdkGroupARN string
			}{
				{
					resID:       "endpoint-group-west",
					sdkRegion:   "us-west-2",
					sdkGroupARN: "arn:aws:globalaccelerator::123456789012:endpointgroup/1234abcd-abcd-1234-abcd-1234abcdefgh/eg-west",
				},
			},
			wantUnmatchedResIDs:     []string{"endpoint-group-central"},
			wantUnmatchedSDKRegions: []string{"us-east-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run the function
			matchedPairs, unmatchedResEndpointGroups, unmatchedSDKEndpointGroups := matchResAndSDKEndpointGroups(tt.resEndpointGroups, tt.sdkEndpointGroups)

			// Collect the actual pairs and IDs for verification
			var actualMatchedPairs []struct {
				resID       string
				sdkRegion   string
				sdkGroupARN string
			}

			for _, pair := range matchedPairs {
				actualMatchedPairs = append(actualMatchedPairs, struct {
					resID       string
					sdkRegion   string
					sdkGroupARN string
				}{
					resID:       pair.resEndpointGroup.ID(),
					sdkRegion:   awssdk.ToString(pair.sdkEndpointGroup.EndpointGroupRegion),
					sdkGroupARN: awssdk.ToString(pair.sdkEndpointGroup.EndpointGroupArn),
				})
			}

			var actualUnmatchedResIDs []string
			for _, eg := range unmatchedResEndpointGroups {
				actualUnmatchedResIDs = append(actualUnmatchedResIDs, eg.ID())
			}

			var actualUnmatchedSDKRegions []string
			for _, eg := range unmatchedSDKEndpointGroups {
				actualUnmatchedSDKRegions = append(actualUnmatchedSDKRegions, awssdk.ToString(eg.EndpointGroupRegion))
			}

			// Sort all slices to ensure consistent comparison
			sort.Slice(actualMatchedPairs, func(i, j int) bool {
				if actualMatchedPairs[i].resID != actualMatchedPairs[j].resID {
					return actualMatchedPairs[i].resID < actualMatchedPairs[j].resID
				}
				return actualMatchedPairs[i].sdkGroupARN < actualMatchedPairs[j].sdkGroupARN
			})

			sort.Slice(tt.wantMatchedPairs, func(i, j int) bool {
				if tt.wantMatchedPairs[i].resID != tt.wantMatchedPairs[j].resID {
					return tt.wantMatchedPairs[i].resID < tt.wantMatchedPairs[j].resID
				}
				return tt.wantMatchedPairs[i].sdkGroupARN < tt.wantMatchedPairs[j].sdkGroupARN
			})

			sort.Strings(actualUnmatchedResIDs)
			sort.Strings(tt.wantUnmatchedResIDs)
			sort.Strings(actualUnmatchedSDKRegions)
			sort.Strings(tt.wantUnmatchedSDKRegions)

			// Verify matched pairs
			assert.Equal(t, len(tt.wantMatchedPairs), len(actualMatchedPairs), "matched pairs count")
			for i := range tt.wantMatchedPairs {
				if i < len(actualMatchedPairs) {
					assert.Equal(t, tt.wantMatchedPairs[i].resID, actualMatchedPairs[i].resID, "matched pair resID at index %d", i)
					assert.Equal(t, tt.wantMatchedPairs[i].sdkRegion, actualMatchedPairs[i].sdkRegion, "matched pair sdkRegion at index %d", i)
					assert.Equal(t, tt.wantMatchedPairs[i].sdkGroupARN, actualMatchedPairs[i].sdkGroupARN, "matched pair sdkGroupARN at index %d", i)
				}
			}

			// Handle nil vs empty slices
			if len(actualUnmatchedResIDs) == 0 && len(tt.wantUnmatchedResIDs) == 0 {
				// Both empty, no need to compare
			} else {
				// Verify unmatched resource endpoint groups
				assert.ElementsMatch(t, tt.wantUnmatchedResIDs, actualUnmatchedResIDs, "unmatched resource endpoint groups")
			}

			if len(actualUnmatchedSDKRegions) == 0 && len(tt.wantUnmatchedSDKRegions) == 0 {
				// Both empty, no need to compare
			} else {
				// Verify unmatched SDK endpoint groups
				assert.ElementsMatch(t, tt.wantUnmatchedSDKRegions, actualUnmatchedSDKRegions, "unmatched SDK endpoint groups")
			}
		})
	}
}

// createSDKEndpointGroup creates an SDK endpoint group with port overrides
func createSDKEndpointGroup(arn string, region string, portOverrides []agatypes.PortOverride) agatypes.EndpointGroup {
	return agatypes.EndpointGroup{
		EndpointGroupArn:    awssdk.String(arn),
		EndpointGroupRegion: awssdk.String(region),
		PortOverrides:       portOverrides,
	}
}

func Test_endpointGroupSynthesizer_getAllEndpointGroupsInListeners(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockGA := services.NewMockGlobalAccelerator(mockCtrl)

	// Create synthesizer instance with mock GA service
	s := &endpointGroupSynthesizer{
		gaService: mockGA,
		logger:    logr.Discard(),
	}

	// Test cases
	tests := []struct {
		name           string
		listenerARNs   []string
		mockSetup      func(mockGA *services.MockGlobalAccelerator)
		expectedGroups []agatypes.EndpointGroup
		expectError    bool
	}{
		{
			name:           "empty listener ARNs list",
			listenerARNs:   []string{},
			mockSetup:      func(mockGA *services.MockGlobalAccelerator) {},
			expectedGroups: []agatypes.EndpointGroup{},
			expectError:    false,
		},
		{
			name:         "single listener with no endpoint groups",
			listenerARNs: []string{"arn:aws:globalaccelerator::123456789012:listener/listener1"},
			mockSetup: func(mockGA *services.MockGlobalAccelerator) {
				mockGA.EXPECT().
					ListEndpointGroupsAsList(gomock.Any(), &globalaccelerator.ListEndpointGroupsInput{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:listener/listener1"),
					}).
					Return([]agatypes.EndpointGroup{}, nil)
			},
			expectedGroups: []agatypes.EndpointGroup{},
			expectError:    false,
		},
		{
			name:         "single listener with one endpoint group",
			listenerARNs: []string{"arn:aws:globalaccelerator::123456789012:listener/listener1"},
			mockSetup: func(mockGA *services.MockGlobalAccelerator) {
				mockGA.EXPECT().
					ListEndpointGroupsAsList(gomock.Any(), &globalaccelerator.ListEndpointGroupsInput{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:listener/listener1"),
					}).
					Return([]agatypes.EndpointGroup{
						{
							EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/eg1"),
							EndpointGroupRegion: awssdk.String("us-west-2"),
						},
					}, nil)
			},
			expectedGroups: []agatypes.EndpointGroup{
				{
					EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/eg1"),
					EndpointGroupRegion: awssdk.String("us-west-2"),
				},
			},
			expectError: false,
		},
		{
			name: "multiple listeners with endpoint groups",
			listenerARNs: []string{
				"arn:aws:globalaccelerator::123456789012:listener/listener1",
				"arn:aws:globalaccelerator::123456789012:listener/listener2",
			},
			mockSetup: func(mockGA *services.MockGlobalAccelerator) {
				// First listener
				mockGA.EXPECT().
					ListEndpointGroupsAsList(gomock.Any(), &globalaccelerator.ListEndpointGroupsInput{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:listener/listener1"),
					}).
					Return([]agatypes.EndpointGroup{
						{
							EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/eg1"),
							EndpointGroupRegion: awssdk.String("us-west-2"),
						},
					}, nil)

				// Second listener
				mockGA.EXPECT().
					ListEndpointGroupsAsList(gomock.Any(), &globalaccelerator.ListEndpointGroupsInput{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:listener/listener2"),
					}).
					Return([]agatypes.EndpointGroup{
						{
							EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/eg2"),
							EndpointGroupRegion: awssdk.String("eu-west-1"),
						},
						{
							EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/eg3"),
							EndpointGroupRegion: awssdk.String("ap-southeast-1"),
						},
					}, nil)
			},
			expectedGroups: []agatypes.EndpointGroup{
				{
					EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/eg1"),
					EndpointGroupRegion: awssdk.String("us-west-2"),
				},
				{
					EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/eg2"),
					EndpointGroupRegion: awssdk.String("eu-west-1"),
				},
				{
					EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/eg3"),
					EndpointGroupRegion: awssdk.String("ap-southeast-1"),
				},
			},
			expectError: false,
		},
		{
			name:         "error retrieving endpoint groups",
			listenerARNs: []string{"arn:aws:globalaccelerator::123456789012:listener/listener1"},
			mockSetup: func(mockGA *services.MockGlobalAccelerator) {
				mockGA.EXPECT().
					ListEndpointGroupsAsList(gomock.Any(), &globalaccelerator.ListEndpointGroupsInput{
						ListenerArn: awssdk.String("arn:aws:globalaccelerator::123456789012:listener/listener1"),
					}).
					Return(nil, errors.New("API error"))
			},
			expectedGroups: nil,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock expectations
			tt.mockSetup(mockGA)

			// Call the function
			ctx := context.Background()
			endpointGroups, err := s.getAllEndpointGroupsInListeners(ctx, tt.listenerARNs)

			// Check error
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Check endpoint groups
			assert.Equal(t, len(tt.expectedGroups), len(endpointGroups))

			if len(tt.expectedGroups) > 0 {
				// Compare each endpoint group
				for i, expectedGroup := range tt.expectedGroups {
					if i < len(endpointGroups) {
						assert.Equal(t, awssdk.ToString(expectedGroup.EndpointGroupArn),
							awssdk.ToString(endpointGroups[i].EndpointGroupArn))
						assert.Equal(t, awssdk.ToString(expectedGroup.EndpointGroupRegion),
							awssdk.ToString(endpointGroups[i].EndpointGroupRegion))
					}
				}
			}
		})
	}
}

func Test_endpointGroupSynthesizer_detectConflictsWithSDKEndpointGroups(t *testing.T) {
	testListener1ARN := "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd/listener/l-1"
	testListener2ARN := "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd/listener/l-2"
	tests := []struct {
		name              string
		resEndpointGroups []*agamodel.EndpointGroup
		sdkEndpointGroups []agatypes.EndpointGroup
		wantConflictCount int
		wantConflictPorts []int32
	}{
		{
			name:              "no endpoint groups",
			resEndpointGroups: []*agamodel.EndpointGroup{},
			sdkEndpointGroups: []agatypes.EndpointGroup{},
			wantConflictCount: 0,
			wantConflictPorts: []int32{},
		},
		{
			name: "no port overrides",
			resEndpointGroups: []*agamodel.EndpointGroup{
				createEndpointGroupWithPortOverrides("eg-1", "us-west-2", testListener1ARN, nil),
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				createSDKEndpointGroup("arn:aws:globalaccelerator::123456789012:endpointgroup/eg-sdk", "us-east-1", nil),
			},
			wantConflictCount: 0,
			wantConflictPorts: []int32{},
		},
		{
			name: "no conflicts - different endpoint ports",
			resEndpointGroups: []*agamodel.EndpointGroup{
				createEndpointGroupWithPortOverrides("eg-1", "us-west-2", testListener1ARN, []agamodel.PortOverride{
					{ListenerPort: 80, EndpointPort: 8080},
				}),
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				createSDKEndpointGroup(
					"arn:aws:globalaccelerator::123456789012:endpointgroup/eg-sdk",
					"us-east-1",
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(443), EndpointPort: awssdk.Int32(9090)}, // Different port
					},
				),
			},
			wantConflictCount: 0,
			wantConflictPorts: []int32{},
		},
		{
			name: "no conflicts - same endpoint port but different regions",
			resEndpointGroups: []*agamodel.EndpointGroup{
				createEndpointGroupWithPortOverrides("eg-1", "us-west-2", testListener1ARN, []agamodel.PortOverride{
					{ListenerPort: 80, EndpointPort: 8080},
				}),
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				createSDKEndpointGroup(
					"arn:aws:globalaccelerator::123456789012:endpointgroup/eg-sdk",
					"us-east-1", // Different region
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(443), EndpointPort: awssdk.Int32(8080)}, // Same endpoint port but different region
					},
				),
			},
			wantConflictCount: 0, // No conflict because different regions
			wantConflictPorts: []int32{},
		},
		{
			name: "single conflict - same region",
			resEndpointGroups: []*agamodel.EndpointGroup{
				createEndpointGroupWithPortOverrides("eg-1", "us-west-2", testListener1ARN, []agamodel.PortOverride{
					{ListenerPort: 80, EndpointPort: 8080},
				}),
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				createSDKEndpointGroup(
					"arn:aws:DIFFERENT:SERVICE:123456789012:endpoint-group/TEST_DIFFERENT_ARN/eg-sdk",
					"us-west-2", // Same region
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(443), EndpointPort: awssdk.Int32(8080)}, // Same endpoint port
					},
				),
			},
			wantConflictCount: 1,
			wantConflictPorts: []int32{8080},
		},
		{
			name: "multiple conflicts with same SDK group - same region",
			resEndpointGroups: []*agamodel.EndpointGroup{
				createEndpointGroupWithPortOverrides("eg-1", "us-west-2", testListener1ARN, []agamodel.PortOverride{
					{ListenerPort: 80, EndpointPort: 8080},
					{ListenerPort: 443, EndpointPort: 8443},
				}),
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				createSDKEndpointGroup(
					"arn:aws:DIFFERENT:SERVICE:123456789012:endpoint-group/DIFFERENT-TYPES/eg-sdk",
					"us-west-2", // Same region
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(81), EndpointPort: awssdk.Int32(8080)},  // Same as first port
						{ListenerPort: awssdk.Int32(444), EndpointPort: awssdk.Int32(8443)}, // Same as second port
					},
				),
			},
			wantConflictCount: 2,
			wantConflictPorts: []int32{8080, 8443},
		},
		{
			name: "multiple conflicts with same SDK group - different region (no conflict)",
			resEndpointGroups: []*agamodel.EndpointGroup{
				createEndpointGroupWithPortOverrides("eg-1", "us-west-2", testListener1ARN, []agamodel.PortOverride{
					{ListenerPort: 80, EndpointPort: 8080},
					{ListenerPort: 443, EndpointPort: 8443},
				}),
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				createSDKEndpointGroup(
					"arn:aws:globalaccelerator::123456789012:endpointgroup/eg-sdk",
					"us-east-1", // Different region
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(81), EndpointPort: awssdk.Int32(8080)},  // Same endpoint port but different region
						{ListenerPort: awssdk.Int32(444), EndpointPort: awssdk.Int32(8443)}, // Same endpoint port but different region
					},
				),
			},
			wantConflictCount: 0, // No conflicts because different regions
			wantConflictPorts: []int32{},
		},
		{
			name: "multiple conflicts with different SDK groups - same regions",
			resEndpointGroups: []*agamodel.EndpointGroup{
				createEndpointGroupWithPortOverrides("eg-1", "us-west-2", testListener1ARN, []agamodel.PortOverride{
					{ListenerPort: 80, EndpointPort: 8080},
					{ListenerPort: 443, EndpointPort: 8443},
				}),
				createEndpointGroupWithPortOverrides("eg-2", "eu-west-1", testListener2ARN, []agamodel.PortOverride{
					{ListenerPort: 81, EndpointPort: 9090},
				}),
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				createSDKEndpointGroup(
					"arn:aws:globalaccelerator::123456789012:endpointgroup/TEST_DIFFERENT_ARN_1/eg-sdk-1",
					"us-west-2", // Same region as first resource group
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(82), EndpointPort: awssdk.Int32(8080)}, // Conflicts with eg-1
					},
				),
				createSDKEndpointGroup(
					"arn:aws:globalaccelerator::123456789012:endpointgroup/TEST_DIFFERENT_ARN_2/eg-sdk-2",
					"eu-west-1", // Same region as second resource group
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(83), EndpointPort: awssdk.Int32(9090)}, // Conflicts with eg-2
					},
				),
			},
			wantConflictCount: 2,
			wantConflictPorts: []int32{8080, 9090},
		},
		{
			name: "multiple conflicts with different SDK groups - different regions (no conflicts)",
			resEndpointGroups: []*agamodel.EndpointGroup{
				createEndpointGroupWithPortOverrides("eg-1", "us-west-2", testListener1ARN, []agamodel.PortOverride{
					{ListenerPort: 80, EndpointPort: 8080},
				}),
				createEndpointGroupWithPortOverrides("eg-2", "eu-west-1", testListener2ARN, []agamodel.PortOverride{
					{ListenerPort: 81, EndpointPort: 9090},
				}),
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				createSDKEndpointGroup(
					"arn:aws:globalaccelerator::123456789012:endpointgroup/eg-sdk-1",
					"us-east-1", // Different region than first resource group
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(82), EndpointPort: awssdk.Int32(8080)}, // Same port but different region
					},
				),
				createSDKEndpointGroup(
					"arn:aws:globalaccelerator::123456789012:endpointgroup/eg-sdk-2",
					"ap-southeast-1", // Different region than second resource group
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(83), EndpointPort: awssdk.Int32(9090)}, // Same port but different region
					},
				),
			},
			wantConflictCount: 0, // No conflicts because different regions
			wantConflictPorts: []int32{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create synthesizer instance
			s := &endpointGroupSynthesizer{
				logger: logr.Discard(),
			}

			// Run the function
			ctx := context.Background()
			conflicts, err := s.detectConflictsWithSDKEndpointGroups(ctx, tt.resEndpointGroups, tt.sdkEndpointGroups)

			// Check errors
			assert.NoError(t, err)

			// Verify the number of conflicts
			assert.Equal(t, tt.wantConflictCount, len(conflicts), "conflict count should match")

			// Collect conflict ports for checking
			var conflictPorts []int32
			for port := range conflicts {
				conflictPorts = append(conflictPorts, port)
			}
			sort.Slice(conflictPorts, func(i, j int) bool {
				return conflictPorts[i] < conflictPorts[j]
			})

			// Sort expected ports for comparison
			expectedPorts := make([]int32, len(tt.wantConflictPorts))
			copy(expectedPorts, tt.wantConflictPorts)
			sort.Slice(expectedPorts, func(i, j int) bool {
				return expectedPorts[i] < expectedPorts[j]
			})

			// Check conflict ports
			assert.ElementsMatch(t, expectedPorts, conflictPorts, "conflicting ports should match")
		})
	}
}

func Test_endpointGroupSynthesizer_resolveConflictsWithSDKEndpointGroups(t *testing.T) {
	// Create a mock GlobalAccelerator service
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockGA := services.NewMockGlobalAccelerator(mockCtrl)

	testARN1 := "arn:aws:globalaccelerator::123456789012:endpointgroup/eg-1"
	testARN2 := "arn:aws:globalaccelerator::123456789012:endpointgroup/eg-2"

	tests := []struct {
		name              string
		conflicts         map[int32][]string
		sdkEndpointGroups []agatypes.EndpointGroup
		mockSetup         func(mockGA *services.MockGlobalAccelerator)
		expectError       bool
	}{
		{
			name:              "no conflicts",
			conflicts:         map[int32][]string{},
			sdkEndpointGroups: []agatypes.EndpointGroup{},
			mockSetup:         func(mockGA *services.MockGlobalAccelerator) {},
			expectError:       false,
		},
		{
			name: "single conflict with one group",
			conflicts: map[int32][]string{
				8080: {testARN1},
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				createSDKEndpointGroup(
					testARN1,
					"us-east-1",
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(80), EndpointPort: awssdk.Int32(8080)},  // Conflicting
						{ListenerPort: awssdk.Int32(443), EndpointPort: awssdk.Int32(8443)}, // Not conflicting
					},
				),
			},
			mockSetup: func(mockGA *services.MockGlobalAccelerator) {
				// Expect an update call with only the non-conflicting port override
				mockGA.EXPECT().UpdateEndpointGroupWithContext(gomock.Any(), &globalaccelerator.UpdateEndpointGroupInput{
					EndpointGroupArn: awssdk.String(testARN1),
					PortOverrides: []agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(443), EndpointPort: awssdk.Int32(8443)},
					},
				}).Return(&globalaccelerator.UpdateEndpointGroupOutput{}, nil)
			},
			expectError: false,
		},
		{
			name: "multiple conflicts with different groups",
			conflicts: map[int32][]string{
				8080: {testARN1},
				9090: {testARN2},
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				createSDKEndpointGroup(
					testARN1,
					"us-east-1",
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(80), EndpointPort: awssdk.Int32(8080)},  // Conflicting
						{ListenerPort: awssdk.Int32(443), EndpointPort: awssdk.Int32(8443)}, // Not conflicting
					},
				),
				createSDKEndpointGroup(
					testARN2,
					"eu-west-1",
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(81), EndpointPort: awssdk.Int32(9090)},  // Conflicting
						{ListenerPort: awssdk.Int32(444), EndpointPort: awssdk.Int32(9443)}, // Not conflicting
					},
				),
			},
			mockSetup: func(mockGA *services.MockGlobalAccelerator) {
				// Expect updates for both groups
				mockGA.EXPECT().UpdateEndpointGroupWithContext(gomock.Any(), &globalaccelerator.UpdateEndpointGroupInput{
					EndpointGroupArn: awssdk.String(testARN1),
					PortOverrides: []agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(443), EndpointPort: awssdk.Int32(8443)},
					},
				}).Return(&globalaccelerator.UpdateEndpointGroupOutput{}, nil)

				mockGA.EXPECT().UpdateEndpointGroupWithContext(gomock.Any(), &globalaccelerator.UpdateEndpointGroupInput{
					EndpointGroupArn: awssdk.String(testARN2),
					PortOverrides: []agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(444), EndpointPort: awssdk.Int32(9443)},
					},
				}).Return(&globalaccelerator.UpdateEndpointGroupOutput{}, nil)
			},
			expectError: false,
		},
		{
			name: "error during update",
			conflicts: map[int32][]string{
				8080: {testARN1},
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				createSDKEndpointGroup(
					testARN1,
					"us-east-1",
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(80), EndpointPort: awssdk.Int32(8080)}, // Conflicting
					},
				),
			},
			mockSetup: func(mockGA *services.MockGlobalAccelerator) {
				// Simulate an error during update
				mockGA.EXPECT().UpdateEndpointGroupWithContext(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("update failed"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up the mock expectations
			tt.mockSetup(mockGA)

			// Create synthesizer instance
			s := &endpointGroupSynthesizer{
				gaService: mockGA,
				logger:    logr.Discard(),
			}

			// Run the function
			ctx := context.Background()
			err := s.resolveConflictsWithSDKEndpointGroups(ctx, tt.conflicts, tt.sdkEndpointGroups)

			// Check errors
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

//func Test_endpointGroupSynthesizer_getAllEndpointGroupsInListeners(t *testing.T) {
//	// Create a mock GlobalAccelerator service
//	mockCtrl := gomock.NewController(t)
//	defer mockCtrl.Finish()
//	mockGA := services.NewMockGlobalAccelerator(mockCtrl)
//	listenerARN1 := "arn:aws:globalaccelerator::123456789012:listener/acc-1/listener-1"
//	listenerARN2 := "arn:aws:globalaccelerator::123456789012:listener/acc-1/listener-2"
//	listenerARN3 := "arn:aws:globalaccelerator::123456789012:listener/acc-2/listener-3"
//
//	endpointGroups1 := []agatypes.EndpointGroup{
//		{
//			EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/eg-1"),
//			EndpointGroupRegion: awssdk.String("us-east-1"),
//		},
//	}
//
//	endpointGroups2 := []agatypes.EndpointGroup{
//		{
//			EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/eg-2"),
//			EndpointGroupRegion: awssdk.String("us-west-2"),
//		},
//	}
//
//	endpointGroups3 := []agatypes.EndpointGroup{
//		{
//			EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/eg-3"),
//			EndpointGroupRegion: awssdk.String("eu-west-1"),
//		},
//		{
//			EndpointGroupArn:    awssdk.String("arn:aws:globalaccelerator::123456789012:endpointgroup/eg-4"),
//			EndpointGroupRegion: awssdk.String("eu-central-1"),
//		},
//	}
//
//	mockGA.EXPECT().ListEndpointGroupsAsList(gomock.Any(), &globalaccelerator.ListEndpointGroupsInput{
//		ListenerArn: awssdk.String(listenerARN1),
//	}).Return(endpointGroups1, nil)
//
//	mockGA.EXPECT().ListEndpointGroupsAsList(gomock.Any(), &globalaccelerator.ListEndpointGroupsInput{
//		ListenerArn: awssdk.String(listenerARN2),
//	}).Return(endpointGroups2, nil)
//
//	mockGA.EXPECT().ListEndpointGroupsAsList(gomock.Any(), &globalaccelerator.ListEndpointGroupsInput{
//		ListenerArn: awssdk.String(listenerARN3),
//	}).Return(endpointGroups3, nil)
//
//	// Create synthesizer instance
//	s := &endpointGroupSynthesizer{
//		gaService: mockGA,
//		logger:    logr.Discard(),
//	}
//
//	// Run the function
//	ctx := context.Background()
//	allEndpointGroups, err := s.getAllEndpointGroupsInListeners(ctx, nil)
//
//	// Check errors
//	assert.NoError(t, err)
//
//	// Verify that all endpoint groups are returned
//	expectedCount := len(endpointGroups1) + len(endpointGroups2) + len(endpointGroups3)
//	assert.Equal(t, expectedCount, len(allEndpointGroups), "should return all endpoint groups")
//
//	// Check that groups from all listeners are included
//	endpointGroupARNs := make(map[string]bool)
//	endpointGroupRegions := make(map[string]bool)
//
//	// Collect all ARNs and regions
//	for _, eg := range allEndpointGroups {
//		endpointGroupARNs[awssdk.ToString(eg.EndpointGroupArn)] = true
//		endpointGroupRegions[awssdk.ToString(eg.EndpointGroupRegion)] = true
//	}
//
//	// Check that expected endpoint groups are included
//	assert.Contains(t, endpointGroupARNs, awssdk.ToString(endpointGroups1[0].EndpointGroupArn), "should contain endpoint group 1")
//	assert.Contains(t, endpointGroupARNs, awssdk.ToString(endpointGroups2[0].EndpointGroupArn), "should contain endpoint group 2")
//	assert.Contains(t, endpointGroupARNs, awssdk.ToString(endpointGroups3[0].EndpointGroupArn), "should contain endpoint group 3")
//	assert.Contains(t, endpointGroupARNs, awssdk.ToString(endpointGroups3[1].EndpointGroupArn), "should contain endpoint group 4")
//
//	// Check that expected regions are included
//	assert.Contains(t, endpointGroupRegions, "us-east-1", "should contain us-east-1 region")
//	assert.Contains(t, endpointGroupRegions, "us-west-2", "should contain us-west-2 region")
//	assert.Contains(t, endpointGroupRegions, "eu-west-1", "should contain eu-west-1 region")
//	assert.Contains(t, endpointGroupRegions, "eu-central-1", "should contain eu-central-1 region")
//}

// Test_endpointGroupSynthesizer_detectConflictsWithSDKEndpointGroups_OwnListener tests that the
// detectConflictsWithSDKEndpointGroups function correctly ignores conflicts with our own listeners
func Test_endpointGroupSynthesizer_detectConflictsWithSDKEndpointGroups_OwnListener(t *testing.T) {
	// Define test listeners and endpoint groups
	testListener1ARN := "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd/listener/l-1"
	testListener2ARN := "arn:aws:globalaccelerator::123456789012:accelerator/5678efgh/listener/l-2"

	// ARNs for SDK endpoint groups - match the structure required by the extractor
	sdkGroup1ARN := "arn:aws:globalaccelerator::123456789012:accelerator/1234abcd/listener/l-1/endpoint-group/eg-1"
	sdkGroup2ARN := "arn:aws:globalaccelerator::123456789012:accelerator/5678efgh/listener/l-2/endpoint-group/eg-2"
	sdkGroup3ARN := "arn:aws:globalaccelerator::123456789012:accelerator/9012ijkl/listener/l-3/endpoint-group/eg-3"

	tests := []struct {
		name              string
		resEndpointGroups []*agamodel.EndpointGroup
		sdkEndpointGroups []agatypes.EndpointGroup
		wantConflictCount int
		wantConflictPorts []int32
	}{
		{
			name: "no conflict with own listener - same endpoint port in same region",
			resEndpointGroups: []*agamodel.EndpointGroup{
				createEndpointGroupWithPortOverrides("eg-1", "us-west-2", testListener1ARN, []agamodel.PortOverride{
					{ListenerPort: 80, EndpointPort: 8080},
				}),
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				// Same listener ARN pattern as resource group, should be ignored
				createSDKEndpointGroup(
					sdkGroup1ARN,
					"us-west-2", // Same region
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(81), EndpointPort: awssdk.Int32(8080)}, // Same endpoint port
					},
				),
			},
			wantConflictCount: 0, // No conflict detected with our own listener
			wantConflictPorts: []int32{},
		},
		{
			name: "conflict with external listener - same endpoint port in same region",
			resEndpointGroups: []*agamodel.EndpointGroup{
				createEndpointGroupWithPortOverrides("eg-1", "us-west-2", testListener1ARN, []agamodel.PortOverride{
					{ListenerPort: 80, EndpointPort: 8080},
				}),
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				// Different listener ARN than resource group, should detect conflict
				createSDKEndpointGroup(
					sdkGroup2ARN,
					"us-west-2", // Same region
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(81), EndpointPort: awssdk.Int32(8080)}, // Same endpoint port
					},
				),
			},
			wantConflictCount: 1,
			wantConflictPorts: []int32{8080},
		},
		{
			name: "multiple listeners in different regions - no conflicts",
			resEndpointGroups: []*agamodel.EndpointGroup{
				createEndpointGroupWithPortOverrides("eg-1", "us-west-2", testListener1ARN, []agamodel.PortOverride{
					{ListenerPort: 80, EndpointPort: 8080},
				}),
				createEndpointGroupWithPortOverrides("eg-2", "eu-west-1", testListener2ARN, []agamodel.PortOverride{
					{ListenerPort: 443, EndpointPort: 8080},
				}),
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				// Same listener ARN but different region - no conflict
				createSDKEndpointGroup(
					sdkGroup1ARN,
					"us-east-1",
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(81), EndpointPort: awssdk.Int32(8080)},
					},
				),
				// Different listener ARN but different region - no conflict
				createSDKEndpointGroup(
					sdkGroup3ARN,
					"eu-central-1",
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(82), EndpointPort: awssdk.Int32(8080)},
					},
				),
			},
			wantConflictCount: 0,
			wantConflictPorts: []int32{},
		},
		{
			name: "complex scenario - multiple listeners, regions, with own and external conflicts",
			resEndpointGroups: []*agamodel.EndpointGroup{
				// Resource group 1
				createEndpointGroupWithPortOverrides("eg-west-1", "us-west-2", testListener1ARN, []agamodel.PortOverride{
					{ListenerPort: 80, EndpointPort: 8080},
					{ListenerPort: 443, EndpointPort: 8443},
				}),
				// Resource group 2 - different region
				createEndpointGroupWithPortOverrides("eg-eu-1", "eu-west-1", testListener2ARN, []agamodel.PortOverride{
					{ListenerPort: 80, EndpointPort: 9090},
				}),
			},
			sdkEndpointGroups: []agatypes.EndpointGroup{
				// Same listener as eg-west-1, should NOT be conflict
				createSDKEndpointGroup(
					sdkGroup1ARN,
					"us-west-2",
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(81), EndpointPort: awssdk.Int32(8080)},
						{ListenerPort: awssdk.Int32(444), EndpointPort: awssdk.Int32(8443)},
					},
				),
				// Same listener as eg-eu-1, should NOT be conflict
				createSDKEndpointGroup(
					sdkGroup2ARN,
					"eu-west-1",
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(82), EndpointPort: awssdk.Int32(9090)},
					},
				),
				// Different listener in us-west-2, SHOULD be conflict with eg-west-1
				createSDKEndpointGroup(
					sdkGroup3ARN,
					"us-west-2",
					[]agatypes.PortOverride{
						{ListenerPort: awssdk.Int32(83), EndpointPort: awssdk.Int32(8080)},
					},
				),
			},
			wantConflictCount: 1, // Only one conflict (port 8080 in us-west-2 with external listener)
			wantConflictPorts: []int32{8080},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create synthesizer instance
			s := &endpointGroupSynthesizer{
				logger: logr.Discard(),
			}

			// Run the function
			ctx := context.Background()
			conflicts, err := s.detectConflictsWithSDKEndpointGroups(ctx, tt.resEndpointGroups, tt.sdkEndpointGroups)

			// Check errors
			assert.NoError(t, err)

			// Verify the number of conflicts
			assert.Equal(t, tt.wantConflictCount, len(conflicts), "conflict count should match")

			// Collect conflict ports for checking
			var conflictPorts []int32
			for port := range conflicts {
				conflictPorts = append(conflictPorts, port)
			}
			sort.Slice(conflictPorts, func(i, j int) bool {
				return conflictPorts[i] < conflictPorts[j]
			})

			// Sort expected ports for comparison
			expectedPorts := make([]int32, len(tt.wantConflictPorts))
			copy(expectedPorts, tt.wantConflictPorts)
			sort.Slice(expectedPorts, func(i, j int) bool {
				return expectedPorts[i] < expectedPorts[j]
			})

			// Check conflict ports
			assert.ElementsMatch(t, expectedPorts, conflictPorts, "conflicting ports should match")

			// If conflicts were detected, verify they're with the right endpoint groups
			if len(conflicts) > 0 {
				for port, sdkGroupARNs := range conflicts {
					for _, sdkGroupARN := range sdkGroupARNs {
						// For each test case, only check for listener ARNs that are actually in our resource groups
						// Get the list of listener ARNs used in this test's resource groups
						// Create a map of all listener ARNs used by our endpoint groups
						// In our test setup, we know exactly which endpoint groups use which listener ARNs
						// based on how we create them in the test cases
						usedListenerARNs := map[string]bool{
							testListener1ARN: false,
							testListener2ARN: false,
						}

						// For our test cases, we directly know which ARNs are used for each test
						switch tt.name {
						case "no conflict with own listener - same endpoint port in same region":
							usedListenerARNs[testListener1ARN] = true
						case "conflict with external listener - same endpoint port in same region":
							usedListenerARNs[testListener1ARN] = true
						case "multiple listeners with same port - one own, one external":
							usedListenerARNs[testListener1ARN] = true
						case "multiple listeners in different regions - no conflicts":
							usedListenerARNs[testListener1ARN] = true
							usedListenerARNs[testListener2ARN] = true
						case "complex scenario - multiple listeners, regions, with own and external conflicts":
							usedListenerARNs[testListener1ARN] = true
							usedListenerARNs[testListener2ARN] = true
						}

						// Only check if the conflict is NOT with listeners we're using
						if usedListenerARNs[testListener1ARN] {
							assert.NotContains(t, sdkGroupARN, testListener1ARN,
								"conflict should not be with our own listener (testListener1ARN) for port %d", port)
						}

						if usedListenerARNs[testListener2ARN] {
							assert.NotContains(t, sdkGroupARN, testListener2ARN,
								"conflict should not be with our own listener (testListener2ARN) for port %d", port)
						}
					}
				}
			}
		})
	}
}
