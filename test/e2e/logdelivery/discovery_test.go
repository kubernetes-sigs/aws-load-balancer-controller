package logdelivery

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/services"
)

func TestReadDeliveryState_filtersOtherLoadBalancers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cwl := services.NewMockCloudWatchLogs(ctrl)
	cwl.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliverySource{
		{Name: awssdk.String("owned"), ResourceArns: []string{"test-lb"}},
		{Name: awssdk.String("unrelated"), ResourceArns: []string{"other-lb"}},
	}, nil)
	cwl.EXPECT().DescribeDeliveriesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.Delivery{
		{Id: awssdk.String("owned"), DeliverySourceName: awssdk.String("owned"), DeliveryDestinationArn: awssdk.String("owned-arn")},
		{Id: awssdk.String("unrelated"), DeliverySourceName: awssdk.String("unrelated"), DeliveryDestinationArn: awssdk.String("unrelated-arn")},
	}, nil)
	cwl.EXPECT().DescribeDeliveryDestinationsAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliveryDestination{
		{Arn: awssdk.String("owned-arn")}, {Arn: awssdk.String("unrelated-arn")},
	}, nil)
	state, err := readDeliveryState(context.Background(), cwl, "test-lb")
	assert.NoError(t, err)
	assert.Len(t, state.sources, 1)
	assert.Contains(t, state.sources, "owned")
	assert.Len(t, state.deliveries, 1)
	assert.Contains(t, state.deliveries, "owned")
	assert.Len(t, state.destinations, 1)
	assert.Contains(t, state.destinations, "owned-arn")
}

func TestReadDeliveryState_withoutSources(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cwl := services.NewMockCloudWatchLogs(ctrl)
	cwl.EXPECT().DescribeDeliverySourcesAsList(gomock.Any(), gomock.Any()).Return([]cwltypes.DeliverySource{{Name: awssdk.String("other"), ResourceArns: []string{"other-lb"}}}, nil)
	state, err := readDeliveryState(context.Background(), cwl, "test-lb")
	assert.NoError(t, err)
	assert.Empty(t, state.sources)
	assert.Empty(t, state.deliveries)
	assert.Empty(t, state.destinations)
}
