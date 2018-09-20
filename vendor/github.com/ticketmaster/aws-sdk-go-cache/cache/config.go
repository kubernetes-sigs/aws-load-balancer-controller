package cache

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/karlseguin/ccache"
)

type Config struct {
	DefaultTTL  time.Duration
	specificTTL map[string]time.Duration
	caches      map[string]*ccache.Cache
	metrics     *cacheCollector
}

const cacheNameFormat = "%v.%v"

// NewConfig returns a cache configuration with the defaultTTL
func NewConfig(defaultTTL time.Duration) *Config {
	return &Config{
		DefaultTTL:  defaultTTL,
		specificTTL: make(map[string]time.Duration),
		caches:      make(map[string]*ccache.Cache),
	}
}

func (c *Config) NewCacheCollector(namespace string) prometheus.Collector {
	c.metrics = newCacheCollector(namespace)
	return c.metrics
}

// SetCacheTTL sets a unique TTL for the service and operation
func (c *Config) SetCacheTTL(serviceName, operationName string, ttl time.Duration) {
	c.specificTTL[fmt.Sprintf(cacheNameFormat, serviceName, operationName)] = ttl
}

// FlushCache flushes all caches for a service
func (c *Config) FlushCache(serviceName string) {
	for cacheName := range c.caches {
		if strings.HasPrefix(cacheName, serviceName) {
			c.caches[cacheName] = ccache.New(ccache.Configure())
			n := strings.Split(cacheName, ".")
			c.incFlush(n[0], n[1])
		}
	}
}

func (c *Config) flushCaches(r *request.Request) {
	opName := r.Operation.Name

	if isCachable(opName) {
		return
	}

	c.FlushCache(r.ClientInfo.ServiceName)

	if strings.Contains(opName, "Tags") {
		c.FlushCache(resourcegroupstaggingapi.ServiceName)
	}
}

func (c *Config) getCache(r *request.Request) *ccache.Cache {
	_, ok := c.caches[cacheName(r)]
	if !ok {
		cache := ccache.New(ccache.Configure())
		c.caches[cacheName(r)] = cache
	}
	return c.caches[cacheName(r)]
}

func (c *Config) get(r *request.Request) *ccache.Item {
	return c.getCache(r).Get(cacheKey(r))
}

func (c *Config) set(r *request.Request, object interface{}) {
	if !isCachable(r.Operation.Name) {
		return
	}

	// Check for custom ttl
	ttl, ok := c.specificTTL[cacheName(r)]
	if !ok {
		ttl = c.DefaultTTL
	}

	c.getCache(r).Set(cacheKey(r), object, ttl)
}

func cacheName(r *request.Request) string {
	return fmt.Sprintf(cacheNameFormat, r.ClientInfo.ServiceName, r.Operation.Name)
}

func cacheKey(r *request.Request) string {
	return awsutil.Prettify(r.Params)
}

func isCachable(operationName string) bool {
	if !(strings.HasPrefix(operationName, "Describe") ||
		strings.HasPrefix(operationName, "List") ||
		strings.HasPrefix(operationName, "Get")) {
		return false
	}
	return true
}

func (c *Config) incHit(r *request.Request) {
	if c.metrics != nil {
		c.metrics.incHit(r)
	}
}

func (c *Config) incMiss(r *request.Request) {
	if c.metrics != nil {
		c.metrics.incMiss(r)
	}
}

func (c *Config) incFlush(serviceName, operationName string) {
	if c.metrics != nil {
		c.metrics.incFlush(serviceName, operationName)
	}
}
