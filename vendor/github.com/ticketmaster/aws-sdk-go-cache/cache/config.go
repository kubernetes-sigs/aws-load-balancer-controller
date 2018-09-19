package cache

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/karlseguin/ccache"
)

var configContextKey = new(contextKeyType)

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

func (c *Config) get(serviceName, operationName string, params interface{}) *ccache.Item {
	return c.cache.Get(c.cacheKey(serviceName, operationName, params))
}

func (c *Config) set(serviceName, operationName string, params interface{}, object *cacheObject) {
	if !cachable(operationName) {
		return
	}

	ttl, ok := c.specificTTL[c.serviceOperation(serviceName, operationName)]
	if !ok {
		ttl = c.DefaultTTL
	}
	c.cache.Set(c.cacheKey(serviceName, operationName, params), object, ttl)
}

func getConfig(ctx context.Context) *Config {
	v := ctx.Value(configContextKey)
	if v == nil {
		return nil
	}
	return v.(*Config)
}

func (c *Config) serviceOperation(serviceName, operationName string) string {
	return serviceName + "." + operationName
}

func (c *Config) cacheKey(serviceName, operationName string, params interface{}) string {
	return c.serviceOperation(serviceName, operationName) + "." + awsutil.Prettify(params)
}

func cachable(operationName string) bool {
	if !(strings.HasPrefix(operationName, "Describe") ||
		strings.HasPrefix(operationName, "List") ||
		strings.HasPrefix(operationName, "Get")) {
		return false
	}
	return true
}
