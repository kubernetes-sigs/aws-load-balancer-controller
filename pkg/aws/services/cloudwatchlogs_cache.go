package services

import (
	"slices"
	"sync"
	"time"

	cloudwatchlogssdk "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"k8s.io/utils/clock"
)

const deliveryDiscoveryCacheTTL = 30 * time.Second

// deliveryListCache shares complete, read-only discovery results across stacks using the same SDK client.
// One entry bounds memory and prevents sharing account or region data across different clients.
type deliveryListCache[T any] struct {
	mu        sync.Mutex
	client    *cloudwatchlogssdk.Client
	items     []T
	expiresAt time.Time
	clock     clock.PassiveClock
}

// get coalesces concurrent full-list requests and caches only successful results for a short interval.
// A request starting at a pagination token bypasses the cache so it cannot replace a complete list.
// Resource contents are read-only; the returned slice itself can be modified without changing the cache.
func (c *deliveryListCache[T]) get(client *cloudwatchlogssdk.Client, cacheable bool, fetch func() ([]T, error)) ([]T, error) {
	if !cacheable {
		return fetch()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.clock == nil {
		c.clock = clock.RealClock{}
	}
	if c.client == client && c.clock.Now().Before(c.expiresAt) {
		return slices.Clone(c.items), nil
	}
	items, err := fetch()
	if err != nil {
		return nil, err
	}
	c.client = client
	c.items = slices.Clone(items)
	c.expiresAt = c.clock.Now().Add(deliveryDiscoveryCacheTTL)
	return items, nil
}

// invalidate discards a list after a mutation attempt, including ambiguous failures that may have reached AWS.
// Taking the same lock as get also discards a stale fetch that overlapped the mutation.
func (c *deliveryListCache[T]) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.client = nil
	c.items = nil
	c.expiresAt = time.Time{}
}
