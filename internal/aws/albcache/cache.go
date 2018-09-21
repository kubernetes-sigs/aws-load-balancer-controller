package albcache

import (
	"time"

	"github.com/karlseguin/ccache"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/prometheus/client_golang/prometheus"
)

var cache *ccache.Cache
var mc metric.Collector

func NewCache(m metric.Collector) {
	cache = ccache.New(ccache.Configure())
	mc = m
}

func Get(c, n string) *ccache.Item {
	key := c + "." + n
	i := cache.Get(key)
	if i == nil || i.Expired() {
		mc.IncAPICacheCount(prometheus.Labels{"cache": c, "action": "miss"})
		return nil
	}
	mc.IncAPICacheCount(prometheus.Labels{"cache": c, "action": "hit"})
	return i
}

func Set(c, n string, value interface{}, duration time.Duration) {
	key := c + "." + n
	cache.Set(key, value, duration)
}

func Delete(cacheKey string) {
	cache.Delete(cacheKey)
}
