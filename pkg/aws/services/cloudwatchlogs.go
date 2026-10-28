package services

import (
	"context"

	cloudwatchlogssdk "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/provider"
)

// CloudWatchLogs provides the CloudWatch Logs vended log delivery APIs.
type CloudWatchLogs interface {
	// DescribeDeliverySourcesAsList returns all delivery sources in the account and region.
	DescribeDeliverySourcesAsList(ctx context.Context, input *cloudwatchlogssdk.DescribeDeliverySourcesInput) ([]types.DeliverySource, error)
	// DescribeDeliveryDestinationsAsList returns all delivery destinations in the account and region.
	DescribeDeliveryDestinationsAsList(ctx context.Context, input *cloudwatchlogssdk.DescribeDeliveryDestinationsInput) ([]types.DeliveryDestination, error)
	// DescribeDeliveriesAsList returns all deliveries in the account and region.
	DescribeDeliveriesAsList(ctx context.Context, input *cloudwatchlogssdk.DescribeDeliveriesInput) ([]types.Delivery, error)

	PutDeliverySourceWithContext(ctx context.Context, input *cloudwatchlogssdk.PutDeliverySourceInput) (*cloudwatchlogssdk.PutDeliverySourceOutput, error)
	DeleteDeliverySourceWithContext(ctx context.Context, input *cloudwatchlogssdk.DeleteDeliverySourceInput) (*cloudwatchlogssdk.DeleteDeliverySourceOutput, error)
	PutDeliveryDestinationWithContext(ctx context.Context, input *cloudwatchlogssdk.PutDeliveryDestinationInput) (*cloudwatchlogssdk.PutDeliveryDestinationOutput, error)
	DeleteDeliveryDestinationWithContext(ctx context.Context, input *cloudwatchlogssdk.DeleteDeliveryDestinationInput) (*cloudwatchlogssdk.DeleteDeliveryDestinationOutput, error)
	CreateDeliveryWithContext(ctx context.Context, input *cloudwatchlogssdk.CreateDeliveryInput) (*cloudwatchlogssdk.CreateDeliveryOutput, error)
	UpdateDeliveryConfigurationWithContext(ctx context.Context, input *cloudwatchlogssdk.UpdateDeliveryConfigurationInput) (*cloudwatchlogssdk.UpdateDeliveryConfigurationOutput, error)
	DeleteDeliveryWithContext(ctx context.Context, input *cloudwatchlogssdk.DeleteDeliveryInput) (*cloudwatchlogssdk.DeleteDeliveryOutput, error)
}

// NewCloudWatchLogs constructs new CloudWatchLogs implementation.
func NewCloudWatchLogs(awsClientsProvider provider.AWSClientsProvider) CloudWatchLogs {
	return &cloudWatchLogsClient{
		awsClientsProvider: awsClientsProvider,
	}
}

// default implementation for CloudWatchLogs.
type cloudWatchLogsClient struct {
	awsClientsProvider provider.AWSClientsProvider
}

func (c *cloudWatchLogsClient) DescribeDeliverySourcesAsList(ctx context.Context, input *cloudwatchlogssdk.DescribeDeliverySourcesInput) ([]types.DeliverySource, error) {
	client, err := c.awsClientsProvider.GetCloudWatchLogsClient(ctx, "DescribeDeliverySources")
	if err != nil {
		return nil, err
	}
	var result []types.DeliverySource
	paginator := cloudwatchlogssdk.NewDescribeDeliverySourcesPaginator(client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.DeliverySources...)
	}
	return result, nil
}

func (c *cloudWatchLogsClient) DescribeDeliveryDestinationsAsList(ctx context.Context, input *cloudwatchlogssdk.DescribeDeliveryDestinationsInput) ([]types.DeliveryDestination, error) {
	client, err := c.awsClientsProvider.GetCloudWatchLogsClient(ctx, "DescribeDeliveryDestinations")
	if err != nil {
		return nil, err
	}
	var result []types.DeliveryDestination
	paginator := cloudwatchlogssdk.NewDescribeDeliveryDestinationsPaginator(client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.DeliveryDestinations...)
	}
	return result, nil
}

func (c *cloudWatchLogsClient) DescribeDeliveriesAsList(ctx context.Context, input *cloudwatchlogssdk.DescribeDeliveriesInput) ([]types.Delivery, error) {
	client, err := c.awsClientsProvider.GetCloudWatchLogsClient(ctx, "DescribeDeliveries")
	if err != nil {
		return nil, err
	}
	var result []types.Delivery
	paginator := cloudwatchlogssdk.NewDescribeDeliveriesPaginator(client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, output.Deliveries...)
	}
	return result, nil
}

func (c *cloudWatchLogsClient) PutDeliverySourceWithContext(ctx context.Context, input *cloudwatchlogssdk.PutDeliverySourceInput) (*cloudwatchlogssdk.PutDeliverySourceOutput, error) {
	client, err := c.awsClientsProvider.GetCloudWatchLogsClient(ctx, "PutDeliverySource")
	if err != nil {
		return nil, err
	}
	return client.PutDeliverySource(ctx, input)
}

func (c *cloudWatchLogsClient) DeleteDeliverySourceWithContext(ctx context.Context, input *cloudwatchlogssdk.DeleteDeliverySourceInput) (*cloudwatchlogssdk.DeleteDeliverySourceOutput, error) {
	client, err := c.awsClientsProvider.GetCloudWatchLogsClient(ctx, "DeleteDeliverySource")
	if err != nil {
		return nil, err
	}
	return client.DeleteDeliverySource(ctx, input)
}

func (c *cloudWatchLogsClient) PutDeliveryDestinationWithContext(ctx context.Context, input *cloudwatchlogssdk.PutDeliveryDestinationInput) (*cloudwatchlogssdk.PutDeliveryDestinationOutput, error) {
	client, err := c.awsClientsProvider.GetCloudWatchLogsClient(ctx, "PutDeliveryDestination")
	if err != nil {
		return nil, err
	}
	return client.PutDeliveryDestination(ctx, input)
}

func (c *cloudWatchLogsClient) DeleteDeliveryDestinationWithContext(ctx context.Context, input *cloudwatchlogssdk.DeleteDeliveryDestinationInput) (*cloudwatchlogssdk.DeleteDeliveryDestinationOutput, error) {
	client, err := c.awsClientsProvider.GetCloudWatchLogsClient(ctx, "DeleteDeliveryDestination")
	if err != nil {
		return nil, err
	}
	return client.DeleteDeliveryDestination(ctx, input)
}

func (c *cloudWatchLogsClient) CreateDeliveryWithContext(ctx context.Context, input *cloudwatchlogssdk.CreateDeliveryInput) (*cloudwatchlogssdk.CreateDeliveryOutput, error) {
	client, err := c.awsClientsProvider.GetCloudWatchLogsClient(ctx, "CreateDelivery")
	if err != nil {
		return nil, err
	}
	return client.CreateDelivery(ctx, input)
}

func (c *cloudWatchLogsClient) UpdateDeliveryConfigurationWithContext(ctx context.Context, input *cloudwatchlogssdk.UpdateDeliveryConfigurationInput) (*cloudwatchlogssdk.UpdateDeliveryConfigurationOutput, error) {
	client, err := c.awsClientsProvider.GetCloudWatchLogsClient(ctx, "UpdateDeliveryConfiguration")
	if err != nil {
		return nil, err
	}
	return client.UpdateDeliveryConfiguration(ctx, input)
}

func (c *cloudWatchLogsClient) DeleteDeliveryWithContext(ctx context.Context, input *cloudwatchlogssdk.DeleteDeliveryInput) (*cloudwatchlogssdk.DeleteDeliveryOutput, error) {
	client, err := c.awsClientsProvider.GetCloudWatchLogsClient(ctx, "DeleteDelivery")
	if err != nil {
		return nil, err
	}
	return client.DeleteDelivery(ctx, input)
}
