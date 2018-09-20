package cache

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/wafregional"

	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"

	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/karlseguin/ccache"
)

type Config struct {
	DefaultTTL  time.Duration
	specificTTL map[string]time.Duration
	caches      map[string]*ccache.Cache
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

// SetCacheTTL sets a unique TTL for the service and operation
func (c *Config) SetCacheTTL(serviceName, operationName string, ttl time.Duration) {
	c.specificTTL[fmt.Sprintf(cacheNameFormat, serviceName, operationName)] = ttl
}

func (c *Config) getCache(r *request.Request) *ccache.Cache {
	_, ok := c.caches[cacheName(r)]
	if !ok {
		cache := ccache.New(ccache.Configure())
		c.caches[cacheName(r)] = cache
	}
	return c.caches[cacheName(r)]
}

// FlushCache flushes all caches for a service
func (c *Config) FlushCache(serviceName string) {
	for cacheName := range c.caches {
		if strings.HasPrefix(cacheName, serviceName) {
			c.caches[cacheName] = ccache.New(ccache.Configure())
		}
	}
}

func (c *Config) flushCaches(r *request.Request) {
	var caches []string
	opName := r.Operation.Name
	switch r.ClientInfo.ServiceName {
	case ec2.ServiceName:
		if strings.HasPrefix(opName, "CreateTags") || strings.HasPrefix(opName, "DeleteTags") {
			caches = append(caches, resourcegroupstaggingapi.ServiceName)
		}
		if strings.HasPrefix(opName, "Assign") ||
			strings.HasPrefix(opName, "Associate") ||
			strings.HasPrefix(opName, "Attach") ||
			strings.HasPrefix(opName, "Authorize") ||
			strings.HasPrefix(opName, "Create") ||
			strings.HasPrefix(opName, "Delete") ||
			strings.HasPrefix(opName, "Import") ||
			strings.HasPrefix(opName, "Modify") ||
			strings.HasPrefix(opName, "Move") ||
			strings.HasPrefix(opName, "Register") ||
			strings.HasPrefix(opName, "Reject") ||
			strings.HasPrefix(opName, "Release") ||
			strings.HasPrefix(opName, "Replace") ||
			strings.HasPrefix(opName, "Request") ||
			strings.HasPrefix(opName, "Reset") ||
			strings.HasPrefix(opName, "Restore") ||
			strings.HasPrefix(opName, "Revoke") ||
			strings.HasPrefix(opName, "Run") ||
			strings.HasPrefix(opName, "Start") ||
			strings.HasPrefix(opName, "Terminate") ||
			strings.HasPrefix(opName, "Unassign") ||
			strings.HasPrefix(opName, "Update") {
			caches = append(caches, ec2.ServiceName)
		}
	case elbv2.ServiceName:
		if strings.HasPrefix(opName, "AddTags") || strings.HasPrefix(opName, "RemoveTags") {
			caches = append(caches, resourcegroupstaggingapi.ServiceName)
		}
		if strings.HasPrefix(opName, "Add") ||
			strings.HasPrefix(opName, "Create") ||
			strings.HasPrefix(opName, "Delete") ||
			strings.HasPrefix(opName, "Deregister") ||
			strings.HasPrefix(opName, "Modify") ||
			strings.HasPrefix(opName, "Register") ||
			strings.HasPrefix(opName, "Remove") {
			caches = append(caches, elbv2.ServiceName)
		}
	case wafregional.ServiceName:
		if strings.HasPrefix(opName, "Associate") ||
			strings.HasPrefix(opName, "Create") ||
			strings.HasPrefix(opName, "Delete") ||
			strings.HasPrefix(opName, "Disassociate") ||
			strings.HasPrefix(opName, "Put") ||
			strings.HasPrefix(opName, "Update") {
			caches = append(caches, wafregional.ServiceName)
		}

	case resourcegroupstaggingapi.ServiceName:
	case acm.ServiceName:
	case iam.ServiceName:

	}

	for _, cache := range caches {
		c.FlushCache(cache)
	}
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
