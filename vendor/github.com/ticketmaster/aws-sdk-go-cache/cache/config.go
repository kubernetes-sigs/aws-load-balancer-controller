package cache

import (
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/request"

	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/karlseguin/ccache"
)

type Config struct {
	DefaultTTL  time.Duration
	specificTTL map[string]time.Duration
	cache       *ccache.Cache
}

// NewConfig returns a cache configuration with the defaultTTL
func NewConfig(defaultTTL time.Duration) *Config {
	return &Config{
		DefaultTTL:  defaultTTL,
		specificTTL: make(map[string]time.Duration),
		cache:       ccache.New(ccache.Configure()),
	}
}

// SetCacheTTL sets a unique TTL for the service and operation
func (c *Config) SetCacheTTL(serviceName, operationName string, ttl time.Duration) {
	c.specificTTL[c.serviceOperation(serviceName, operationName)] = ttl
}

func (c *Config) get(r *request.Request) *ccache.Item {
	return c.cache.Get(c.cacheKey(r.ClientInfo.ServiceName, r.Operation.Name, r.Params))
}

func (c *Config) set(r *request.Request, object interface{}) {
	if !isCachable(r.Operation.Name) {
		return
	}

	// Check for custom ttl
	ttl, ok := c.specificTTL[c.serviceOperation(r.ClientInfo.ServiceName, r.Operation.Name)]
	if !ok {
		ttl = c.DefaultTTL
	}

	c.cache.Set(c.cacheKey(r.ClientInfo.ServiceName, r.Operation.Name, r.Params), object, ttl)
}

func (c *Config) serviceOperation(serviceName, operationName string) string {
	return serviceName + "." + operationName
}

func (c *Config) cacheKey(serviceName, operationName string, params interface{}) string {
	return c.serviceOperation(serviceName, operationName) + "." + awsutil.Prettify(params)
}

func isCachable(operationName string) bool {
	if !(strings.HasPrefix(operationName, "Describe") ||
		strings.HasPrefix(operationName, "List") ||
		strings.HasPrefix(operationName, "Get")) {
		return false
	}
	return true
}
