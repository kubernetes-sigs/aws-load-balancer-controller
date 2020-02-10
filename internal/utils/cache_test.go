package utils

import (
	"testing"
	"time"

	"github.com/magiconair/properties/assert"
	"k8s.io/apimachinery/pkg/util/sets"
)

func Test_CacheGet(t *testing.T) {
	for _, tc := range []struct {
		key            string
		value          string
		duration       time.Duration
		waitDuration   time.Duration
		expectedValue  interface{}
		expectedExists bool
	}{
		{
			key:            "key-1",
			value:          "value-1",
			duration:       CacheNoExpiration,
			waitDuration:   10 * time.Millisecond,
			expectedValue:  "value-1",
			expectedExists: true,
		},
		{
			key:            "key-2",
			value:          "value-2",
			duration:       1 * time.Millisecond,
			waitDuration:   10 * time.Millisecond,
			expectedValue:  nil,
			expectedExists: false,
		},
		{
			key:            "key-3",
			value:          "value-3",
			duration:       20 * time.Second,
			waitDuration:   1 * time.Millisecond,
			expectedValue:  "value-3",
			expectedExists: true,
		},
	} {
		cache := NewCache()
		t.Run(tc.key, func(t *testing.T) {
			cache.Set(tc.key, tc.value, tc.duration)
			<-time.After(tc.waitDuration)
			value, exists := cache.Get(tc.key)
			assert.Equal(t, value, tc.expectedValue)
			assert.Equal(t, exists, tc.expectedExists)
		})
	}
}

func Test_CacheShrink(t *testing.T) {
	cache := NewCache()
	cache.Set("key-1", "value-1", CacheNoExpiration)
	cache.Set("key-2", "value-2", CacheNoExpiration)
	cache.Set("key-3", "value-3", CacheNoExpiration)
	cache.Shrink(sets.NewString("key-1", "key-3"))

	for _, tc := range []struct {
		key            string
		expectedValue  interface{}
		expectedExists bool
	}{
		{
			key:            "key-1",
			expectedValue:  "value-1",
			expectedExists: true,
		},
		{
			key:            "key-2",
			expectedValue:  nil,
			expectedExists: false,
		},
		{
			key:            "key-3",
			expectedValue:  "value-3",
			expectedExists: true,
		},
	} {
		value, exists := cache.Get(tc.key)
		assert.Equal(t, value, tc.expectedValue)
		assert.Equal(t, exists, tc.expectedExists)
	}
}
