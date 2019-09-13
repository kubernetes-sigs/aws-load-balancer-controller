package utils

import (
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	CacheNoExpiration time.Duration = 0
)

type Cache interface {
	// Set will set value for specified key with validityPeriod duration.
	Set(key string, value interface{}, duration time.Duration)

	// Get will get value for specified key
	Get(key string) (interface{}, bool)

	// Shrink will shrink cache to only contain desiredKeys.
	Shrink(desiredKeys sets.String)
}

func NewCache() Cache {
	return &cache{
		items: make(map[string]cacheItem),
		mu:    sync.RWMutex{},
	}
}

type cacheItem struct {
	value interface{}

	expireAt *time.Time
}

type cache struct {
	items map[string]cacheItem
	mu    sync.RWMutex
}

func (c *cache) Set(key string, value interface{}, duration time.Duration) {
	item := cacheItem{
		value:    value,
		expireAt: nil,
	}
	if duration != CacheNoExpiration {
		expireAt := time.Now().Add(duration)
		item.expireAt = &expireAt
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = item
}

func (c *cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, ok := c.items[key]
	if !ok {
		return nil, false
	}
	if item.expireAt != nil && time.Now().After(*item.expireAt) {
		return nil, false
	}
	return item.value, true
}

func (c *cache) Shrink(desiredKeys sets.String) {
	c.mu.Lock()
	defer c.mu.Unlock()
	existingKeys := sets.StringKeySet(c.items)
	for key := range existingKeys.Difference(desiredKeys) {
		delete(c.items, key)
	}
}
