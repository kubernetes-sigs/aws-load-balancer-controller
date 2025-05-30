package elbv2

import (
	"sync"
)

// TargetGroupCache provides a cache for sharing target group data between synthesizers
type TargetGroupCache struct {
	sdkTargetGroups []TargetGroupWithTags
	mutex           sync.RWMutex
}

// NewTargetGroupCache creates a new target group cache
func NewTargetGroupCache() *TargetGroupCache {
	return &TargetGroupCache{}
}

// SetSDKTargetGroups stores the SDK target groups in the cache
func (c *TargetGroupCache) SetSDKTargetGroups(targetGroups []TargetGroupWithTags) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.sdkTargetGroups = targetGroups
}

// GetSDKTargetGroups retrieves the SDK target groups from the cache
func (c *TargetGroupCache) GetSDKTargetGroups() ([]TargetGroupWithTags, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	if c.sdkTargetGroups == nil {
		return nil, false
	}
	return c.sdkTargetGroups, true
}
