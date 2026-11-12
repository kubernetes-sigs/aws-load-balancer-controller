package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	cloudwatchlogssdk "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/stretchr/testify/require"
	clocktesting "k8s.io/utils/clock/testing"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/provider"
)

type testCloudWatchLogsProvider struct {
	provider.AWSClientsProvider
	client *cloudwatchlogssdk.Client
}

func (p testCloudWatchLogsProvider) GetCloudWatchLogsClient(context.Context, string) (*cloudwatchlogssdk.Client, error) {
	return p.client, nil
}

func TestDeliveryListCache(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Now())
	cache := deliveryListCache[string]{clock: fakeClock}
	firstClient, secondClient := &cloudwatchlogssdk.Client{}, &cloudwatchlogssdk.Client{}
	calls := 0
	fetch := func() ([]string, error) {
		calls++
		return []string{fmt.Sprintf("fetch-%d", calls)}, nil
	}
	first, err := cache.get(firstClient, true, fetch)
	require.NoError(t, err)
	first[0] = "caller changed the slice"
	cached, err := cache.get(firstClient, true, fetch)
	require.NoError(t, err)
	require.Equal(t, []string{"fetch-1"}, cached)
	require.Equal(t, 1, calls)

	// Token-based requests must not replace the complete cached list.
	_, err = cache.get(firstClient, false, fetch)
	require.NoError(t, err)
	cached, err = cache.get(firstClient, true, fetch)
	require.NoError(t, err)
	require.Equal(t, []string{"fetch-1"}, cached)
	require.Equal(t, 2, calls)

	fakeClock.Step(deliveryDiscoveryCacheTTL)
	_, err = cache.get(firstClient, true, fetch)
	require.NoError(t, err)
	require.Equal(t, 3, calls)
	_, err = cache.get(secondClient, true, fetch)
	require.NoError(t, err)
	require.Equal(t, 4, calls, "clients for different accounts or regions must not share results")

	cache.invalidate()
	_, err = cache.get(secondClient, true, func() ([]string, error) {
		return []string{"partial"}, errors.New("second page failed")
	})
	require.Error(t, err)
	refreshed, err := cache.get(secondClient, true, fetch)
	require.NoError(t, err)
	require.Equal(t, []string{"fetch-5"}, refreshed, "failed listings must not populate the cache")
}

func TestDeliveryListCacheConcurrentRequests(t *testing.T) {
	cache := deliveryListCache[string]{}
	client := &cloudwatchlogssdk.Client{}
	var fetches atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cache.get(client, true, func() ([]string, error) {
				fetches.Add(1)
				return []string{"shared"}, nil
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), fetches.Load())
}

func TestDeliveryListCacheInvalidatesOverlappingFetch(t *testing.T) {
	cache := deliveryListCache[string]{}
	client := &cloudwatchlogssdk.Client{}
	fetchStarted, finishFetch, invalidated := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		_, _ = cache.get(client, true, func() ([]string, error) {
			close(fetchStarted)
			<-finishFetch
			return []string{"stale"}, nil
		})
	}()
	<-fetchStarted
	go func() { cache.invalidate(); close(invalidated) }()
	close(finishFetch)
	<-invalidated
	items, err := cache.get(client, true, func() ([]string, error) { return []string{"fresh"}, nil })
	require.NoError(t, err)
	require.Equal(t, []string{"fresh"}, items)
}

func TestCloudWatchLogsDiscoveryPaginationAndMutationInvalidation(t *testing.T) {
	ctx := context.Background()
	for _, mutation := range []struct {
		operation string
		list      string
		call      func(CloudWatchLogs) error
	}{
		{"PutDeliverySource", "DescribeDeliverySources", func(c CloudWatchLogs) error {
			_, err := c.PutDeliverySourceWithContext(ctx, &cloudwatchlogssdk.PutDeliverySourceInput{Name: awssdk.String("source"), ResourceArn: awssdk.String("arn:aws:elasticloadbalancing:us-west-2:111122223333:loadbalancer/app/test/id"), LogType: awssdk.String("ALB_ACCESS_LOGS")})
			return err
		}},
		{"DeleteDeliverySource", "DescribeDeliverySources", func(c CloudWatchLogs) error {
			_, err := c.DeleteDeliverySourceWithContext(ctx, &cloudwatchlogssdk.DeleteDeliverySourceInput{Name: awssdk.String("source")})
			return err
		}},
		{"PutDeliveryDestination", "DescribeDeliveryDestinations", func(c CloudWatchLogs) error {
			_, err := c.PutDeliveryDestinationWithContext(ctx, &cloudwatchlogssdk.PutDeliveryDestinationInput{Name: awssdk.String("destination")})
			return err
		}},
		{"DeleteDeliveryDestination", "DescribeDeliveryDestinations", func(c CloudWatchLogs) error {
			_, err := c.DeleteDeliveryDestinationWithContext(ctx, &cloudwatchlogssdk.DeleteDeliveryDestinationInput{Name: awssdk.String("destination")})
			return err
		}},
		{"CreateDelivery", "DescribeDeliveries", func(c CloudWatchLogs) error {
			_, err := c.CreateDeliveryWithContext(ctx, &cloudwatchlogssdk.CreateDeliveryInput{DeliverySourceName: awssdk.String("source"), DeliveryDestinationArn: awssdk.String("arn:aws:logs:us-west-2:111122223333:delivery-destination:destination")})
			return err
		}},
		{"UpdateDeliveryConfiguration", "DescribeDeliveries", func(c CloudWatchLogs) error {
			_, err := c.UpdateDeliveryConfigurationWithContext(ctx, &cloudwatchlogssdk.UpdateDeliveryConfigurationInput{Id: awssdk.String("delivery")})
			return err
		}},
		{"DeleteDelivery", "DescribeDeliveries", func(c CloudWatchLogs) error {
			_, err := c.DeleteDeliveryWithContext(ctx, &cloudwatchlogssdk.DeleteDeliveryInput{Id: awssdk.String("delivery")})
			return err
		}},
	} {
		for _, failMutation := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/failure=%t", mutation.operation, failMutation), func(t *testing.T) {
				var mu sync.Mutex
				counts := map[string]int{}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, operation, _ := strings.Cut(r.Header.Get("X-Amz-Target"), ".")
					mu.Lock()
					counts[operation]++
					mu.Unlock()
					w.Header().Set("Content-Type", "application/x-amz-json-1.1")
					if operation == mutation.operation && failMutation {
						w.WriteHeader(http.StatusBadRequest)
						fmt.Fprint(w, `{"__type":"AccessDeniedException","message":"denied"}`)
						return
					}
					var input struct{ NextToken *string }
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
						t.Error(err)
					}
					field, identifier := "", "name"
					switch operation {
					case "DescribeDeliverySources":
						field = "deliverySources"
					case "DescribeDeliveryDestinations":
						field = "deliveryDestinations"
					case "DescribeDeliveries":
						field, identifier = "deliveries", "id"
					}
					if field == "" {
						fmt.Fprint(w, `{}`)
					} else if input.NextToken == nil {
						fmt.Fprintf(w, `{"%s":[{"%s":"first"}],"nextToken":"page-2"}`, field, identifier)
					} else {
						fmt.Fprintf(w, `{"%s":[{"%s":"second"}]}`, field, identifier)
					}
				}))
				defer server.Close()
				sdkClient := cloudwatchlogssdk.NewFromConfig(awssdk.Config{Region: "us-west-2", Credentials: awssdk.AnonymousCredentials{}, HTTPClient: server.Client(), RetryMaxAttempts: 1}, func(o *cloudwatchlogssdk.Options) { o.BaseEndpoint = &server.URL })
				client := NewCloudWatchLogs(testCloudWatchLogsProvider{client: sdkClient})
				listAll := func() {
					sources, err := client.DescribeDeliverySourcesAsList(ctx, &cloudwatchlogssdk.DescribeDeliverySourcesInput{})
					require.NoError(t, err)
					require.Len(t, sources, 2)
					destinations, err := client.DescribeDeliveryDestinationsAsList(ctx, &cloudwatchlogssdk.DescribeDeliveryDestinationsInput{})
					require.NoError(t, err)
					require.Len(t, destinations, 2)
					deliveries, err := client.DescribeDeliveriesAsList(ctx, &cloudwatchlogssdk.DescribeDeliveriesInput{})
					require.NoError(t, err)
					require.Len(t, deliveries, 2)
				}
				listAll()
				listAll()
				err := mutation.call(client)
				if failMutation {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				listAll()
				mu.Lock()
				defer mu.Unlock()
				for _, list := range []string{"DescribeDeliverySources", "DescribeDeliveryDestinations", "DescribeDeliveries"} {
					want := 2
					if list == mutation.list {
						want = 4
					}
					require.Equal(t, want, counts[list], list)
				}
				require.Equal(t, 1, counts[mutation.operation])
			})
		}
	}
}
